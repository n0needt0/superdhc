package main

import (
	"bytes"
	"container/list"
	"encoding/json"
	"flag"
	"fmt"
	zmq "github.com/alecthomas/gozmq"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/pat"
	logging "github.com/op/go-logging"
	"github.com/paulbellamy/ratecounter"
	"github.com/vaughan0/go-ini"
	"io"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//TODO may move to config one day
const (
	HEARTBEAT_INTERVAL = time.Second      //  time.Duration
	HEARTBEAT_LIVENESS = 3                //  1-3 is good
	INTERVAL_INIT      = time.Second      //  Initial reconnect
	INTERVAL_MAX       = 32 * time.Second //  After exponential backoff
	PPP_READY          = "\001"           //  Signals worker is ready
	PPP_HEARTBEAT      = "\002"           //  Signals worker heartbeat
)

//this is log file
var logFile *os.File
var logFormat = logging.MustStringFormatter("%{color}%{time:15:04:05.000000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}")
var log = logging.MustGetLogger("logfile")
var Gloglevel logging.Level = logging.DEBUG
var Gdebugdelay bool = false
var Grpool *redis.Pool

//this where errors go to die
var err error

//the followign are flags passed from commandline
var Configfile *string = flag.String("config", "/etc/dhc4/history-home.cfg", "Config file location default: /etc/dhc4/history-home.cfg")
var help *bool = flag.Bool("help", false, "Show options")
var cfg ini.File

//uris for front facing and back facing connections
var Gbackuri, Gfronturi, Gredis string

//Stats structure
var GStats = struct {
	RateCounter *ratecounter.RateCounter
	Rate        int64
	Workers     int
	sync.RWMutex
}{
	ratecounter.NewRateCounter(time.Duration(1) * time.Second),
	int64(0),
	0,
	sync.RWMutex{},
}

//Helper function to precess requests
var GetRequest = func(JsonMsg []byte) (map[string]interface{}, error) {
	msg := make(map[string]interface{})

	d := json.NewDecoder(bytes.NewReader(JsonMsg))
	d.UseNumber()

	if err := d.Decode(&msg); err != nil {
		return msg, err
	}

	return msg, nil
}

//parse command line
func init() {
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		os.Exit(1)
	}
}

