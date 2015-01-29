//Cleaner  daemon based on HA Client
//Andrew Yasinsky

package main

import (
	dhc4 "./dhc4"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/gorilla/pat"
	logging "github.com/op/go-logging"
	"github.com/vaughan0/go-ini"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DB_HC              = "hc"
	COL_HC             = "hc"
	DB_HIST            = "hist"
	COL_HIST           = "hist"
	DB_EVENT           = "event"
	COL_EVENT          = "event"
	DB_SYS             = "sys"
	COL_SYS            = "sys"
	EXPIRESEC_HIST     = 60
	MIN_SLEEP_SEC      = 1
	MAX_SLEEP_SEC      = 600
	RUN_EVERY_SEC      = 60
	MAX_STALE_LOCK_SEC = 61
)

type FeederLock struct {
	BusyTs    int32  `json:"bts" bson:"bts"`
	Sn        string `json:"sn" bson:"sn"`
	LastRunTs int32  `json:"lr" bson:"lr"`
}

//this is log file
var logFile *os.File
var logFormat = logging.MustStringFormatter("%{color}%{time:15:04:05.000000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}")
var log = logging.MustGetLogger("cleaner")
var Gloglevel logging.Level = logging.DEBUG

//this where errors go to die
var err error

//the followign are flags passed from commandline
var Configfile *string = flag.String("config", "/etc/dhc4/feeder.cfg", "Config file location default /etc/dhc4/feeder.cfg")
var help *bool = flag.Bool("help", false, "Show these options")
var cfg ini.File
var Gdb, Gcollection, Gnodeid, Gstats string
var Gdebugdelay bool = false //debugdelay
var Glocation string = dhc4.UNKNOWN

//health check structure
type Hc struct {
	Hcid    int32                  `json:"hc_id"`
	HcType  string                 `json:"class"`
	Meta    map[string]interface{} `json:"config"`
	Updated int32                  `json:"updated"`
}

