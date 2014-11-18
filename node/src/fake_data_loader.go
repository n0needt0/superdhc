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
	Id            bson.ObjectId `json:"id" bson:"_id,omitempty"`
	HealthcheckId string
	BusyTs        int64
	RunEverySec   int
	LastRunTs     int64
	NextRunTs     int64
	OwnerId       string
	HcClass       string
	State         int
	History       map[string]interface{}
	TestConf      map[string]interface{}
	AlertConf     map[string]interface{}
}

func main() {
	session, err := mgo.Dial("192.168.42.100:27017,192.168.42.110:27017,192.168.42.120:27017")

	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}
	defer session.Close()

	session.SetMode(mgo.Strong, true)

	c := session.DB("test").C("test")

	// Unique Index
	index := mgo.Index{
		Key:        []string{"healthcheckid"},
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
	err = c.EnsureIndexKey("bysyts", "nextrunts")
	if err != nil {
		fmt.Printf("%s", err)
	}

	//populate

	for i := 0; i < 10000000192.168.2.105; i++ {

		err = c.Insert(&Hc{
			//

			HealthcheckId: fmt.Sprintf("id %d", i),
			BusyTs:        time.Now().Unix() - 6,
			RunEverySec:   60,
			LastRunTs:     time.Now().Unix(),
			NextRunTs:     time.Now().Unix(),
			OwnerId:       fmt.Sprintf("owner %d", i),
			HcClass:       fmt.Sprintf("testclass %d", i),
			State:         1,
			History:       make(map[string]interface{}),
			TestConf:      make(map[string]interface{}),
			AlertConf:     make(map[string]interface{}),
		})

		if err != nil {
			fmt.Printf("Insert %s", err)
		}
	}
}
