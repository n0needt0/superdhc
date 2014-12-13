//Cleaner  daemon based on HA Client
//Andrew Yasinsky

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	zmq "github.com/alecthomas/gozmq"
	"github.com/gorhill/cronexpr"
	"github.com/gorilla/pat"
	logging "github.com/op/go-logging"
	"github.com/paulbellamy/ratecounter"
	"github.com/vaughan0/go-ini"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
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
var logFormat = logging.MustStringFormatter("%{color}%{time:15:04:05.000000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}")
var log = logging.MustGetLogger("logfile")
var Gloglevel logging.Level = logging.DEBUG

//this where errors go to die
var err error

//the followign are flags passed from commandline
var Configfile *string = flag.String("config", "/etc/dhc4/dispatch.cfg", "Config file location")
var help *bool = flag.Bool("help", false, "Show these options")
var cfg ini.File
var Gdb, Gcollection, Gnodeid string
var Gttl int64
var Grebalance int
var Gdebugdelay bool = false //debugdelay
var Ghwm int

var GUpStats = struct {
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

var GDownStats = struct {
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

var UpHA = struct {
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

var DownHA = struct {
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
	Id        bson.ObjectId `json:"id" bson:"_id,omitempty"`
	BusyTs    int32         `json:"bts" bson:"bts"`
	Sn        string
	Cron      string
	LastRunTs int32 `json:"lr" bson:"lr"`
	NextRunTs int32 `json:"nr" bson:"nr"`
	Hcid      string
	HcType    string
	Ver       int32 `json:"v" bson:"v"`
	Meta      map[string]interface{}
}

type Msg struct {
	SERIAL string
	TS     int64
	ERROR  string
	LOAD   Hc
}

//*****Processor helper functionsz
var GetUpRequest = func(serial string, load Hc) ([]byte, error) {
	var msg = Msg{
		SERIAL: serial,
		TS:     time.Now().Unix(),
		ERROR:  "",
		LOAD:   load,
	}

	jsonstr, err := json.Marshal(msg)
	if err != nil {
		return []byte(""), err

	}
	return jsonstr, nil
}

var GetUpReply = func(JsonMsg []byte) (Msg, error, error) {
	var msg = Msg{}

	d := json.NewDecoder(bytes.NewReader(JsonMsg))
	d.UseNumber()

	if err := d.Decode(&msg); err != nil {
		return Msg{}, err, nil
	}

	if msg.ERROR != "" {
		return Msg{}, nil, errors.New(msg.ERROR)
	}

	return msg, nil, nil
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
		log.Fatal("'logfile' missing from 'system' section")
	}

	//open log file
	logFile, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("%s, %s ", logfile, err)
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

	//NODE ID
	Gnodeid, ok = cfg.Get("system", "nodeid")
	if !ok {
		log.Fatal("'nodeid' missing from 'system' section")
	}

	//http server
	httphost, ok := cfg.Get("http", "host")
	if !ok {
		log.Fatal("'host' missing from 'http' section")
	}

	httpport, ok := cfg.Get("http", "port")
	if !ok {
		log.Fatal("'port' missing from 'http' section")
	}

	strworkers, ok := cfg.Get("system", "workers")
	if !ok {
		log.Fatal("'workers' missing from 'system' section")
	}

	numworkers, err := strconv.Atoi(strworkers)
	if err != nil {
		log.Fatal("'workers' parameter malformed in 'system' section")
	}

	uptargetstr, ok := cfg.Get("zmq", "uptargets")
	if !ok {
		log.Fatal("'uptargets' missing from 'zmq' section")
	}

	downtargetstr, ok := cfg.Get("zmq", "downtargets")
	if !ok {
		log.Fatal("'downtargets' missing from 'zmq' section")
	}

	uptimeoutstr, ok := cfg.Get("zmq", "uptimeout")
	if !ok {
		log.Fatal("'uptimeout' missing from 'zmq' section")
	}

	downtimeoutstr, ok := cfg.Get("zmq", "downtimeout")
	if !ok {
		log.Fatal("'downtimeout' missing from 'zmq' section")
	}

	mongos, ok := cfg.Get("mongo", "mongos")
	if !ok {
		log.Fatal("'mongos' missing from 'mongo' section")
	}

	Gdb, ok := cfg.Get("mongo", "db")
	if !ok {
		log.Fatal("'db' missing from 'mongo' section")
	}

	Gcollection, ok := cfg.Get("mongo", "collection")
	if !ok {
		log.Fatal("'collection' missing from 'mongo' section")
	}

	ttl, ok := cfg.Get("mongo", "ttl")
	if !ok {
		log.Fatal("'ttl' missing from 'mongo' section")
	}

	ittl, err := strconv.Atoi(ttl)
	if err != nil {
		log.Fatal("'ttl' parameter malformed in 'mongo' section")
	}

	Gttl = int64(ittl)

	strrebalance, ok := cfg.Get("zmq", "rebalance")
	if !ok {
		log.Fatal("'rebalance' missing from 'zmq' section")
	}

	Grebalance, err = strconv.Atoi(strrebalance)
	if err != nil {
		log.Fatal("'strrebalance' parameter malformed in 'zmq' section")
	}

	ddelay, ok := cfg.Get("system", "debugdelay")
	if ok {
		if ddelayb, err := strconv.ParseBool(ddelay); err == nil {
			Gdebugdelay = ddelayb
		}
	}

	Ghwm = 1000
	hwmstr, ok := cfg.Get("zmq", "hwm")
	if ok {
		hwmi, err := strconv.Atoi(hwmstr)
		if err == nil && hwmi > 1000 {
			Ghwm = hwmi
		}
	}

	Grebalance, err = strconv.Atoi(strrebalance)
	if err != nil {
		log.Fatal("'strrebalance' parameter malformed in 'zmq' section")
	}

	UpHA.Servers = strings.Split(uptargetstr, ",")

	UpHA.Timeout, err = time.ParseDuration(uptimeoutstr)
	if err != nil {
		log.Fatal("'uptimeout' parameter malformed in 'system' section")
	}

	DownHA.Servers = strings.Split(downtargetstr, ",")

	DownHA.Timeout, err = time.ParseDuration(downtimeoutstr)
	if err != nil {
		log.Fatal("'downtimeout' parameter malformed in 'system' section")
	}

	upretries, ok := cfg.Get("zmq", "upretries")
	if !ok {
		log.Fatal("'upretries' missing from 'zmq' section")
	}

	UpHA.Retries, err = strconv.Atoi(upretries)
	if err != nil {
		log.Fatal("'upretries' parameter malformed in 'system' section")
	}

	downretries, ok := cfg.Get("zmq", "downretries")
	if !ok {
		log.Fatal("'downretries' missing from 'zmq' section")
	}

	DownHA.Retries, err = strconv.Atoi(downretries)
	if err != nil {
		log.Fatal("'downretries' parameter malformed in 'system' section")
	}

	go func() {
		for {
			select {
			case <-time.After(time.Duration(5) * time.Second):
				GUpStats.Lock()
				GUpStats.Rate = GUpStats.RateCounter.Rate()
				GUpStats.Unlock()
				log.Notice("UP rate %d sec, workers %d,  drops %d", GUpStats.Rate, GUpStats.Workers, GUpStats.ConsecutiveDead)
				GDownStats.Lock()
				GDownStats.Rate = GDownStats.RateCounter.Rate()
				GDownStats.Unlock()
				log.Notice("rate %d sec, workers %d,  drops %d", GDownStats.Rate, GDownStats.Workers, GDownStats.ConsecutiveDead)
			}
		}
	}()

	//connect to mongo
	MGOsession, err := mgo.Dial(mongos)
	if err != nil {
		log.Fatalf("Mongo connection error : %s", err)
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
		log.Fatalf("%s", err)
	}

	// search index
	err = c.EnsureIndexKey("bts", "nr")
	if err != nil {
		log.Fatalf("%s", err)
	}

	// search serial index
	err = c.EnsureIndexKey("sn")
	if err != nil {
		log.Fatalf("%s", err)
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

		log.Notice("HTTP Listening %s : %s", httphost, httpport)

		err = http.ListenAndServe(httphost+":"+httpport, nil)
		if err != nil {
			log.Fatalf("ListenAndServe: ", err)
		}

		wg.Done()
	}()

	wg.Add(1)

	//start up clients
	go func() {
		for i := 1; i <= numworkers; i++ {
			go upclient(i, MGOsession, Gdb, Gcollection)
			GUpStats.Lock()
			GUpStats.Workers++
			GUpStats.Unlock()
			time.Sleep(time.Duration(10) * time.Millisecond) //dont kill cpu
		}
		wg.Done()
	}()

	wg.Wait()
}

func upclient(me int, mongoSession *mgo.Session, db string, collection string) {

	context, _ := zmq.NewContext()
	defer context.Close()

	session := mongoSession.Copy()
	defer session.Close()

	session.SetMode(mgo.Strong, true)
	c := session.DB(db).C(collection)

	client, err := context.NewSocket(zmq.REQ)
	if err != nil {
		log.Fatalf("can not start upworker#%d, %s", me, err)
	}

	if Ghwm > 1000 {
		client.SetHWM(Ghwm)
	}

	targeturi, targetindex := GetRandomHaServer(UpHA.Servers)
	log.Debug("Up Target %s, %d", targeturi, targetindex)

	//otherwise no reason to connect home even
	err = client.Connect(targeturi)
	if err != nil {
		log.Fatalf("can not connect upworker#%d, %s", me, err)
	}

	NumofRetries := 1

	if UpHA.Retries == 0 {
		//never timesout
		NumofRetries = 100
	} else {
		NumofRetries = UpHA.Retries
	}

	sleepTime := 100 //this is minimum to sleep

	serial := "" //serial number for each request

	TargetCnt := len(UpHA.Servers)

	for sequence, retriesLeft := 1, NumofRetries; retriesLeft > 0; sequence++ {

		if Gdebugdelay {
			//this is if we set extra delay for debugging
			time.Sleep(time.Duration(1) * time.Second)
		}

		if UpHA.Retries == 0 {
			//never timeout
			retriesLeft = 100
		}

		if sequence > Grebalance {

			//rewind to prevent overflow
			sequence = 1

			//also good time to  rebalance to prefered servers
			//prefered server is first in target list.
			//in case of failover this will re try correct server

			if TargetCnt > 1 && targeturi != UpHA.Servers[0] {
				targetindex = 0
				targeturi = GetHaServer(UpHA.Servers, targetindex)
				log.Notice("Rebalancing Up Servers %s, %d", targeturi, targetindex)

				client.SetLinger(0)
				client.Close()

				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("can not start up worker %s", err)
				}

				err = client.Connect(targeturi)
				if err != nil {
					log.Fatalf("can not connect up worker %s", err)
				}
			}
		}

		serial = MakeSerial(Gnodeid, me, sequence)

		//PRE REQUEST WORK//
		query := bson.M{"bts": 0, "nr": bson.M{"$lt": int(time.Now().Unix())}}

		change := mgo.Change{
			Update:    bson.M{"$set": bson.M{"bts": int(time.Now().Unix()), "lr": int(time.Now().Unix()), "sn": serial}},
			ReturnNew: true,
		}

		healthcheck := Hc{} //Return unlocked object into this

		log.Debug("query %v", query)

		info, err := c.Find(query).Sort("bts").Apply(change, &healthcheck)
		if err != nil {
			log.Debug("Result running query %v, %s, %v", info, err, query)
			//more likely no results are found increase sleep time
			if sleepTime < 60000 {
				sleepTime = sleepTime + 100
				log.Debug("Up Sleep Time %d msec", sleepTime)
			}
			continue //nothing to send
		}

		//reset sleep on each real work
		sleepTime = 10

		//Get data for request
		data, err := GetUpRequest(serial, healthcheck)
		if err != nil {
			log.Warning("UP GetRequest Error %s", err)
			continue
		}

		log.Debug("UP Connected to: %s", targeturi)
		log.Debug("UP REQ (%s)", data)

		client.Send(data, 0)

		for expectReply := true; expectReply; {
			//  Poll socket for a reply, with timeout
			items := zmq.PollItems{
				zmq.PollItem{Socket: client, Events: zmq.POLLIN},
			}
			if _, err := zmq.Poll(items, UpHA.Timeout); err != nil {
				log.Warning("UP REC Timeout:%s ", serial)
				continue
			}

			//  Here we process a server reply. If we didn't a reply we close the client
			//  socket and resend the request.

			if item := items[0]; item.REvents&zmq.POLLIN != 0 {
				//  We got a reply from the server, must match sequence
				reply, err := item.Socket.Recv(0)
				if err != nil {
					log.Warning("UP Receive: %s ", serial)
					continue
				}

				log.Debug("%s", reply)

				//unpack reply  here
				ReplyMsg, localerr, remoteerr := GetUpReply(reply)

				if localerr != nil {
					log.Warning("UP GetReply local err:%s : %s", serial, localerr)
					continue
				}

				if remoteerr != nil {
					log.Warning("UP GetReply remote err:%s : %s", serial, remoteerr)
					continue
				}

				if ReplyMsg.SERIAL == serial {
					log.Debug("UP OK seq:%s rep:%s", serial, ReplyMsg.SERIAL)
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

					log.Debug("setting time to run: now: %s, cron: %s, next: %s", now, ReplyMsg.LOAD.Cron, next)

					query := bson.M{"sn": ReplyMsg.SERIAL}
					change := mgo.Change{
						Update:    bson.M{"$set": bson.M{"bts": 0, "nr": nr}},
						ReturnNew: true,
					}

					healthcheck := Hc{} //Return query object into this

					log.Debug("query %v", query)

					info, err := c.Find(query).Apply(change, &healthcheck)
					if err != nil {
						log.Debug("Result running query %v, %s, %v", info, err, query)
					}

					log.Debug("Up unlocked %+v", healthcheck)
					//
					//submit report here

					//deal with local stats here
					retriesLeft = NumofRetries
					GUpStats.Lock()
					GUpStats.ConsecutiveDead = 0
					GUpStats.RateCounter.Incr(int64(1))
					GUpStats.Unlock()
					expectReply = false
				} else {
					log.Warning("UP SEQ Mismatch: req: %s rep:%s", serial, ReplyMsg.SERIAL)
					continue
				}
			} else if retriesLeft--; retriesLeft == 0 {
				client.SetLinger(0)
				client.Close()
				log.Fatal(" All Servers are down...bye..")

			} else {

				if UpHA.Retries == 0 {
					//never timesout
					retriesLeft = 100
				}

				if sequence > Grebalance {
					sequence = 1 //rewind to prevent overflow
				}

				if targetindex+1 >= len(UpHA.Servers) {
					targetindex = 0
				} else {
					//try next one
					targetindex++
				}

				GUpStats.Lock()
				GUpStats.ConsecutiveDead++
				GUpStats.Unlock()

				targeturi = GetHaServer(UpHA.Servers, targetindex)

				log.Warning("UP failing over to %s", targeturi)
				//  Old socket is confused; close it and open a new one
				client.SetLinger(0)
				client.Close()

				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("can not start up worker %s", err)
				}

				err = client.Connect(targeturi)
				if err != nil {
					log.Fatalf("can not connect up worker %s", err)
				}

				log.Notice("UP Resending...")
				log.Debug("%s", data)

				//  Send request again, on new socket
				client.Send(data, 0)
			}
		}
	}
}

