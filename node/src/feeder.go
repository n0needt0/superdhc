//Cleaner  daemon based on HA Client
//Andrew Yasinsky

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	zmq "github.com/alecthomas/gozmq"
	"github.com/gorhill/cronexpr"
	"github.com/gorilla/pat"
	"github.com/paulbellamy/ratecounter"
	"github.com/vaughan0/go-ini"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"log"
	"math/rand"
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

//this is log file
var logFile *os.File

//random maker
var Random *os.File

//this where errors go to die
var err error

//the followign are flags passed from commandline
var logdebug *bool = flag.Bool("debug", false, "enable debug logging")
var Configfile *string = flag.String("config", "/etc/fortihealth/feeder.cfg", "Config file location")
var help *bool = flag.Bool("help", false, "Show these options")
var delay *bool = flag.Bool("delay", false, "Extra 1 sec delay between requests")
var cfg ini.File
var Gdb, Gcollection, Gnodeid string
var GReportHome bool = true //always report home
var Gttl int64
var Grebalance int

var GStats = struct {
	RateCounter     *ratecounter.RateCounter
	Rate            int64
	Workers         int
	ConsecutiveDead int64
	sync.RWMutex
}{
	ratecounter.NewRateCounter(time.Duration(1) * time.Second),
	0,
	0,
	0,
	sync.RWMutex{},
}

var HA = struct {
	Servers []string
	Timeout time.Duration
	Retries int
	sync.RWMutex
}{
	[]string{""},
	time.Duration(2500) * time.Millisecond,
	10,
	sync.RWMutex{},
}

func GetRandomHaServer(servers []string) (string, int) {
	rand.Seed(time.Now().UnixNano())
	i := rand.Intn(len(servers))
	return strings.TrimSpace(servers[i]), i
}

func GetHaServer(servers []string, i int) string {
	return strings.TrimSpace(servers[i])
}

//health check structure
type Hc struct {
	Id          bson.ObjectId `json:"id" bson:"_id,omitempty"`
	BusyTs      int32         `json:"bts" bson:"bts"`
	Sn          string
	L5          []interface{}
	IntervalSec int32 `json:"ints" bson:"ints"`
	Cron        string
	LastRunTs   int32 `json:"lr" bson:"lr"`
	NextRunTs   int32 `json:"nr" bson:"nr"`
	Hcid        string
	HcType      string `json:"hctype" bson:"hct"`
	Ver         int32  `json:"v" bson:"v"`
	Skip        []string
	Meta        map[string]interface{}
}

type Msg struct {
	SERIAL string
	TS     int64
	ERROR  error
	LOAD   Hc
}

//*****Processor helper function
var GetRequest = func(serial string, load Hc) ([]byte, error) {

	var msg = Msg{
		SERIAL: serial,
		TS:     time.Now().Unix(),
		ERROR:  nil,
		LOAD:   load,
	}

	jsonstr, err := json.Marshal(msg)
	if err != nil {
		return []byte(""), err

	}
	return jsonstr, nil
}

var GetReply = func(JsonMsg []byte) (Msg, error) {
	var msg = Msg{}

	d := json.NewDecoder(bytes.NewReader(JsonMsg))
	d.UseNumber()

	if err := d.Decode(&msg); err != nil {
		return Msg{}, err
	}

	if msg.ERROR != nil {
		return Msg{}, msg.ERROR
	}

	return msg, nil
}

/**
* Creates serial number for request node.int.int:uuid
 */
func MakeSerial(nodeid string, salt int, seed int) string {
	return fmt.Sprintf("%s:%d:%d:%d", Gnodeid, salt, seed, time.Now().UnixNano())
}