//Redis pool func
func newPool(server string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

//  Helper function that returns a new configured socket
//  connected to the workers uri

func WorkerSocket(context *zmq.Context) *zmq.Socket {

	worker, err := context.NewSocket(zmq.DEALER)
	if err != nil {
		log.Fatalf("can not start backend worker %s", err)
	}

	err = worker.Connect(Gbackuri)
	if err != nil {
		log.Fatalf("can not connect backend worker %s", err)
	}

	//  Tell queue we're ready for work
	log.Debug("worker ready")

	err = worker.Send([]byte(PPP_READY), 0)

	if err != nil {
		log.Fatal("can not Send ready on backend worker", err)
	}
	return worker
}

type PPWorker struct {
	address []byte    //  Address of worker
	expiry  time.Time //  Expires at this time
}

func NewPPWorker(address []byte) *PPWorker {
	return &PPWorker{
		address: address,
		expiry:  time.Now().Add(HEARTBEAT_LIVENESS * HEARTBEAT_INTERVAL),
	}
}

type WorkerQueue struct {
	queue *list.List
}

func NewWorkerQueue() *WorkerQueue {
	return &WorkerQueue{
		queue: list.New(),
	}
}

func (workers *WorkerQueue) Len() int {
	return workers.queue.Len()
}

func (workers *WorkerQueue) Next() []byte {
	elem := workers.queue.Back()
	worker, _ := elem.Value.(*PPWorker)
	workers.queue.Remove(elem)
	return worker.address
}

func (workers *WorkerQueue) Ready(worker *PPWorker) {
	for elem := workers.queue.Front(); elem != nil; elem = elem.Next() {
		if w, _ := elem.Value.(*PPWorker); string(w.address) == string(worker.address) {
			workers.queue.Remove(elem)
			break
		}
	}
	workers.queue.PushBack(worker)
}

func (workers *WorkerQueue) Purge() {
	now := time.Now()
	for elem := workers.queue.Front(); elem != nil; elem = workers.queue.Front() {
		if w, _ := elem.Value.(*PPWorker); w.expiry.After(now) {
			break
		}
		workers.queue.Remove(elem)
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	//parse config
	cfg, err := ini.LoadFile(*Configfile)

	if err != nil {
		log.Fatalf("parse config "+*Configfile+" file error: %s", err)
	}

	logfile, ok := cfg.Get("system", "logfile")
	if !ok {
		log.Fatalf("'logfile' missing from 'system' section")
	}

	//open log file
	logFile, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Log file error: %s %s", logfile, err)
	}

	defer func() {
		logFile.WriteString(fmt.Sprintf("closing %s", time.UnixDate))
		logFile.Close()
	}()

	logback := logging.NewLogBackend(logFile, "", 0)
	logformatted := logging.NewBackendFormatter(logback, logFormat)

	loglevel, ok := cfg.Get("system", "loglevel")
	if !ok {
		log.Fatal("'loglevel' missing from 'system' section")
	}

	Gloglevel, err = logging.LogLevel(loglevel)
	if err != nil {
		Gloglevel = logging.DEBUG
	}
	logging.SetLevel(Gloglevel, "")
	logging.SetBackend(logformatted)

	//see what we have here
	for name, section := range cfg {
		log.Debug("Section: %v\n", name)
		for k, v := range section {
			log.Debug("%v: %v\n", k, v)
		}
	}

	//kill channel to programatically
	killch := make(chan os.Signal, 1)
	signal.Notify(killch, os.Interrupt)
	signal.Notify(killch, syscall.SIGTERM)
	signal.Notify(killch, syscall.SIGINT)
	signal.Notify(killch, syscall.SIGQUIT)
	go func() {
		<-killch
		log.Fatalf("Interrupt %s", time.Now().String())
	}()

	//get goodies from config file
	//http server
	httphost, ok := cfg.Get("http", "host")
	if !ok {
		log.Fatalf("'host' missing from 'http' section")
	}

	httpport, ok := cfg.Get("http", "port")
	if !ok {
		log.Fatalf("'port' missing from 'http' section")
	}

	strworkers, ok := cfg.Get("system", "workers")
	if !ok {
		log.Fatalf("'workers' missing from 'system' section")
	}

	numworkers, err := strconv.Atoi(strworkers)
	if err != nil {
		log.Fatalf("'workers' parameter malformed in 'system' section")
	}

	Gfronturi, ok = cfg.Get("zmq", "fronturi")
	if !ok {
		log.Fatalf("'fronturi' uri string missing from 'zmq' section")
	}

	Gredis, ok = cfg.Get("redis", "server")
	if !ok {
		log.Fatalf("'server' missing from 'redis' section")
	}

	Gbackuri, ok = cfg.Get("zmq", "backuri")
	if !ok {
		log.Fatalf("'backuri' missing from 'zmq' section")
	}

	ddelay, ok := cfg.Get("system", "debugdelay")
	if ok {
		if ddelayb, err := strconv.ParseBool(ddelay); err == nil {
			Gdebugdelay = ddelayb
		}
	}

	//we need to start 2 servers, http for status and zmq
	wg := &sync.WaitGroup{}
	wg.Add(1)
	//first start http interface for self stats
	go func() {

		r := pat.New()
		r.Get("/health", http.HandlerFunc(healthHandle))
		r.Get("/loglevel/{loglevel}", http.HandlerFunc(logHandle))
		r.Get("/delay", http.HandlerFunc(delayHandle))
		http.Handle("/", r)

		log.Notice("Listening %s : %s", httphost, httpport)

		err = http.ListenAndServe(httphost+":"+httpport, nil)
		if err != nil {
			log.Fatalf("ListenAndServe: %s", err)
		}

		wg.Done()
	}()

	wg.Add(1)

	go func() {

		//see if we run as slave no reason to run own router process
		//workers will use what ever specified in masteruri
		slave, ok := cfg.Get("system", "slave")
		if ok {
			if r, err := strconv.ParseBool(slave); err == nil && r == true {
				return
			}
		}

		//start workers
		for i := 1; i <= numworkers; i++ {
			go worker()
		}

		// Prepare our context and sockets
		context, err := zmq.NewContext()
		if err != nil {
			log.Fatalf("starting zmq server context %s", err)
		}
		defer context.Close()

		//start service for outside connections
		log.Info("Zmq server starting %s", Gfronturi)

		frontend, err := context.NewSocket(zmq.ROUTER)
		if err != nil {
			log.Fatalf("starting frontend zmq %s", err)
		}
		defer frontend.Close()
		err = frontend.Bind(Gfronturi)
		if err != nil {
			log.Fatalf("error binding frontend zmq %s", err)
		}

		// start socket for internal workers connection
		backend, err := context.NewSocket(zmq.ROUTER)
		if err != nil {
			log.Fatalf("starting backend zmq %s", err)
		}
		defer backend.Close()
		err = backend.Bind(Gbackuri)
		if err != nil {
			log.Fatalf(" binding backend zmq %s", err)
		}

		//prestart
		workers := NewWorkerQueue()
		heartbeatAt := time.Now().Add(HEARTBEAT_INTERVAL)

		Grpool = newPool(Gredis)

		go func() {
			for {
				select {
				case <-time.After(time.Duration(5) * time.Second):
					GStats.Lock()
					GStats.Rate = GStats.RateCounter.Rate()
					GStats.Workers = workers.Len()
					GStats.Unlock()
					log.Info("rate %d req/sec, workers %d", GStats.Rate, GStats.Workers)
				}
			}
		}()

		// connect work threads to client threads via a queue
		for {
			items := zmq.PollItems{
				zmq.PollItem{Socket: backend, Events: zmq.POLLIN},
				zmq.PollItem{Socket: frontend, Events: zmq.POLLIN},
			}

			//  Poll frontend only if we have available workers
			if workers.Len() > 0 {
				zmq.Poll(items, HEARTBEAT_INTERVAL)
			} else {
				zmq.Poll(items[:1], HEARTBEAT_INTERVAL)
			}

			//  Handle worker activity on backend
			if items[0].REvents&zmq.POLLIN != 0 {
				frames, err := backend.RecvMultipart(0)
				if err != nil {
					log.Warning("receiving from back end %s", err)
					//drop message to teh ground
					continue
				}

				address := frames[0]
				workers.Ready(NewPPWorker(address))

				//  Validate control message, or return reply to client
				if msg := frames[1:]; len(msg) == 1 {
					switch status := string(msg[0]); status {
					case PPP_READY:
						//log.Debug("rcv. ready")
					case PPP_HEARTBEAT:
						//log.Debug("rcv. heartbeat")
					default:
						log.Warning("Invalid message from worker: %v", msg)
					}
				} else {
					GStats.RateCounter.Incr(int64(1))
					frontend.SendMultipart(msg, 0)
				}
			}

			if items[1].REvents&zmq.POLLIN != 0 {
				//  Now get next client request, route to next worker
				frames, err := frontend.RecvMultipart(0)
				if err != nil {
					log.Warning("receiving from front end %s", err)
					//drop message to teh ground
					continue
				}
				frames = append([][]byte{workers.Next()}, frames...)
				backend.SendMultipart(frames, 0)
			}

			//  .split handle heartbeating
			//  We handle heartbeating after any socket activity. First we send
			//  heartbeats to any idle workers if it's time. Then we purge any
			//  dead workers:
			if heartbeatAt.Before(time.Now()) {
				for elem := workers.queue.Front(); elem != nil; elem = elem.Next() {
					w, _ := elem.Value.(*PPWorker)
					msg := [][]byte{w.address, []byte(PPP_HEARTBEAT)}
					backend.SendMultipart(msg, 0)
				}
				heartbeatAt = time.Now().Add(HEARTBEAT_INTERVAL)
			}

			workers.Purge()
		}
		wg.Done()

	}()

	wg.Wait()

}

//Zmq worker Handlers, they may connect to local or remote queue
//  The interesting parts here are
//  the heartbeating, which lets the worker detect if the queue has
//  died, and vice-versa:

func insert(message_id string, healthcheck_id string, health_check_location string, health_test_class string, stats_array map[string]interface{}) error {
	rconn := Grpool.Get()
	defer rconn.Close()

	score := ""

	v, ok := stats_array["checked"]
	if !ok {
		//in case its missing
		score = fmt.Sprintf("%d", time.Now().Unix())
	} else {
		score, ok = v.(string)
		if !ok {
			//in case its missing
			score = fmt.Sprintf("%d", time.Now().Unix())
		}
	}

	//check last char of a timestamp, some how redis dont like trailing zeroes
	if score[:len(score)-1] == "0" {
		score = fmt.Sprintf("%s%s", score[:len(score)-1], "1")
	}

	zkey := strings.ToUpper(fmt.Sprintf("%s:%s:%s", healthcheck_id, health_test_class, health_check_location))

	zval, err := json.Marshal(stats_array)
	if err != nil {
		return err
	}

	log.Debug("inserting %s %s %s", score, zkey, zval)

	_, err = rconn.Do("ZADD", zkey, score, zval)
	if err != nil {
		return err
	}

	err = rconn.Flush()
	if err != nil {
		return err
	}

	return nil
}

func worker() {
	context, err := zmq.NewContext()
	if err != nil {
		log.Warning("starting workers context %s", err)
		return
	}
	defer context.Close()

	worker := WorkerSocket(context)

	liveness := HEARTBEAT_LIVENESS
	interval := INTERVAL_INIT
	heartbeatAt := time.Now().Add(HEARTBEAT_INTERVAL)

	for {
		items := zmq.PollItems{
			zmq.PollItem{Socket: worker, Events: zmq.POLLIN},
		}

		zmq.Poll(items, HEARTBEAT_INTERVAL)

		if items[0].REvents&zmq.POLLIN != 0 {
			frames, err := worker.RecvMultipart(0)
			if err != nil {
				log.Warning("worker error %s", err)
				worker.Close()
			}

			if len(frames) == 2 {

				//real work

				msg, err := GetRequest(frames[1])
				if err != nil {
					log.Warning("GetRequest %s", err)
				}

				//test for data parts
				goodkeys := []string{"sender", "message_id", "health_check_location", "healthcheck_id", "health_test_class", "health_stats"}
				keysok := true

				for _, key := range goodkeys {
					_, ok := msg[key]
					if !ok {
						log.Warning("missing %s in message %+v", key, msg)
						keysok = false
						break
					}
				}

				if !keysok {
					continue
				}

				message_id, ok := msg["message_id"].(string)
				if !ok {
					log.Warning("bad message id %+v", msg["message_id"])
					continue
				}

				healthcheck_id, ok := msg["healthcheck_id"].(string)
				if !ok {
					log.Warning("bad healthcheck_id %+v", msg["healthcheck_id"])
					continue
				}

				health_check_location, ok := msg["health_check_location"].(string)
				if !ok {
					log.Warning("bad health_check_location %+v", msg["health_check_location"])
					continue
				}

				health_test_class, ok := msg["health_test_class"].(string)
				if !ok {
					log.Warning("bad health_test_class %+v", msg["health_test_class"])
					continue
				}

				health_stats, ok := msg["health_stats"].(map[string]interface{})
				if !ok {
					log.Warning("bad health_stats %+v", msg["health_stats"])
					continue
				}

				err = insert(message_id, healthcheck_id, health_check_location, health_test_class, health_stats)
				if err != nil {
					log.Warning("REDIS error inserting %s : %s", message_id, err)
				} else {
					log.Debug("REDIS inserted %s", message_id)
				}

				liveness = HEARTBEAT_LIVENESS

				if Gdebugdelay {
					//this is if we set extra delay for debugging
					log.Notice("Sleeping 1 sec..")
					time.Sleep(time.Duration(1) * time.Second)
				}

			} else if len(frames) == 1 && string(frames[0]) == PPP_HEARTBEAT {
				//log.Debug("rcv. queue heartbeat")
				liveness = HEARTBEAT_LIVENESS
			} else {
				log.Warning("rcv. invalid message")
			}
			interval = INTERVAL_INIT
		} else if liveness--; liveness == 0 {
			log.Warning("Heartbeat failure, Reconnecting queue in %d sec...", interval/time.Second)
			time.Sleep(interval)
			if interval < INTERVAL_MAX {
				interval *= 2
			}
			worker.Close()
			worker = WorkerSocket(context)
			liveness = HEARTBEAT_LIVENESS
		}

		if heartbeatAt.Before(time.Now()) {
			heartbeatAt = time.Now().Add(HEARTBEAT_INTERVAL)
			worker.Send([]byte(PPP_HEARTBEAT), 0)
		}
	}
}

//Http Handlers
func serve404(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "Not Found")
}