func downclient(me int, mongoSession *mgo.Session, db string, collection string) {

	context, _ := zmq.NewContext()
	defer context.Close()

	session := mongoSession.Copy()
	defer session.Close()

	session.SetMode(mgo.Strong, true)
	c := session.DB(db).C(collection)

	client, err := context.NewSocket(zmq.REQ)
	if err != nil {
		log.Fatalf("can not start downworker#%d, %s", me, err)
	}

	if Ghwm > 1000 {
		client.SetHWM(Ghwm)
	}

	targeturi, targetindex := GetRandomHaServer(DownHA.Servers)
	log.Debug("Down target %s, %d", targeturi, targetindex)

	err = client.Connect(targeturi)
	if err != nil {
		log.Fatalf("can not connect down worker#%d, %s", me, err)
	}

	NumofRetries := 1

	if DownHA.Retries == 0 {
		//never timesout
		NumofRetries = 100
	} else {
		NumofRetries = DownHA.Retries
	}

	sleepTime := 100 //this is minimum to sleep

	serial := "" //serial number for each request

	TargetCnt := len(DownHA.Servers)

	for sequence, retriesLeft := 1, NumofRetries; retriesLeft > 0; sequence++ {

		if Gdebugdelay {
			//this is if we set extra delay for debugging
			time.Sleep(time.Duration(1) * time.Second)
		}

		if DownHA.Retries == 0 {
			//never timeout
			retriesLeft = 100
		}

		if sequence > Grebalance {

			//rewind to prevent overflow
			sequence = 1

			//also good time to  rebalance to prefered servers
			//prefered server is first in [zmq] target list.
			//in case of failoverwill re try correct server

			if TargetCnt > 1 && targeturi != DownHA.Servers[0] {
				targetindex = 0
				targeturi = GetHaServer(DownHA.Servers, targetindex)
				log.Notice("Rebalancing Down Target Server %s, %d", targeturi, targetindex)

				client.SetLinger(0)
				client.Close()

				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("can not start down worker %s", err)
				}

				err = client.Connect(targeturi)
				if err != nil {
					log.Fatalf("can not connect downworker %s", err)
				}

			}
		}

		//TODOHERE

		serial = MakeSerial(Gnodeid, me, sequence)

		//PRE REQUEST WORK//
		query := bson.M{"bts": 0, "nr": bson.M{"$lt": int(time.Now().Unix())}}

		change := mgo.Change{
			Update:    bson.M{"$set": bson.M{"bts": int(time.Now().Unix()), "lr": int(time.Now().Unix()), "sn": serial}},
			ReturnNew: true,
		}

		healthcheck := Hc{} //Return unlocked object into this

		log.Debug("query %v", query)

		info, err := c.Find(query).Sort("bts").Apply(change, &healthcheck)
		if err != nil {
			log.Debug("Result running query %v, %s, %v", info, err, query)
			//more likely no results are found increase sleep time
			if sleepTime < 60000 {
				sleepTime = sleepTime + 100
				log.Debug("Sleep Time %d msec", sleepTime)
			}
			continue //nothing to send
		}

		//reset sleep on each real work
		sleepTime = 10

		//Get data for request
		data, err := GetDownRequest(serial, healthcheck)
		if err != nil {
			log.Warning("Down GetRequest %s", err)
			continue
		}

		log.Debug("Down Connected to: %s", targeturi)
		log.Debug("Down REQ (%s)", data)

		client.Send(data, 0)

		for expectReply := true; expectReply; {
			//  Poll socket for a reply, with timeout
			items := zmq.PollItems{
				zmq.PollItem{Socket: client, Events: zmq.POLLIN},
			}
			if _, err := zmq.Poll(items, UpHA.Timeout); err != nil {
				log.Warning("Down REC Timeout:%s ", serial)
				continue
			}

			//  Here we process a server reply. If we didn't a reply we close the client
			//  socket and resend the request.

			if item := items[0]; item.REvents&zmq.POLLIN != 0 {
				//  We got a reply from the server, must match sequence
				reply, err := item.Socket.Recv(0)
				if err != nil {
					log.Warning("Down Receive: %s ", serial)
					continue
				}

				log.Debug("Down reply %s", reply)

				//unpack reply  here
				ReplyMsg, localerr, remoteerr := GetReply(reply)

				if localerr != nil {
					log.Warning("GetDownReply local err:%s : %s", serial, localerr)
					continue
				}

				if remoteerr != nil {
					log.Warning("GetDownReply remote err:%s : %s", serial, remoteerr)
					continue
				}

				if ReplyMsg.SERIAL == serial {
					log.Debug("Down OK seq:%s rep:%s", serial, ReplyMsg.SERIAL)
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

					log.Debug("setting time to run: now: %s, cron: %s, next: %s", now, ReplyMsg.LOAD.Cron, next)

					query := bson.M{"sn": ReplyMsg.SERIAL}

					change := mgo.Change{
						Update:    bson.M{"$set": bson.M{"bts": 0, "nr": nr}},
						ReturnNew: true,
					}

					healthcheck := Hc{} //Return query object into this

					log.Debug("query %v", query)

					info, err := c.Find(query).Apply(change, &healthcheck)
					if err != nil {
						log.Debug("Result running query %v, %s, %v", info, err, query)
					}

					log.Debug("unlocked %+v", healthcheck)

					//deal with local stats here
					retriesLeft = NumofRetries
					GDownStats.Lock()
					GDownStats.ConsecutiveDead = 0
					GDownStats.RateCounter.Incr(int64(1))
					GDownStats.Unlock()
					expectReply = false
				} else {
					log.Warning("Down SEQ Mismatch: req: %s rep:%s", serial, ReplyMsg.SERIAL)
					continue
				}
			} else if retriesLeft--; retriesLeft == 0 {
				client.SetLinger(0)
				client.Close()
				log.Fatal("All Down Servers are down...bye..")

			} else {

				if DownHA.Retries == 0 {
					//never timesout
					retriesLeft = 10
				}

				if sequence > Grebalance {
					sequence = 1 //rewind to prevent overflow, highwater mark is set at 1000 by default
				}

				if targetindex+1 >= len(DownHA.Servers) {
					targetindex = 0
				} else {
					//try another one
					targetindex++
				}

				GDownStats.Lock()
				GDownStats.ConsecutiveDead++
				GDownStats.Unlock()

				targeturi = GetHaServer(UpHA.Servers, targetindex)

				log.Warning("failing over to %s", targeturi)
				//  Old socket is confused; close it and open a new one
				client.SetLinger(0)
				client.Close()

				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("can not start worker %s", err)
				}

				err = client.Connect(targeturi)
				if err != nil {
					log.Fatalf("can not connect worker %s", err)
				}

				log.Notice("Resending...")
				log.Debug("%s", data)

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

	Gloglevel, err = logging.LogLevel(loglevel)
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

	res["rate"] = fmt.Sprintf("%d", GStats.Rate)
	res["workers"] = fmt.Sprintf("%d", GStats.Workers)
	res["consecutivedead"] = fmt.Sprintf("%d", GStats.ConsecutiveDead)

	b, err := json.Marshal(res)
	if err != nil {
		log.Error("error: %s", err)
	}

	w.Write(b)
	return
}

//Utilities

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