//parse command line
func init() {
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		os.Exit(1)
	}
}

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

	c = session.DB(DB_SYS).C(COL_SYS)
	// Unique Index
	index = mgo.Index{
		Key:        []string{"sys"},
		Unique:     true,
		DropDups:   true,
		Background: true,
		Sparse:     true,
	}

	err = c.EnsureIndex(index)
	if err != nil {
		log.Fatalf("%s", err)
	}

	//insert lock
	err = c.Insert(bson.M{"sys": "feederlock", "bts": 0, "lr": 0, "sn": ""})
	if err != nil {
		log.Warning("error inserting %s", err)
	}

}

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	//parse config
	cfg, err = ini.LoadFile(*Configfile)

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

	//http server
	httphost, ok := cfg.Get("http", "host")
	if !ok {
		log.Fatal("'host' missing from 'http' section")
	}

	httpport, ok := cfg.Get("http", "port")
	if !ok {
		log.Fatal("'port' missing from 'http' section")
	}

	//NODE ID
	Gnodeid, ok = cfg.Get("system", "nodeid")
	if !ok {
		log.Fatal("'nodeid' missing from 'system' section")
	}

	//LOCATION
	Glocation, ok = cfg.Get("system", "location")
	if !ok {
		log.Fatal("'location' missing from 'system' section")
	}

	apiurl, ok := cfg.Get("api", "url")
	if !ok {
		log.Fatal("'url' missing from 'api' section")
	}

	apiuser, ok := cfg.Get("api", "user")
	if !ok {
		log.Fatal("'user' missing from 'api' section")
	}

	apipassword, ok := cfg.Get("api", "password")
	if !ok {
		log.Fatal("'password' missing from 'api' section")
	}

	mongos, ok := cfg.Get("mongo", "mongos")
	if !ok {
		log.Fatal("'mongos' missing from 'mongo' section")
	}

	ddelay, ok := cfg.Get("system", "debugdelay")
	if ok {
		if ddelayb, err := strconv.ParseBool(ddelay); err == nil {
			Gdebugdelay = ddelayb
		}
	}

	//start systems

	wg := &sync.WaitGroup{}
	wg.Add(1)
	//first start http interface for self stats
	go func() {

		r := pat.New()
		r.Get("/health", http.HandlerFunc(healthHandle))
		r.Get("/loglevel/{loglevel}", http.HandlerFunc(logHandle))

		http.Handle("/", r)

		log.Notice("HTTP Listening %s : %s", httphost, httpport)

		err = http.ListenAndServe(httphost+":"+httpport, nil)
		if err != nil {
			log.Fatalf("ListenAndServe: ", err)
		}

		wg.Done()
	}()

	//connect to mongo
	MGOsession, err := mgo.Dial(mongos)

	if err != nil {
		log.Fatalf("Mongo connection error : %s", err)
	}
	defer MGOsession.Close()

	MGOsession.SetMode(mgo.Strong, true)
	hc_conn := MGOsession.DB(DB_HC).C(COL_HC)

	sys_conn := MGOsession.DB(DB_SYS).C(COL_SYS)

	indexmongo(MGOsession)

	//some times you need to sleep on errors
	//this is how we do it
	sleepTimeSec := MIN_SLEEP_SEC //at least

	//create lock so only one process can do this at the time
	//this is query we will try to execute
	//however it will run once every 10 minutes no matter what

	for {

		query := bson.M{"sys": "feederlock", "bts": 0, "lr": bson.M{"$lt": int(time.Now().Unix()) - MAX_STALE_LOCK_SEC}}

		//this is info we will try to lock with
		change := mgo.Change{
			Update:    bson.M{"$set": bson.M{"bts": int(time.Now().Unix()), "lr": int(time.Now().Unix()), "sn": Gnodeid}},
			ReturnNew: true,
		}

		log.Debug("running query %+v", query)

		log.Debug("with change %+v", change)

		feederlock := FeederLock{} //Return unlocked object into this

		log.Debug("query %v", query)

		info, err := sys_conn.Find(query).Sort("bts").Apply(change, &feederlock)
		if err != nil {
			log.Debug("Query result %v, %s, %v", info, err, query)
			//more likely no results are found increase sleep time
			if sleepTimeSec < MAX_SLEEP_SEC {
				sleepTimeSec++
				log.Debug("Sleep Time %d sec", sleepTimeSec)
			}
			time.Sleep(time.Duration(sleepTimeSec) * time.Second)
			continue //nothing to send
		}

		log.Debug("Feeded Locked %s", Gnodeid)

		//reset it right back
		sleepTimeSec = MIN_SLEEP_SEC

		//the following is used to diff existing and new data for deletes
		existing := make(map[string]bool)

		var AllRec []map[string]string
		rec_updated := 0
		rec_inserted := 0
		rec_deleted := 0

		//find all ids of existing hcids

		err = hc_conn.Find(bson.M{}).Select(bson.M{"_id": 0, "hcid": 1}).All(&AllRec)
		if err != nil {
			log.Debug("Query result %s", err)
		}

		for _, rec := range AllRec {
			if v, ok := rec["hcid"]; ok {
				existing[v] = true
			}
		}

		//release
		AllRec = nil

		//get url content first
		client := &http.Client{}
		req, err := http.NewRequest("GET", apiurl, nil)
		req.SetBasicAuth(apiuser, apipassword)
		response, err := client.Do(req)
		if err != nil {
			log.Warning(fmt.Sprintf("%s", err))
		} else {
			defer response.Body.Close()
			contents, err := ioutil.ReadAll(response.Body)

			if err != nil || response.StatusCode != 200 {
				log.Warning(fmt.Sprintf("%s", err))
			}

			var data []Hc

			//this
			err = json.Unmarshal(contents, &data)
			if err != nil {
				log.Warning(fmt.Sprintf("%s", err))
			}

			//this does update job list by translating and updating mongo directly

			for _, v := range data {

				base := make(map[string]interface{})

				hctype := strings.TrimSpace(strings.ToUpper(v.HcType))
				hcid := fmt.Sprintf("%d", v.Hcid)
				hcupdated := v.Updated

				base, err = clean_base(v.Meta)
				if err != nil {
					log.Warning("bad config %s", err)
					fmt.Printf("bad config %s\n", err)
					continue
				}

				base["upd"] = hcupdated

				switch hctype {
				case "DNS":
					base = clean_dns(v.Meta, base)
				case "TCP":
					base = clean_tcp(v.Meta, base)
				case "PING":
					base = clean_ping(v.Meta, base)
				case "HTTP_GET":
					base = clean_http(v.Meta, base)
					base["request"] = "get"
				case "HTTP_POST":
					base = clean_http(v.Meta, base)
					base["request"] = "post"
				case "HTTP_HEAD":
					base = clean_http(v.Meta, base)
					base["request"] = "head"
				}

				cron := base["cron"].(string)
				updated := base["upd"].(int32)
				delete(base, "cron")
				delete(base, "upd")

				//now lets see if we need to insert or update this

				if _, ok := existing[hcid]; ok {
					//update

					query := bson.M{"hcid": hcid}
					change := bson.M{"$set": bson.M{
						"cron":    cron,
						"meta":    base,
						"updated": updated,
					}}

					err = hc_conn.Update(query, change)
					if err != nil {
						log.Warning("error updating %s", err)
					} else {
						rec_updated++
					}

					//delete from existing map, ramainng will need to be deleted
					delete(existing, hcid)

				} else {
					//insert

					//make proper object
					hc := dhc4.Hc{
						BusyTs:    0,
						Sn:        "",
						Cron:      cron,
						LastRunTs: 0,
						NextRunTs: 0,
						Hcid:      hcid,
						HcType:    hctype,
						Ver:       dhc4.DHC_VER,
						Meta:      base,
						Updated:   updated,
					}

					err = hc_conn.Insert(&hc)
					if err != nil {
						log.Warning("error inserting %s", err)
					} else {
						rec_inserted++
					}
				}
			}

			for todelete, _ := range existing {

				err = hc_conn.Remove(bson.M{"hcid": todelete})
				if err != nil {
					log.Debug("Error deleting hcid %s result %s", todelete, err)
				} else {
					rec_deleted++
				}
			}

			//now unlock sys

			query := bson.M{"sys": "feederlock", "sn": Gnodeid}

			//this is info we will try to lock with
			change := mgo.Change{
				Update:    bson.M{"$set": bson.M{"bts": 0, "lr": int(time.Now().Unix()), "sn": Gnodeid}},
				ReturnNew: false,
			}

			log.Debug("running query %+v", query)

			log.Debug("with change %+v", change)

			feederlock := FeederLock{} //Return unlocked object into this

			log.Debug("query %v", query)

			info, err := sys_conn.Find(query).Sort("bts").Apply(change, &feederlock)
			if err != nil {
				log.Debug("Query result %v, %s, %v", info, err, query)
			}
		}
		Gstats = fmt.Sprintf("inserted: %d, updated: %d, deleted: %d", rec_inserted, rec_updated, rec_deleted)
		log.Notice(Gstats)
		if sleepTimeSec == MIN_SLEEP_SEC {
			time.Sleep(time.Duration(RUN_EVERY_SEC) * time.Second)
		}
	}
}