func delayHandle(w http.ResponseWriter, r *http.Request) {

	if Gdebugdelay {
		Gdebugdelay = false
	} else {
		Gdebugdelay = true
	}
	log.Notice("Debug delay %t", Gdebugdelay)
	io.WriteString(w, fmt.Sprintf("\ndelay: %t\n", Gdebugdelay))
}

func logHandle(w http.ResponseWriter, r *http.Request) {

	loglevel := r.URL.Query().Get(":loglevel")

	loglevel = strings.ToUpper(loglevel)

	Gloglevel, err := logging.LogLevel(loglevel)
	if err != nil {
		Gloglevel = logging.DEBUG
		loglevel = "DEBUG"
	}

	res := fmt.Sprintf("\nSetting log level to: %s \n Valid log levels are CRITICAL, ERROR,  WARNING, NOTICE, INFO, DEBUG\n", loglevel)

	log.Notice(res)

	logging.SetLevel(Gloglevel, "")
	io.WriteString(w, res)
}

func healthHandle(w http.ResponseWriter, r *http.Request) {

	res := make(map[string]string)
	major, minor, patch := zmq.Version()

	res["status"] = "OK"

	if GStats.Workers < 1 {
		res["status"] = "DEAD"
	}

	res["ts"] = time.Now().String()
	res["zmq_version"] = fmt.Sprintf("%d.%d.%d", major, minor, patch)
	res["ratesec"] = fmt.Sprintf("%d", GStats.Rate)
	res["workers"] = fmt.Sprintf("%d", GStats.Workers)

	b, err := json.Marshal(res)
	if err != nil {
		log.Error("error: %s", err)
	}

	w.Write(b)
	return
}

//dumps given obj
func dump(t interface{}) string {
	s := reflect.ValueOf(t).Elem()
	typeOfT := s.Type()
	res := ""

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		res = fmt.Sprint(res, fmt.Sprintf("%s %s = %v\n", typeOfT.Field(i).Name, f.Type(), f.Interface()))
	}

	return res
}

func makeTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
