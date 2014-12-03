package main

import (
	//"encoding/json"
	"fmt"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	//"log"
	"os"
	"time"
)

type Hc struct {
	Id          bson.ObjectId `bson:"_id,omitempty" json:"id"`               //Can we make _id to be our internal index
	Hcid        string        `bson:"HEalthId,omitempty" json:"hcid,string"` //this is legacy healthcheck it same as in db
	BusyTs      int64         `bson:",minsize" json:"busyts,string"`
	RunEverySec int           // `bson:",omitempty" json:"everysec,string"`
	LastRunTs   int64         // `bson:",omitempty" json:"lastts,string"`
	NextRunTs   int64         // `bson:",omitempty" json:"nextts,string"`
	HcClass     string
	State       int
	History     map[string]string
	TestConf    map[string]string
	AlertConf   map[string]string
	expireAt    time.Time
}

func main() {
	session, err := mgo.Dial("192.168.82.100:27017,192.168.82.110:27017,192.168.82.120:27017")

	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}
	defer session.Close()

	session.SetMode(mgo.Strong, true)

	c := session.DB("test").C("test")

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
		fmt.Printf("%s", err)
	}

	// search index
	err = c.EnsureIndexKey("bts", "nr")
	if err != nil {
		fmt.Printf("%s", err)
	}

	//populate

	for i := 0; i < 10; i++ {

		err = c.Insert(&Hc{
			//

			Hcid:        fmt.Sprintf("id %d", i),
			BusyTs:      time.Now().Unix() - 6,
			RunEverySec: 60,
			LastRunTs:   time.Now().UnixNano(),
			NextRunTs:   time.Now().Unix(),
			HcClass:     fmt.Sprintf("testclass %d", i),
			State:       1,
			History:     make(map[string]string),
			TestConf:    make(map[string]string),
			AlertConf:   make(map[string]string),
			expireAt:    time.Now(),
		})

		if err != nil {
			fmt.Printf("Insert %s", err)
		}
	}
}
