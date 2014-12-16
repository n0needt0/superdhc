package main

import (
	//dhc4dns "./dhc4/dns"
	dhc4 "./dhc4"
	"GoStats/stats"
	"bytes"
	"container/list"
	"encoding/json"
	//"errors"
	"flag"
	"fmt"
	zmq "github.com/alecthomas/gozmq"
	"github.com/gorilla/pat"
	logging "github.com/op/go-logging"
	"github.com/paulbellamy/ratecounter"

	"github.com/vaughan0/go-ini"
	"io"
	"net"
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
	HEARTBEAT_INTERVAL   = time.Second      //  time.Duration
	HEARTBEAT_LIVENESS   = 3                //  1-3 is good
	INTERVAL_INIT        = time.Second      //  Initial reconnect
	INTERVAL_MAX         = 32 * time.Second //  After exponential backoff
	PPP_READY            = "\001"           //  Signals worker is ready
	PPP_HEARTBEAT        = "\002"           //  Signals worker heartbeat
	STATS_WINDOW_SECONDS = 60
	HEALTH_STATE_UP      = 1
	HEALTH_STATE_DOWN    = 0
)

//this is log file
var logFile *os.File
var logFormat = logging.MustStringFormatter("%{color}%{time:15:04:05.000000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}")
var log = logging.MustGetLogger("logfile")
var Gloglevel logging.Level = logging.DEBUG
var GTestStats []string = []string{"ping_total_ms", "tcp_total_ms", "dns_total_ms", "http_get_total_ms", "http_head_total_ms", "http_post_total_ms"}

//the followign are flags passed from commandline
var Configfile *string = flag.String("config", "/etc/dhc4/node.cfg", "Config file location default: /etc/fortihealth/node.cfg")
var help *bool = flag.Bool("help", false, "Show options")
var cfg ini.File
var Gdebugdelay bool = false //debugdelay

//uris for front facing and back facing connections
var Gbackuri, Gfronturi string

//Stats structure
var GStats = struct {
	RateCounter *ratecounter.RateCounter
	Rate        int64
	Workers     int
	Details     map[string]*stats.Stats
	sync.RWMutex
}{
	ratecounter.NewRateCounter(time.Duration(1) * time.Second),
	int64(0),
	0,
	make(map[string]*stats.Stats),
	sync.RWMutex{},
}

type Hc struct {
	BusyTs    int32 `json:"bts" bson:"bts"`
	Sn        string
	Cron      string
	LastRunTs int32 `json:"lr" bson:"lr"`
	NextRunTs int32 `json:"nr" bson:"nr"`
	Hcid      string
	HcType    string `json:"hct" bson:"hct"`
	Ver       string `json:"v" bson:"v"`
	Meta      map[string]interface{}
}

type HcTester interface {
	DoTest(map[string]interface{}) error
}

//ping headers
type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

type TimeoutMsec struct {
	Overall   int
	Dns       int
	Test      int
	AfterTest int
}

type HcPing struct {
	Host        string
	TimeoutMsec TimeoutMsec //Various timeouts
	Plossok     float32     //percent loss
	Packets     int         //we use 4 iterations to test by default up to 10 otherwise
	Size        int         //packet byte size 10 to 100
	Res         map[string]interface{}
}

//ping headers

type Msg struct {
	SERIAL string
	TS     int64
	ERROR  string
	HC     Hc
	RESULT map[string]interface{}
}

var GetRequest = func(JsonMsg []byte) (Msg, error) {
	var msg = Msg{
		SERIAL: "",
		TS:     0,
		ERROR:  "",
		HC:     Hc{},
		RESULT: make(map[string]interface{}),
	}

	d := json.NewDecoder(bytes.NewReader(JsonMsg))
	d.UseNumber()

	if err := d.Decode(&msg); err != nil {
		return msg, err
	}

	return msg, nil
}

var GetReply = func(msg Msg) ([]byte, error) {

	jsonstr, err := json.Marshal(msg)
	if err != nil {
		return []byte("{}"), err
	}

	return jsonstr, nil
}

