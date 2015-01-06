//Dispatch daemon
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

// _HC  healthcheck def db
// _HIST healthcheck n history
//_EVENT healthcheck event

const (
	HEALTH_UP       = true
	HEALTH_DOWN     = false
	DB_HC           = "hc"
	COL_HC          = "hc"
	DB_HIST         = "hist"
	COL_HIST        = "hist"
	DB_EVENT        = "event"
	COL_EVENT       = "event"
	EXPIRESEC_HIST  = 86400 // maximum time for history to live on this node 24hr
	EXPIRECNT_HIST  = 10    //maximum number of history records for given health check id
	FORCESEC_REPORT = 86400 //report down event if still down and older than
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
var Gnodeid string
var Grebalance int
var Gdebugdelay bool = false //debugdelay
var Ghwm int = 1000          //highwatermark

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

type HcResult struct {
	ID        bson.ObjectId `bson:"_id,omitempty"`
	Result    bool          `json:"r" bson:"r"`
	Hcid      string
	HcType    string
	Sn        string
	LastRunTs int32 `json:"lr" bson:"lr"`
	Meta      map[string]interface{}
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
}

type HcEvent struct {
	ID         bson.ObjectId `bson:"_id,omitempty"`
	SERIAL     string
	Result     bool
	Hcid       string
	AckSerial  string
	ReportedAt time.Time `json:"reportedAt" bson:"reportedAt"`
}

type Msg struct {
	SERIAL string
	TS     int64
	ERROR  string
	HC     Hc
	RESULT map[string]interface{}
}

//format request
var GetRequest = func(serial string, hc Hc) ([]byte, error) {
	var msg = Msg{
		SERIAL: serial,
		TS:     time.Now().Unix(),
		ERROR:  "",
		HC:     hc,
		RESULT: make(map[string]interface{}),
	}

	jsonstr, err := json.Marshal(msg)
	if err != nil {
		return []byte(""), err

	}
	return jsonstr, nil
}

//process reply
var GetReply = func(JsonMsg []byte) (Msg, error, error) {
	var msg = Msg{}

	d := json.NewDecoder(bytes.NewReader(JsonMsg))
	d.UseNumber()

	if err := d.Decode(&msg); err != nil {
		//can not decode it. let cleaner process deal with it
		return Msg{}, err, nil
	}

	if msg.ERROR != "" {
		//decoded it but something is reported from far
		return msg, nil, errors.New(msg.ERROR)
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

func lookup_collection(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

//create various indexes
func indexmongo(mongoSession *mgo.Session) {
	session := mongoSession.Copy()
	defer session.Close()

	session.SetMode(mgo.Strong, true)
	c := session.DB(DB_HC).C(COL_HC)

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

	//HISTORY DB
	c = session.DB(DB_HIST).C(COL_HIST)
	// search index
	err = c.EnsureIndexKey("hcid")
	if err != nil {
		log.Fatalf("%s", err)
	}

	//expire after
	index = mgo.Index{
		Key:         []string{"createdAt"},
		ExpireAfter: time.Duration(EXPIRESEC_HIST) * time.Second,
	}

	err = c.DropIndex("createdAt")
	if err != nil {
		log.Warning("%s", err)
	}

	err = c.EnsureIndex(index)
	if err != nil {
		log.Fatalf("%s", err)
	}

	c = session.DB(DB_EVENT).C(COL_EVENT)

	// Unique Index
	index = mgo.Index{
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

	timeoutstr, ok := cfg.Get("zmq", "timeout")
	if !ok {
		log.Fatal("'timeout' missing from 'zmq' section")
	}

	mongos, ok := cfg.Get("mongo", "mongos")
	if !ok {
		log.Fatal("'mongos' missing from 'mongo' section")
	}

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

	targetstr, ok := cfg.Get("zmq", "targets")
	if !ok {
		log.Fatal("'targets' missing from 'zmq' section")
	}

	HA.Servers = strings.Split(targetstr, ",")

	HA.Timeout, err = time.ParseDuration(timeoutstr)
	if err != nil {
		log.Fatal("'timeout' parameter malformed in 'system' section")
	}

	retries, ok := cfg.Get("zmq", "retries")
	if !ok {
		log.Fatal("'retries' missing from 'zmq' section")
	}

	HA.Retries, err = strconv.Atoi(retries)
	if err != nil {
		log.Fatal("'retries' parameter malformed in 'system' section")
	}

	//connect to mongo to build some indexes
	MGOsession, err := mgo.Dial(mongos)
	if err != nil {
		log.Fatalf("Mongo connection error : %s", err)
	}
	defer MGOsession.Close()

	indexmongo(MGOsession)

	go func() {
		for {
			select {
			case <-time.After(time.Duration(5) * time.Second):
				GStats.Lock()
				GStats.Rate = GStats.RateCounter.Rate()
				GStats.Unlock()
				log.Notice("rate %d sec, wrkrs %d,  drops %d", GStats.Rate, GStats.Workers, GStats.ConsecutiveDead)
			}
		}
	}()

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

	go func() {
		//start workers
		for i := 1; i <= numworkers; i++ {
			go client(i, MGOsession)
			GStats.Lock()
			GStats.Workers++
			GStats.Unlock()
			time.Sleep(time.Duration(100) * time.Millisecond) //dont kill cpu
		}
		wg.Done()
	}()
	wg.Wait()
}

func client(me int, mongoSession *mgo.Session) {

	context, _ := zmq.NewContext()
	defer context.Close()

	session := mongoSession.Copy()
	defer session.Close()

	session.SetMode(mgo.Strong, true)

	hc_conn := session.DB(DB_HC).C(COL_HC)
	hist_conn := session.DB(DB_HIST).C(COL_HIST)
	event_conn := session.DB(DB_EVENT).C(COL_EVENT)

	client, err := context.NewSocket(zmq.REQ)
	if err != nil {
		log.Fatalf("can not start worker#%d, %s", me, err)
	}

	//set highwatermark
	if Ghwm > 1000 {
		client.SetHWM(Ghwm)
	}

	homeserver, homeserverindex := GetRandomHaServer(HA.Servers)
	log.Debug("Home Server %s, %d", homeserver, homeserverindex)

	err = client.Connect(homeserver)
	if err != nil {
		log.Fatalf("can not connect worker#%d, %s", me, err)
	}

	//maximum retries per serial number before drop it to the floor and close socket
	NumofRetries := 5

	if HA.Retries > 0 && HA.Retries < 5 {
		NumofRetries = HA.Retries
	}

	sleepTime := 100 //this is minimum to sleep if nothing to do

	serial := "" //serial number for each request

	TargetCnt := len(HA.Servers)
	sequence := 0

	for {
		sequence++

		session.Refresh()

		if Gdebugdelay {
			//this is if we set extra delay for debugging
			time.Sleep(time.Duration(1) * time.Second)
		}

		if sequence > Grebalance {

			//rewind to prevent overflow
			sequence = 1
			//also good time to  rebalance to prefered servers
			//prefered server is first in [zmq] target list.
			//in case of failover even this will re try correct server

			if TargetCnt > 1 && homeserver != HA.Servers[0] {
				homeserverindex = 0
				homeserver = GetHaServer(HA.Servers, homeserverindex)
				log.Notice("Rebalancing Home Server %s, %d", homeserver, homeserverindex)

				client.SetLinger(0)
				client.Close()

				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("can not start worker %s", err)
				}

				err = client.Connect(homeserver)
				if err != nil {
					log.Fatalf("can not connect worker %s", err)
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

		info, err := hc_conn.Find(query).Sort("bts").Apply(change, &healthcheck)
		if err != nil {
			log.Debug("Query result %v, %s, %v", info, err, query)
			//more likely no results are found increase sleep time
			if sleepTime < 60000 {
				sleepTime = sleepTime + 100
				log.Debug("Sleep Time %d msec", sleepTime)
			}
			continue //nothing to send
		}

		//wake up we found work
		sleepTime = 100

		//Get data for request
		data, err := GetRequest(serial, healthcheck)
		if err != nil {
			log.Warning("Error GetRequest %s", err)
			continue
		}

		log.Debug("Connected to: %s", homeserver)
		log.Debug("REQUEST (%s)", data)

		client.Send(data, 0)

		retriesLeft := NumofRetries
		expectReply := true

		for expectReply {

			//  Poll socket for a reply, with timeout
			items := zmq.PollItems{
				zmq.PollItem{Socket: client, Events: zmq.POLLIN},
			}
			if _, err := zmq.Poll(items, HA.Timeout); err != nil {
				log.Warning("REQUEST Timeout:%s ", serial)
				expectReply = false
			}

			//  Here we process a server reply. If we didn't a reply we close the client
			//  socket and resend the request.

			if item := items[0]; item.REvents&zmq.POLLIN != 0 {
				//  We got a reply from the server, must match sequence
				reply, err := item.Socket.Recv(0)
				if err != nil {
					log.Warning("REQUEST: %s ", serial)
					expectReply = false
				}

				log.Debug("REPLY %s", reply)
				//unpack reply  here
				ReplyMsg, localerr, remoteerr := GetReply(reply)

				if localerr != nil {
					log.Warning("GetReply local err:%s : %s", serial, localerr)
					expectReply = false
				}

				if remoteerr != nil {
					log.Warning("GetReply remote err:%s : %s", serial, remoteerr)
				}

				if ReplyMsg.SERIAL == serial {
					log.Debug("OK seq:%s rep:%s", serial, ReplyMsg.SERIAL)
					//ok reply returned see whats going on with it
					//but first unlock the locked hc and set next run time along with result snap
					cronexp, ok := cronexpr.Parse(ReplyMsg.HC.Cron)

					now := time.Now()
					next := now

					nr := int32(now.Unix() + 300) //worst case run every 5 minutes if cron not compiles

					if ok == nil {
						next = cronexp.Next(now)
						//add few random seconds to spread load
						rand.Seed(time.Now().UnixNano())
						nr = int32(next.Unix()) + int32(rand.Intn(30))
					}

					log.Debug("setting next run in: %d sec per cron: %s", nr-int32(now.Unix()), ReplyMsg.HC.Cron)

					query := bson.M{"sn": ReplyMsg.SERIAL}

					change := mgo.Change{
						Update:    bson.M{"$set": bson.M{"bts": 0, "nr": nr}},
						ReturnNew: true,
					}

					unlock := Hc{} //Return query object into this

					log.Debug("query %v", query)

					info, err := hc_conn.Find(query).Apply(change, &unlock)
					if err != nil {
						log.Debug("Query res %v, %s, %v", info, err, query)
					}

					log.Debug("unlocked %+v", unlock)

					state := HEALTH_DOWN

					if res, ok := ReplyMsg.RESULT["state"]; ok {
						s := fmt.Sprintf("%s", res)
						if s == "1" {
							state = HEALTH_UP
							log.Debug("intstate %s ", s)
						}

					}

					result := &HcResult{
						Result:    state,
						Hcid:      ReplyMsg.HC.Hcid,
						HcType:    ReplyMsg.HC.HcType,
						Sn:        ReplyMsg.SERIAL,
						LastRunTs: ReplyMsg.HC.LastRunTs,
						Meta:      ReplyMsg.RESULT,
						CreatedAt: time.Now(),
					}

					err = hist_conn.Insert(&result)
					if err != nil {
						log.Warning("error inserting %s", err)
					}

					//so far record is unlocked for next run we need to see if up/down event had in fact happened
					//this requires running over last N results, and randomly trimming history for given hcid
					var results []HcResult

					query = bson.M{"hcid": ReplyMsg.HC.Hcid}

					err = hist_conn.Find(query).Sort("-timestamp").All(&results)
					if err != nil {
						log.Debug("Query res %v, %s, %v", info, err, query)
					}

					//these are run time evaluation of flap and status
					up, down := 0, 0
					flapping := false
					rlock := false //this flag stops evaluation yet allows for history cleanup

					//default test settings
					tup, tdown := 1, 1

					if sup, ok := unlock.Meta["rup"]; ok == true {
						iup, err := strconv.Atoi(sup.(string))
						if err == nil && iup < EXPIRECNT_HIST {
							tup = iup
						}
					}

					if sdown, ok := unlock.Meta["rdown"]; ok == true {
						idown, err := strconv.Atoi(sdown.(string))
						if err == nil && idown < EXPIRECNT_HIST {
							tdown = idown
						}
					}

					teststr := ""

					for k, v := range results {

						//we have enough to fire event
						if v.Result == true && !rlock {
							teststr = fmt.Sprintf("%s%s", teststr, "1")
							up++
							if down > 0 {
								flapping = true
								rlock = true
								teststr = fmt.Sprintf("%s%s", teststr, "F")
								log.Debug("EVAL flopping up %s", v.Hcid)
							}
							if up > tup {
								rlock = true
								teststr = fmt.Sprintf("%s%s", teststr, "U")
								log.Debug("EVAL enough up %s", v.Hcid)
							}
						} else if v.Result == false && !rlock {
							teststr = fmt.Sprintf("%s%s", teststr, "0")
							down++
							if up > 0 {
								flapping = true
								rlock = true
								teststr = fmt.Sprintf("%s%s", teststr, "F")
								log.Debug("EVAL flopping down %s", v.Hcid)
							}
							if down > tdown {
								rlock = true
								teststr = fmt.Sprintf("%s%s", teststr, "D")
								log.Debug("EVAL enough down %s", v.Hcid)
							}
						} else {
							if v.Result == true {
								teststr = fmt.Sprintf("%s%s", teststr, "1")
							} else {
								teststr = fmt.Sprintf("%s%s", teststr, "0")
							}
						}

						//remove old records
						if k > EXPIRECNT_HIST {
							log.Debug("CLEAN max cnt %d id %s", k, v.ID.String())

							err = hist_conn.RemoveId(v.ID)
							if err != nil {
								log.Warning("CLEAN Can't remove %+v", v.ID.String())
							}
						}
					}

					log.Debug("JUDGE %s (up:%d tup:%d), (dn:%d, tdn:%d)", teststr, up, tup, down, tdown)

					if flapping {
						log.Debug("JUDGE NO flapping %s", ReplyMsg.HC.Hcid)
					} else {
						if up > 0 && up < tup {
							log.Debug("JUDGE NO %s report up, not enough samples have: %d, need %d ", ReplyMsg.HC.Hcid, up, tup)
						}

						if down > 0 && down < tdown {
							log.Debug("JUDGE NO %s report down, not enough samples have: %d, need %d ", ReplyMsg.HC.Hcid, down, tdown)
						}

						eventresult := HcEvent{}
						state := HEALTH_DOWN

						if up > 0 && up >= tup {
							//node up
							log.Debug("JUDGE YES %s pre report up", ReplyMsg.HC.Hcid)
							//check report history
							state = HEALTH_UP

						} else {

							if down > 0 && down >= tdown {
								//node down
								log.Debug("JUDGE YES %s pre report down", ReplyMsg.HC.Hcid)
								state = HEALTH_DOWN
							}
						}

						query = bson.M{"hcid": ReplyMsg.HC.Hcid}

						err = event_conn.Find(query).One(&eventresult)
						if err != nil {
							log.Debug("JUDGE find EVENT %v, %s, %v, %b", info, err, query, state)
						}

						//log.Debug("JUDGE %+v", eventresult)

						//if there are no result or
						//it is different or
						//it is down for more than 24 hr
						//or if no ack is set
						judgedo := false

						if eventresult.SERIAL == "" {
							log.Notice("JUDGE DO new %s, now: %t", ReplyMsg.HC.Hcid, state)
							judgedo = true
						} else if eventresult.Result != state {
							log.Notice("JUDGE DO change %s, was %t, now: %t", ReplyMsg.HC.Hcid, eventresult.Result, state)
							judgedo = true
						} else if eventresult.AckSerial == "" {
							log.Notice("JUDGE DO ack %s, was %t, now: %t", ReplyMsg.HC.Hcid, eventresult.Result, state)
							judgedo = true
						} else if eventresult.Result == HEALTH_DOWN && int32(time.Since(eventresult.ReportedAt).Seconds()) > FORCESEC_REPORT {
							//report and log
							log.Notice("JUDGE DO down %s, over %d sec, was %t, now: %t", ReplyMsg.HC.Hcid, FORCESEC_REPORT, eventresult.Result, state)
							judgedo = true
						} else {
							log.Notice("JUDGE NO change %s", ReplyMsg.HC.Hcid)
						}

						if judgedo {
							//send event and update db once ack is received
							//TODO WRAP INTO ZMQ CALL

							eventresult.SERIAL = ReplyMsg.SERIAL
							eventresult.AckSerial = ReplyMsg.SERIAL
							eventresult.Hcid = ReplyMsg.HC.Hcid
							eventresult.Result = state
							eventresult.ReportedAt = time.Now()

							query = bson.M{"hcid": ReplyMsg.HC.Hcid}

							_, err = event_conn.Upsert(query, eventresult)
							if err != nil {
								log.Debug("JUDGE ADD EVENT res %v, %s, %v, %b", info, err, query, state)
							}
						}

					}

					//deal with local stats here
					retriesLeft = NumofRetries
					GStats.Lock()
					GStats.ConsecutiveDead = 0
					GStats.RateCounter.Incr(int64(1))
					GStats.Unlock()
					expectReply = false
				} else {
					log.Warning("SEQ Mismatch: req: %s rep:%s", serial, ReplyMsg.SERIAL)
					expectReply = false
				}
			} else {

				//this will re send the message

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
				log.Warning("%d trying %s", NumofRetries-retriesLeft+1, homeserver)
				//  Old socket is confused; close it and open a new one
				client.SetLinger(0)
				client.Close()
				client, err = context.NewSocket(zmq.REQ)
				if err != nil {
					log.Fatalf("can not start worker %s", err)
				}
				err = client.Connect(homeserver)
				if err != nil {
					log.Fatalf("can not connect worker %s", err)
				}

				if retriesLeft < 1 {
					log.Warning("Dropping ...")
					log.Debug("%s", data)

					//TODO
					//somehow start throthling here
					expectReply = false
				} else {

					log.Notice("Resending...")
					log.Debug("%s", data)
					//  Send request again, on new socket
					client.Send(data, 0)
					retriesLeft--
				}
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
