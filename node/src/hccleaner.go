package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"log"
	"os"
	"time"
)

type Hc struct {
	Id          string
	BusyTs      int64
	RunEverySec int
	LastRunTs   int64
	NextRunTs   int64
	OwnerId     string
	HcClass     string
	State       int
	History     map[string]interface{}
	TestConf    map[string]interface{}
	AlertConf   map[string]interface{}
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
		Key:        []string{"id"},
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

	query := bson.M{"busyts": bson.M{"$lt": time.Now().Unix() - 60, "$gt": 0}}

	change := mgo.Change{
		Update:    bson.M{"$set": bson.M{"busyts": 0}},
		ReturnNew: true,
	}

	result := Hc{}
	//err = c.Find(bson.M{"id": "id2"}).One(&result)

	_, err = c.Find(query).Sort("busyts").Apply(change, &result)

	if err != nil {
		log.Fatal(err)
	}

	b, err := json.Marshal(result)
	if err != nil {
		fmt.Println("error:", err)
	}
	os.Stdout.Write(b)

}