//parse command line
func init() {
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		os.Exit(1)
	}

	f, err := os.Open("/dev/urandom")
	if err != nil {
		log.Fatalf("FATAL: opening /dev/urandom %s ", err)
	}
	Random = f
}

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	//parse config
	cfg, err := ini.LoadFile(*Configfile)

	if err != nil {
		log.Fatalf("FATAL: parse config "+*Configfile+" file error: %s", err)
	}

	logfile, ok := cfg.Get("system", "logfile")
	if !ok {
		log.Fatal("FATAL: 'logfile' missing from 'system' section")
	}

	//open log file
	logFile, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("FATAL: %s, %s ", logfile, err)
	}

	defer func() {
		logFile.WriteString(fmt.Sprintf("closing %s", time.UnixDate))
		logFile.Close()
	}()

	log.SetOutput(logFile)

	if *logdebug {
		//see what we have here
		for name, section := range cfg {
			debug(fmt.Sprintf("Section: %v\n", name))
			for k, v := range section {
				debug(fmt.Sprintf("%v: %v\n", k, v))
			}
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
		log.Fatalf("INFO: Interrupt %s", time.Now().String())
	}()

	//get goodies from config file
	//NODE ID
	Gnodeid, ok = cfg.Get("system", "nodeid")
	if !ok {
		log.Fatal("FATAL: 'nodeid' missing from 'system' section")
	}

	//http server
	httphost, ok := cfg.Get("http", "host")
	if !ok {
		log.Fatal("FATAL: 'host' missing from 'http' section")
	}

	httpport, ok := cfg.Get("http", "port")
	if !ok {
		log.Fatal("FATAL: 'port' missing from 'http' section")
	}

	strworkers, ok := cfg.Get("system", "workers")
	if !ok {
		log.Fatal("FATAL: 'workers' missing from 'system' section")
	}

	numworkers, err := strconv.Atoi(strworkers)
	if err != nil {
		log.Fatal("FATAL: 'workers' parameter malformed in 'system' section")
	}

	targetstr, ok := cfg.Get("zmq", "targets")
	if !ok {
		log.Fatal("FATAL: 'targets' missing from 'zmq' section")
	}

	timeoutstr, ok := cfg.Get("zmq", "timeout")
	if !ok {
		log.Fatal("FATAL: 'timeout' missing from 'zmq' section")
	}

	mongos, ok := cfg.Get("mongo", "mongos")
	if !ok {
		log.Fatal("FATAL: 'mongos' missing from 'mongo' section")
	}

	Gdb, ok := cfg.Get("mongo", "db")
	if !ok {
		log.Fatal("FATAL: 'db' missing from 'mongo' section")
	}

	Gcollection, ok := cfg.Get("mongo", "collection")
	if !ok {
		log.Fatal("FATAL: 'collection' missing from 'mongo' section")
	}

	ttl, ok := cfg.Get("mongo", "ttl")
	if !ok {
		log.Fatal("FATAL: 'ttl' missing from 'mongo' section")
	}

	ittl, err := strconv.Atoi(ttl)
	if err != nil {
		log.Fatal("FATAL: 'ttl' parameter malformed in 'mongo' section")
	}

	Gttl = int64(ittl)

	strrebalance, ok := cfg.Get("zmq", "rebalance")
	if !ok {
		log.Fatal("FATAL: 'rebalance' missing from 'zmq' section")
	}

	Grebalance, err = strconv.Atoi(strrebalance)
	if err != nil {
		log.Fatal("FATAL: 'strrebalance' parameter malformed in 'zmq' section")
	}

	reps, ok := cfg.Get("system", "reporthome")
	if ok {
		if repb, err := strconv.ParseBool(reps); err == nil {
			GReportHome = repb
		}
	}

	HA.Servers = strings.Split(targetstr, ",")

	HA.Timeout, err = time.ParseDuration(timeoutstr)
	if err != nil {
		log.Fatal("FATAL: 'timeout' parameter malformed in 'system' section")
	}

	retries, ok := cfg.Get("zmq", "retries")
	if !ok {
		log.Fatal("FATAL: 'retries' missing from 'zmq' section")
	}

	HA.Retries, err = strconv.Atoi(retries)
	if err != nil {
		log.Fatal("FATAL: 'retries' parameter malformed in 'system' section")
	}

	go func() {
		for {
			select {
			case <-time.After(time.Duration(5) * time.Second):
				GStats.Lock()
				GStats.Rate = GStats.RateCounter.Rate()
				GStats.Unlock()
				log.Printf("INFO: rate %d sec, workers %d,  drops %d", GStats.Rate, GStats.Workers, GStats.ConsecutiveDead)
			}
		}
	}()

	//connect to mongo
	MGOsession, err := mgo.Dial(mongos)

	if err != nil {
		log.Fatalf("FATAL: Mongo connection error : %s", err)
	}
	defer MGOsession.Close()

	MGOsession.SetMode(mgo.Strong, true)

	c := MGOsession.DB(Gdb).C(Gcollection)

	// Unique Index
	index := mgo.Index{
		Key:        []string{"hcid"},
		Unique:     true,
		DropDups:   true,
		Background: true,
		Sparse:     true,
	}

	err = c.EnsureIndex(index)
	if err != nil {
		log.Printf("%s", err)
		os.Exit(1)
	}

	// search index
	err = c.EnsureIndexKey("bts", "nr", "skip")
	if err != nil {
		log.Printf("%s", err)
		os.Exit(1)
	}

	// search serial index
	err = c.EnsureIndexKey("sn")
	if err != nil {
		log.Printf("%s", err)
		os.Exit(1)
	}

	//we need to start 2 servers, http for status and zmq
	wg := &sync.WaitGroup{}
	wg.Add(1)
	//first start http interface for self stats
	go func() {

		r := pat.New()
		r.Get("/health", http.HandlerFunc(healthHandle))

		http.Handle("/", r)

		log.Printf("INFO: HTTP Listening %s : %s", httphost, httpport)

		err = http.ListenAndServe(httphost+":"+httpport, nil)
		if err != nil {
			log.Fatalf("FATAL: ListenAndServe: ", err)
		}

		wg.Done()
	}()

	wg.Add(1)

	go func() {

		//start workers
		for i := 1; i <= numworkers; i++ {
			go client(i, MGOsession, Gdb, Gcollection)
			GStats.Lock()
			GStats.Workers++
			GStats.Unlock()
			time.Sleep(time.Duration(10) * time.Millisecond) //dont kill cpu
		}

		wg.Done()

	}()

	wg.Wait()

}