//parse command line
func init() {
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		os.Exit(1)
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

	loglevel = strings.ToUpper(loglevel)

	Gloglevel, err = logging.LogLevel(loglevel)
	if err != nil {
		Gloglevel = logging.DEBUG
	}

	logging.SetBackend(logformatted)

	//see what we have here
	for name, section := range cfg {
		log.Info("Section: %v\n", name)
		for k, v := range section {
			log.Info("%v: %v\n", k, v)
		}
	}

	logging.SetLevel(Gloglevel, "")

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

		//initialize system detail stats collectors
		for _, stat_name := range GTestStats {
			GStats.Details[stat_name] = &stats.Stats{}
		}

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

		go func() {
			for {
				select {
				case <-time.After(time.Duration(STATS_WINDOW_SECONDS) * time.Second):
					GStats.Lock()
					GStats.Rate = GStats.RateCounter.Rate()
					GStats.Workers = workers.Len()

					for _, stat_name := range GTestStats {
						//only if observed
						if GStats.Details[stat_name].Count() > 0 {
							log.Notice("SYS %s Count:  %d", stat_name, GStats.Details[stat_name].Count())
							log.Notice("SYS %s Min:  %f", stat_name, GStats.Details[stat_name].Min())
							log.Notice("SYS %s Max:  %f", stat_name, GStats.Details[stat_name].Max())
							log.Notice("SYS %s Mean:  %f", stat_name, GStats.Details[stat_name].Mean())
						}
					}

					//reset stats so we only collect what we need
					for _, stat_name := range GTestStats {
						GStats.Details[stat_name] = &stats.Stats{}
					}

					GStats.Unlock()
					log.Notice("SYS: for last %d sec: rate %d req/sec, workers %d", STATS_WINDOW_SECONDS, GStats.Rate, GStats.Workers)
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

		//timeout channel, it will be pased to everything below and when it closes we return to calling

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

			if len(frames) == 3 {

				//real work
				msg, err := GetRequest(frames[2])
				if err != nil {
					log.Warning("GetRequest %s", err)
				}

				log.Debug("request %+v", msg)

				//do work

				hctype := strings.TrimSpace(strings.ToUpper(msg.HC.HcType))

				switch hctype {
				case "DNS":
					testStart := makeTimestamp()

					//chanel on which results if any will come from the ping
					reschan := make(chan map[string]interface{})

					log.Debug("%s spec %v", hctype, msg.HC.Meta)

					//return ping struct with proposed config, will fill in default values
					hcdns, err := dhc4.NewDns(msg.HC.Meta)

					if err != nil {
						//not much we can do without ping obj
						msg.ERROR = fmt.Sprintf("%s", err)
						log.Warning("%s", msg.ERROR)
					} else {
						//fireaway
						go hcdns.DoTest(reschan)

						testing := true

						for testing {
							select {
							case res := <-reschan:
								log.Debug("DNS RES: %+v", res)
								//copy what we got
								for k, v := range res {
									hcdns.Res[string(k)] = v
								}
								testing = false
							case <-time.After(time.Duration(hcdns.Timeout) * time.Second):
								msg := fmt.Sprintf("DNS: %s:%s timeout %d sec", hcdns.Host, hcdns.RecType, hcdns.Timeout)
								log.Warning(msg)
								hcdns.Res["msg"] = msg
								testing = false
							}
						}
					}

					totalTestTime := makeTimestamp() - testStart
					hcdns.Res["total_ms"] = totalTestTime
					GStats.Details["dns_total_ms"].Update(float64(totalTestTime))
					log.Debug("DNS RESULT %+v", hcdns.Res)
					log.Debug("DNS Exit %s", hcdns.Host)
					msg.RESULT = hcdns.Res

				case "TCP":
					testStart := makeTimestamp()

					//chanel on which results if any will come from the ping
					reschan := make(chan map[string]interface{})

					log.Debug("%s spec %v", hctype, msg.HC.Meta)

					//return ping struct with proposed config, will fill in default values
					hctcp, err := dhc4.NewTcp(msg.HC.Meta)

					if err != nil {
						//not much we can do without ping obj
						msg.ERROR = fmt.Sprintf("%s", err)
						log.Warning("%s", msg.ERROR)
					} else {
						//fireaway
						go hctcp.DoTest(reschan)

						testing := true

						for testing {
							select {
							case res := <-reschan:
								log.Debug("TCP RES: %+v", res)
								//copy what we got
								for k, v := range res {
									hctcp.Res[string(k)] = v
								}
								testing = false
							case <-time.After(time.Duration(hctcp.Timeout) * time.Second):
								msg := fmt.Sprintf("TCP: %s %s:%s timeout %d sec", hctcp.Proto, hctcp.Host, hctcp.Port, hctcp.Timeout)
								log.Warning(msg)
								hctcp.Res["msg"] = msg
								testing = false
							}
						}
					}

					totalTestTime := makeTimestamp() - testStart
					hctcp.Res["total_ms"] = totalTestTime
					GStats.Details["tcp_total_ms"].Update(float64(totalTestTime))
					log.Debug("TCP RESULT %+v", hctcp.Res)
					log.Debug("TCP Exit %s", hctcp.Host)
					msg.RESULT = hctcp.Res

				case "PING":
					testStart := makeTimestamp()

					//chanel on which results if any will come from the ping
					reschan := make(chan map[string]interface{})

					log.Debug("%s spec %v", hctype, msg.HC.Meta)

					//return ping struct with proposed config, will fill in default values
					hcping, err := dhc4.NewPing(msg.HC.Meta)

					if err != nil {
						//not much we can do without ping obj
						msg.ERROR = fmt.Sprintf("%s", err)
						log.Warning("%s", msg.ERROR)
					} else {
						//fireaway
						go hcping.DoTest(reschan)

						testing := true

						for testing {
							select {
							case res := <-reschan:
								log.Debug("PING RES: %+v", res)
								//copy what we got
								for k, v := range res {
									hcping.Res[string(k)] = v
								}
								testing = false
							case <-time.After(time.Duration(hcping.Timeout) * time.Second):
								msg := fmt.Sprintf("PING: %s timeout %d sec", hcping.Host, hcping.Timeout)
								log.Warning(msg)
								hcping.Res["msg"] = msg
								testing = false
							}
						}
					}

					totalTestTime := makeTimestamp() - testStart
					hcping.Res["total_ms"] = totalTestTime
					GStats.Details["ping_total_ms"].Update(float64(totalTestTime))
					log.Debug("PING RESULT %+v", hcping.Res)
					log.Debug("PING Exit %s", hcping.Host)
					msg.RESULT = hcping.Res

				case "HTTP_GET":
				//	log.Info("%s test params %v", hctype, msg.HC.Meta)

				case "HTTP_POST":
				//	log.Info("%s test params %v", hctype, msg.HC.Meta)

				case "HTTP_HEAD":
				//	log.Info("%s test params %v", hctype, msg.HC.Meta)

				default:
					log.Debug("%s test params %v", hctype, msg.HC.Meta)
					msg.ERROR = fmt.Sprintf("Invalid Test Submitted \"%s\"", msg.HC.HcType)
				}

				Reply, err := GetReply(msg)
				if err != nil {
					log.Warning("GetReply %s", err)
				}

				log.Debug("reply %+v", msg)

				frames[2] = Reply

				worker.SendMultipart(frames, 0)

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