//these are translators from Web API to dhc4 api
func clean_base(config map[string]interface{}) (map[string]interface{}, error) {
	meta := make(map[string]interface{})

	if el, ok := config["host"]; ok {
		if v, ok := el.(string); ok {
			meta["host"] = v
		} else {
			return meta, errors.New("Invalid host value")
		}
	} else {
		return meta, errors.New("Invalid host value")
	}

	meta["cron"] = dhc4.DEFAULT_CRON_1MIN
	if el, ok := config["cron"]; ok {
		if v, ok := el.(string); ok {
			meta["cron"] = v
		}
	}

	meta["timeout"] = dhc4.TIMEOUT_SEC
	if el, ok := config["timeout_sec"]; ok == true {
		v := dhc4.TIMEOUT_SEC
		if _, ok := el.(string); ok {
			if v, err := strconv.Atoi(el.(string)); err == nil {
				v = v
			}
		}

		if _, ok := el.(int); ok {
			v = el.(int)
		}

		if v > 2 || v <= 10 {
			meta["timeout"] = v
		}
	}

	if el, ok := config["updated"]; ok == true {
		v := int32(time.Now().Unix())
		if _, ok := el.(string); ok {
			if v, err := strconv.Atoi(el.(string)); err == nil {
				v = v
			}
		}

		if _, ok := el.(int32); ok {
			v = el.(int32)
		}

		meta["upd"] = int32(v)
	}

	//retry up
	meta["rup"] = "1"
	if el, ok := config["retry_up"]; ok == true {
		v := "1"

		if _, ok := el.(string); ok {
			if v, err := strconv.Atoi(el.(string)); err == nil {
				v = v
			}
		}

		if _, ok := el.(int); ok {
			v = fmt.Sprintf("%d", el.(int))
		}

		meta["rup"] = v
	}

	meta["rdn"] = "1"
	if el, ok := config["retry_down"]; ok == true {
		v := "1"

		if _, ok := el.(string); ok {
			if v, err := strconv.Atoi(el.(string)); err == nil {
				v = v
			}
		}

		if _, ok := el.(int); ok {
			v = fmt.Sprintf("%d", el.(int))
		}

		meta["rup"] = v
	}

	return meta, nil
}

