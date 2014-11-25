//List of structs


//Struct to create running queuee





type Hc struct {
  Id                    bson.ObjectId   `bson:"_id,omitempty" json:"id"`            //Can we make _id to be our internal index
  Hcid                string                `bson:"hcid,omitempty" json:"hcid,string"`   //this is legacy healthcheck it same as in db
  BusyTs            int64                 `bson:",minsize" json:"busyts,string"`
  RunEverySec   int                     `bson:",omitempty" json:"everysec,string"`
  LastRunTs        int64                `bson:",minsize" json:"lastts,string"`
  NextRunTs       int64                `bson:",minsize" json:"nextts,string"`
  HcClass            string
  State                int
  History             map[string]interface{}
  TestConf          map[string]interface{}
  AlertConf         map
  expireAt          time.Time
}




//health check structure
type Hc struct {
  Id            bson.ObjectId `json:"id" bson:"_id,omitempty"`
  Hcid string
  BusyTs        int64 `json:"id,string,omitempty"`
  RunEverySec   int   `json:"runeverysec,string,omitempty"`
  LastRunTs     int64 `json:"id,string,omitempty"`
  NextRunTs     int64 `json:"id,string,omitempty"`
  OwnerId       string
  HcClass       string
  State         int
  History       map[string]interface{}
  TestConf      map[string]interface{}
  AlertConf     map
  expireAt time.Time : time.Now()
}

  type HcTs struct{
              year  int
              month int
              day int
              wday  int
              hour  int
              min int
              sec int
              zone  string
              offset  int
  }


type HcHist struct {
    Id                bson.ObjectId         `json:"id" bson:"_id,omitempty"`
    Hcid            string                      `json:"id,string,omitempty"`
    Ts               HcTs
    Node           string                      `json:"id,string,omitempty"`
    Serial          string                      `json:"id,string,omitempty"`
    result          map[string]string
}

db.log_events.ensureIndex( { "expireAt": 1 }, { expireAfterSeconds: 0 } )
"expireAt": new Date('July 22, 2013 14:00:00'),
Add expire at