func client(me int, mongoSession *mgo.Session, db string, collection string) {

	context, _ := zmq.NewContext()
	defer context.Close()

	session := mongoSession.Copy()
	defer session.Close()

	session.SetMode(mgo.Strong, true)
	c := session.DB(db).C(collection)

	client, err := context.NewSocket(zmq.REQ)
	if err != nil {
		log.Fatalf("FATAL: can not start worker#%d, %s", me, err)
	}

	homeserver, homeserverindex := GetRandomHaServer(HA.Servers)
	log.Printf("INFO: Home Server %s, %d", homeserver, homeserverindex)

	if GReportHome {
		//otherwise no reason to connect home even
		err = client.Connect(homeserver)
		if err != nil {
			log.Fatalf("FATAL: can not connect worker#%d, %s", me, err)
		}
	}

	NumofRetries := 1

	if HA.Retries == 0 {
		//never timesout
		NumofRetries = 100
	} else {
		NumofRetries = HA.Retries
	}

	sleepTime := 10 //this is minimum to sleep

	serial := "" //serial number for each request

	TargetCnt := len(HA.Servers)

	for sequence, retriesLeft := 1, NumofRetries; retriesLeft > 0; sequence++ {

		if *delay {
			//this is if we set extra delay for debugging
			time.Sleep(time.Duration(1) * time.Second)
		}

		if HA.Retries == 0 {
			//never timeout
			retriesLeft = 100
		}

		if sequence > Grebalance {
			//rewind to prevent overflow, highwater mark is set at 1000 by default
			sequence = 1

			if TargetCnt > 1 && homeserver != HA.Servers[0] {
				homeserverindex = 0
				homeserver = GetHaServer(HA.Servers, homeserverindex)
				log.Printf("INFO: Rebalancing Home Server %s, %d", homeserver, homeserverindex)

				client.SetLinger(0)
				client.Close()

				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("FATAL: can not start worker %s", err)
				}

				err = client.Connect(homeserver)
				if err != nil {
					log.Fatalf("FATAL: can not connect worker %s", err)
				}
			}
		}

		//PRE REQUEST WORKout//
		//Get data serial number for request
		//grab most due , unlocked task for this node
		//lock it

		serial = MakeSerial(Gnodeid, me, sequence)

		query := bson.M{"bts": 0, "nr": bson.M{"$lt": int(time.Now().Unix())}, "skip": bson.M{"$nin": [...]string{Gnodeid}}}

		change := mgo.Change{
			Update:    bson.M{"$set": bson.M{"bts": int(time.Now().Unix()), "lr": int(time.Now().Unix()), "sn": serial}},
			ReturnNew: true,
		}

		healthcheck := Hc{} //Return unlocked object into this

		debug(fmt.Sprintf("query %v", query))

		info, err := c.Find(query).Sort("bts").Apply(change, &healthcheck)
		if err != nil {
			debug(fmt.Sprintf("Result running query %v, %s, %v", info, err, query))
			//more likely no results are found increase sleep time
			if sleepTime < 60000 {
				sleepTime = sleepTime + 100
				debug(fmt.Sprintf("Sleep Time %d msec", sleepTime))
			}
			continue //nothing to send
		}

		if GReportHome != true {
			//we are not sending anything at all
			continue
		}

		//reset sleep on each real work
		sleepTime = 10

		data, err := GetRequest(serial, healthcheck)
		if err != nil {
			log.Printf("WARNING: GetRequest %s", err)
			continue
		}

		debug(fmt.Sprintf("Connected to: %s", homeserver))
		debug(fmt.Sprintf("REQ (%s)", data))

		client.Send(data, 0)

		for expectReply := true; expectReply; {
			//  Poll socket for a reply, with timeout
			items := zmq.PollItems{
				zmq.PollItem{Socket: client, Events: zmq.POLLIN},
			}
			if _, err := zmq.Poll(items, HA.Timeout); err != nil {
				log.Printf("WARNING: REC Timeout:%s ", serial)
				continue
			}

			//  Here we process a server reply. If we didn't a reply we close the client
			//  socket and resend the request.

			if item := items[0]; item.REvents&zmq.POLLIN != 0 {
				//  We got a reply from the server, must match sequence
				reply, err := item.Socket.Recv(0)
				if err != nil {
					log.Printf("WARNING: Receive: %s ", serial)
					continue
				}

				debug("REPLY %s", reply)
				//unpack reply  here
				ReplyMsg, err := GetReply(reply)
				if err != nil {
					log.Printf("WARNING: GetReply:%s : %s", serial, err)
					continue
				}

				if ReplyMsg.SERIAL == serial {

					debug(fmt.Sprintf("OK seq:%s rep:%s", serial, ReplyMsg.SERIAL))
					//ok reply returned see whats going on with it
					//but first unlock the locked hc and set next run time along with result snap

					cronexp, ok := cronexpr.Parse(ReplyMsg.LOAD.Cron)

					nr := int32(time.Now().Unix() + 300)

					now := time.Now()
					next := now

					if ok == nil {
						next = cronexp.Next(now)
						nr = int32(next.Unix())
					}

					debug(fmt.Sprintf("setting time to run: now: %s, cron: %s, next: %s", now, ReplyMsg.LOAD.Cron, next))

					ReplyMsg.LOAD.L5[0] = ReplyMsg.LOAD.L5[1]
					ReplyMsg.LOAD.L5[1] = ReplyMsg.LOAD.L5[2]
					ReplyMsg.LOAD.L5[2] = ReplyMsg.LOAD.L5[3]
					ReplyMsg.LOAD.L5[3] = ReplyMsg.LOAD.L5[4]
					ReplyMsg.LOAD.L5[4] = true

					query := bson.M{"sn": ReplyMsg.SERIAL}

					change := mgo.Change{
						Update:    bson.M{"$set": bson.M{"bts": 0, "nr": nr, "l5": ReplyMsg.LOAD.L5}},
						ReturnNew: true,
					}

					healthcheck := Hc{} //Return query object into this

					debug(fmt.Sprintf("query %v", query))

					info, err := c.Find(query).Apply(change, &healthcheck)
					if err != nil {
						debug(fmt.Sprintf("Result running query %v, %s, %v", info, err, query))
					}

					debug("unlocked %+v", healthcheck)

					//deal with local stats here
					retriesLeft = NumofRetries
					GStats.Lock()
					GStats.ConsecutiveDead = 0
					GStats.RateCounter.Incr(int64(1))
					GStats.Unlock()
					expectReply = false
				} else {
					log.Printf("WARNING: SEQ Mismatch: req: %s rep:%s", serial, ReplyMsg.SERIAL)
					continue
				}
			} else if retriesLeft--; retriesLeft == 0 {
				client.SetLinger(0)
				client.Close()
				GStats.Lock()
				GStats.Workers--
				GStats.Unlock()
				log.Fatal("FATAL:  All Servers are down...bye..")

			} else {

				if HA.Retries == 0 {
					//never timesout
					retriesLeft = 10
				}

				if sequence > Grebalance {
					sequence = 1 //rewind to prevent overflow, highwater mark is set at 1000 by default
				}

				if homeserverindex+1 >= len(HA.Servers) {
					homeserverindex = 0
				} else {
					//try another one
					homeserverindex++
				}

				GStats.Lock()
				GStats.ConsecutiveDead++
				GStats.Unlock()

				homeserver = GetHaServer(HA.Servers, homeserverindex)

				log.Printf("WARNING: failing over to %s", homeserver)
				//  Old socket is confused; close it and open a new one
				client.SetLinger(0)
				client.Close()

				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("FATAL: can not start worker %s", err)
				}

				err = client.Connect(homeserver)
				if err != nil {
					log.Fatalf("FATAL: can not connect worker %s", err)
				}

				log.Println("INFO: Resending...")
				debug(fmt.Sprintf("%s", data))

				//  Send request again, on new socket
				client.Send(data, 0)

			}
		}
	}
}

//Http Handlers
func serve404(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "Not Found")
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

	res["rate"] = fmt.Sprintf("%d", GStats.Rate)
	res["workers"] = fmt.Sprintf("%d", GStats.Workers)
	res["consecutivedead"] = fmt.Sprintf("%d", GStats.ConsecutiveDead)

	b, err := json.Marshal(res)
	if err != nil {
		log.Println("error:", err)
	}

	w.Write(b)
	return
}

//Utilities
//debuggin function dump
func debug(format string, args ...interface{}) {
	if *logdebug {
		if len(args) > 0 {
			log.Printf("DEBUG: "+format, args)
		} else {
			log.Printf("DEBUG: " + format)
		}
	}
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