func clean_dns(config map[string]interface{}, meta map[string]interface{}) map[string]interface{} {
	v := dhc4.DEFAULT_DNS_REC
	if el, ok := config["record_type"]; ok {
		if _, ok := el.(string); ok {
			v = el.(string)
		}
	}
	meta["record_type"] = v

	return meta
}

func clean_tcp(config map[string]interface{}, meta map[string]interface{}) map[string]interface{} {

	if el, ok := config["port"]; ok {

		if _, ok := el.(string); ok {
			meta["port"] = el.(string)
		}
		if _, ok := el.(int); ok {
			meta["port"] = fmt.Sprintf("%d", el.(int))
		}
	}

	v := "tcp"
	if el, ok := config["proto"]; ok {
		if _, ok := el.(string); ok {
			v = el.(string)
		}
	}

	meta["proto"] = v

	return meta
}

func clean_ping(config map[string]interface{}, meta map[string]interface{}) map[string]interface{} {
	meta["ok_ploss"] = dhc4.OK_PACKET_LOSS_P
	meta["packets"] = dhc4.DEFAULT_PACKET_CNT
	meta["size"] = dhc4.DEFAULT_PACKET_SIZE

	if el, ok := config["success"]; ok == true {
		//parse interface
		if el, ok := el.(map[string]interface{}); ok == true {
			//parse string out of map
			if el, ok := el["percent_loss"]; ok == true {
				//parce out a float
				//type ensure
				if _, ok := el.(string); ok {
					if el, ok := el.(string); ok == true {
						meta["ok_ploss"] = el
					}
				}
			}
		}
	}

	return meta
}

func clean_http(config map[string]interface{}, meta map[string]interface{}) map[string]interface{} {

	if el, ok := config["port"]; ok {
		if _, ok := el.(string); ok {
			meta["port"] = el.(string)
		}
		if _, ok := el.(int); ok {
			meta["port"] = fmt.Sprintf("%d", el.(int))
		}
	}

	meta["url"] = "/"
	if el, ok := config["url"]; ok {
		if _, ok := el.(string); ok {
			meta["url"] = el.(string)
		}
	}

	meta["proto"] = "head"
	if el, ok := config["protocol"]; ok {
		if _, ok := el.(string); ok {
			meta["proto"] = el.(string)
		}
	}

	v := []string{}
	if el, ok := meta["headers"]; ok == true {
		if _, ok := el.([]string); ok {
			for _, h := range el.([]string) {
				v = append(v, string(h))
			}
		}
		meta["headers"] = v
	}

	if el, ok := config["post_fields"]; ok {
		meta["post_fields"] = ""
		if _, ok := el.(string); ok {
			meta["post_fields"] = el.(string)
		}
	}

	meta["ok_code"] = []string{"200", "206"}

	if el, ok := config["success"]; ok == true {
		//parse interface
		if el, ok := el.(map[string]interface{}); ok == true {

			if el, ok := el["response_string"]; ok == true {
				v_ok := []string{}
				if els, ok := el.([]interface{}); ok {
					for _, v := range els {
						//js only has floats !!!
						if v, ok := v.(string); ok {
							v_ok = append(v_ok, string(v))
						}
					}

					meta["ok_string"] = v_ok
				}
			}

			if el, ok := el["headers"]; ok == true {
				v_ok := []string{}
				if els, ok := el.([]interface{}); ok {
					for _, v := range els {
						//js only has floats !!!
						if v, ok := v.(string); ok {
							v_ok = append(v_ok, string(v))
						}
					}

					meta["ok_headers"] = v_ok
				}
			}

			if el, ok := el["response_code"]; ok == true {
				v_ok := []string{}
				if els, ok := el.([]interface{}); ok {
					for _, v := range els {
						//js only has floats !!!
						if v, ok := v.(float64); ok {
							v_ok = append(v_ok, fmt.Sprintf("%d", int32(v)))
						}
					}

					meta["ok_code"] = v_ok
				}
			}

		}
	}

	return meta
}

//Http Handlers
func serve404(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "Not Found")
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
	res["ts"] = time.Now().String()
	res["msg"] = Gstats

	b, err := json.Marshal(res)
	if err != nil {
		log.Error("error: %s", err)
	}

	w.Write(b)
	return
}
