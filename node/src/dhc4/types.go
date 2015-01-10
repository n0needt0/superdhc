/**
* DNS types Module by AY
* Contains basic types used and message manipulation
**/

package dhc4

import (
	"bytes"
	"encoding/json"
	"errors"
	"gopkg.in/mgo.v2/bson"
	"time"
)

//result of health check
type HcResult struct {
	ID        bson.ObjectId `bson:"_id,omitempty"`
	Result    bool          `json:"r" bson:"r"`
	Hcid      string
	HcType    string
	Sn        string
	LastRunTs int64 `json:"lr" bson:"lr"`
	Meta      map[string]interface{}
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
}

//even struct
type HcEvent struct {
	ID         bson.ObjectId `bson:"_id,omitempty"`
	SERIAL     string
	Result     bool
	Hcid       string
	AckSerial  string
	ReportedAt time.Time `json:"reportedAt" bson:"reportedAt"`
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

//********NODE MESSAGE******
//message to communicate hc to node and receive results
type NodeMsg struct {
	SERIAL string
	TS     int64
	ERROR  string
	HC     Hc
	RESULT map[string]interface{}
}

func NewNodeMsg() NodeMsg {
	return NodeMsg{}
}

//format request
func (msg NodeMsg) Pack(serial string, hc Hc) (jsonstr []byte, err error) {

	msg.SERIAL = serial
	msg.TS = time.Now().Unix()
	msg.ERROR = ""
	msg.HC = hc
	msg.RESULT = make(map[string]interface{})

	jsonstr, err = json.Marshal(msg)
	if err != nil {
		return []byte(""), err
	}
	return jsonstr, nil
}

func (msg NodeMsg) Unpack(JsonMsg []byte) (NodeMsg, error, error) {

	d := json.NewDecoder(bytes.NewReader(JsonMsg))
	d.UseNumber()

	if err := d.Decode(&msg); err != nil {
		//can not decode it. let cleaner process deal with it
		return NodeMsg{}, err, nil
	}

	if msg.ERROR != "" {
		//decoded it but something is reported from far
		return msg, nil, errors.New(msg.ERROR)
	}
	return msg, nil, nil
}

func (msg NodeMsg) Marshal(nmsg NodeMsg) ([]byte, error) {

	jsonstr, err := json.Marshal(nmsg)
	if err != nil {
		return []byte("{}"), err
	}

	return jsonstr, nil
}

//*********NODE MESSAGE END********

//*********JUDGE MESSAGE START*****

type JudgeMsg struct {
	SERIAL string
	TS     int64
	ERROR  string
	HC     Hc
	HIST   []HcResult
	STATE  bool
	TRACE  map[string]interface{}
	RESULT map[string]interface{}
}

func NewJudgeMsg() JudgeMsg {
	return JudgeMsg{}
}

//format request
func (msg JudgeMsg) Pack(serial string, hc Hc, history []HcResult, state bool, trace map[string]interface{}) (jsonstr []byte, err error) {

	msg.SERIAL = serial
	msg.TS = time.Now().Unix()
	msg.ERROR = ""
	msg.HC = hc
	msg.HIST = history
	msg.STATE = state
	msg.TRACE = trace
	msg.RESULT = make(map[string]interface{})

	jsonstr, err = json.Marshal(msg)
	if err != nil {
		return []byte(""), err
	}
	return jsonstr, nil
}

func (msg JudgeMsg) Unpack(JsonMsg []byte) (JudgeMsg, error, error) {

	d := json.NewDecoder(bytes.NewReader(JsonMsg))
	d.UseNumber()

	if err := d.Decode(&msg); err != nil {
		//can not decode it. let cleaner process deal with it
		return JudgeMsg{}, err, nil
	}

	if msg.ERROR != "" {
		//decoded it but something is reported from far
		return msg, nil, errors.New(msg.ERROR)
	}
	return msg, nil, nil
}

func (msg JudgeMsg) Marshal(nmsg JudgeMsg) ([]byte, error) {

	jsonstr, err := json.Marshal(nmsg)
	if err != nil {
		return []byte("{}"), err
	}

	return jsonstr, nil
}

//********JUDGE MESSAGE END**********************************
