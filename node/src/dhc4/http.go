/**
* HTTP DHC4 Module by AY
**/

package dhc4

import (
	"errors"
	"fmt"
	"github.com/andelf/go-curl"
	"net/url"
	"strconv"
	"strings"
)

var valid_request map[string]int = map[string]int{"get": 1, "post": 1, "head": 1}
var valid_protocol map[string]int = map[string]int{"http": 1, "https": 1}

type HcHttp struct {
	Host        string
	Request     string
	Proto       string
	Headers     []string
	Port        int
	Url         string
	Post_fields string
	Timeout     int //timeout sec
	OkCode      []string
	OkString    string
	OkHeader    string
	Res         map[string]interface{}
}

func NewHttp(meta map[string]interface{}) (*HcHttp, error) {
	hchttp := &HcHttp{
		Host:        "",
		Request:     "get",
		Proto:       "http",
		Headers:     []string{},
		Port:        80,
		Url:         "/",
		Post_fields: "",
		Timeout:     TIMEOUT_SEC,
		OkCode:      []string{"200", "206"},
		OkString:    "",
		OkHeader:    "",
		Res:         make(map[string]interface{}),
	}

	err := hchttp.loadMeta(meta)
	if err != nil {
		return hchttp, err
	}

	return hchttp, nil
}

func (hchttp *HcHttp) loadMeta(meta map[string]interface{}) error {
	//required settings
	if el, ok := meta["host"]; ok {
		if _, ok := el.(string); ok {
			v := el.(string)
			hchttp.Host = v
		} else {
			return errors.New("HTTP: Invalid host value")
		}
	} else {
		return errors.New("HTTP: Invalid host value")
	}

	hchttp.Timeout = TIMEOUT_SEC
	if el, ok := meta["timeout"]; ok == true {
		v := 0

		if _, ok := el.(string); ok {
			if v, err := strconv.Atoi(el.(string)); err == nil {
				v = v
			}
		}

		if _, ok := el.(int); ok {
			v = el.(int)
		}

		if v > 2 || v < 10 {
			hchttp.Timeout = v
		}
	}

	hchttp.Url = "/"
	if el, ok := meta["url"]; ok == true {
		if _, ok := el.(string); ok {
			hchttp.Url = el.(string)
		}
	}

	hchttp.Proto = "http"
	if el, ok := meta["proto"]; ok == true {
		if _, ok := el.(string); ok {
			if _, ok := valid_protocol[el.(string)]; ok == true {
				hchttp.Proto = el.(string)
			}
		}
	}

	hchttp.Port = 80

	if el, ok := meta["port"]; ok == true {
		if _, ok := el.(string); ok {
			v, err := strconv.Atoi(el.(string))
			if err == nil {
				hchttp.Port = v
			}
		}
		if _, ok := el.(int); ok {
			hchttp.Port = el.(int)
		}
	} else {
		if hchttp.Proto == "https" {
			hchttp.Port = 433
		}
	}

	hchttp.Request = "head"
	if el, ok := meta["request"]; ok == true {
		if _, ok := el.(string); ok {
			if _, ok := valid_request[el.(string)]; ok == true {
				hchttp.Request = el.(string)
			}
		}
	}

	hchttp.Headers = []string{}

	if el, ok := meta["headers"]; ok == true {
		if _, ok := el.([]string); ok {
			for _, h := range el.([]string) {
				hchttp.Headers = append(hchttp.Headers, string(h))
			}
		}
	}

	hchttp.Post_fields = ""
	if el, ok := meta["post_fields"]; ok == true {
		if _, ok := el.(string); ok {
			hchttp.Post_fields = el.(string)
		}
	}
	hchttp.OkCode = []string{"200", "206"}

	if el, ok := meta["ok"]; ok == true {
		//parse interface
		if el, ok := el.(map[string]interface{}); ok == true {
			//parse string out of map
			if el, ok := el["rcode"]; ok == true {
				codes := []string{}
				if _, ok := el.([]int); ok {
					for _, e := range el.([]int) {
						codes = append(codes, fmt.Sprintf("%d", e))
					}
					hchttp.OkCode = codes
				}

				if _, ok := el.([]string); ok {
					for _, e := range el.([]string) {
						codes = append(codes, e)
					}
					hchttp.OkCode = codes
				}
			}
		}
	}

	hchttp.OkString = ""
	if el, ok := meta["ok"]; ok == true {
		//parse interface
		if el, ok := el.(map[string]interface{}); ok == true {
			//parse string out of map
			if el, ok := el["string"]; ok == true {
				if _, ok := el.(string); ok {
					hchttp.OkString = el.(string)
				}
			}
		}
	}

	hchttp.OkHeader = ""

	if el, ok := meta["ok"]; ok == true {
		//parse interface
		if el, ok := el.(map[string]interface{}); ok == true {
			//parse string out of map
			if el, ok := el["header"]; ok == true {
				if _, ok := el.(string); ok {
					hchttp.OkHeader = el.(string)
				}
			}
		}
	}

	return nil
}

func (hchttp *HcHttp) DoTest(result chan map[string]interface{}) error {

	res := make(map[string]interface{})

	hchttp.Res["state"] = HEALTH_STATE_DOWN //it is not working unless noted otherwise

	curlobj := curl.EasyInit()
	defer curlobj.Cleanup()

	if curlobj == nil {
		msg := fmt.Sprintf("HTTP failed to initialize curl")
		log.Error(msg)
		res["msg"] = msg
		result <- res
		return nil
	}

	uri := fmt.Sprintf("%s://%s/%s", hchttp.Proto, hchttp.Host, strings.TrimLeft(hchttp.Url, "/"))
	fulluri := fmt.Sprintf("%s://%s:%d/%s", hchttp.Proto, hchttp.Host, hchttp.Port, strings.TrimLeft(hchttp.Url, "/"))

	curlobj.Setopt(curl.OPT_URL, uri)
	curlobj.Setopt(curl.OPT_FAILONERROR, true)
	curlobj.Setopt(curl.OPT_VERBOSE, false)
	curlobj.Setopt(curl.OPT_HEADER, true)
	curlobj.Setopt(curl.OPT_HTTPHEADER, []string{"Expect:"})

	//read headers callback
	okstringheader := false
	curlobj.Setopt(curl.OPT_HEADERFUNCTION,
		func(buf []byte, userdata interface{}) bool {
			if hchttp.OkHeader != "" && strings.Contains(string(buf), hchttp.OkHeader) {
				okstringheader = true
			}
			return true
		})

	//read body callback
	okstringinbody := false
	curlobj.Setopt(curl.OPT_WRITEFUNCTION,
		func(buf []byte, userdata interface{}) bool {
			if hchttp.OkString != "" && strings.Contains(string(buf), hchttp.OkString) {
				okstringinbody = true
			}
			return true
		})

	if hchttp.Request == "head" { //set header request
		curlobj.Setopt(curl.OPT_NOBODY, true)
	} else {
		if hchttp.Request == "post" {
			post_data := url.QueryEscape("test=test")
			curlobj.Setopt(curl.OPT_POST, true)
			curlobj.Setopt(curl.OPT_POSTFIELDSIZE, len(post_data))

			//doing post callback
			sent := false
			curlobj.Setopt(curl.OPT_READFUNCTION,
				func(buf []byte, userdata interface{}) int {
					// WARNING: never use append()
					if !sent {
						sent = true
						ret := copy(buf, post_data)
						return ret
					}
					return 0 // sent ok
				})

		}
	}

	curlobj.Setopt(curl.OPT_TIMEOUT, hchttp.Timeout)
	curlobj.Setopt(curl.OPT_CONNECTTIMEOUT, hchttp.Timeout)
	curlobj.Setopt(curl.OPT_FRESH_CONNECT, true)

	if hchttp.Port != 0 {
		curlobj.Setopt(curl.OPT_PORT, hchttp.Port)
	}

	curlobj.Setopt(curl.OPT_USERAGENT, USER_AGENT)

	curlobj.Setopt(curl.OPT_FOLLOWLOCATION, true)

	curlobj.Setopt(curl.OPT_SSL_VERIFYHOST, false)
	curlobj.Setopt(curl.OPT_SSL_VERIFYPEER, false)

	curlobj.Setopt(curl.OPT_RANGE, "0-262144")

	curlobj.Setopt(curl.OPT_HTTP200ALIASES, hchttp.OkCode)
	curlobj.Setopt(curl.OPT_HTTPHEADER, hchttp.Headers)

	err := curlobj.Perform()
	if err != nil {
		msg := fmt.Sprintf("HTTP to %s result %s", fulluri, err)
		res["msg"] = msg
		result <- res
		return nil
	}

	v, _ := curlobj.Getinfo(curl.INFO_HTTP_CODE)
	if _, ok := v.(int); ok {
		res["http_code"] = v.(int)
	}

	if v, err := curlobj.Getinfo(curl.INFO_NAMELOOKUP_TIME); err == nil {
		if _, ok := v.(float64); ok {
			res["nsl_ms"] = v.(float64) * float64(1000) //convert to msed
		}
	}

	if v, err := curlobj.Getinfo(curl.INFO_PRETRANSFER_TIME); err == nil {
		if _, ok := v.(float64); ok {
			res["con_ms"] = v.(float64) * float64(1000)
		}

	}
	if v, err := curlobj.Getinfo(curl.INFO_STARTTRANSFER_TIME); err == nil {
		if _, ok := v.(float64); ok {
			res["tfb_ms"] = v.(float64) * float64(1000)
		}
	}
	if v, err := curlobj.Getinfo(curl.INFO_TOTAL_TIME); err == nil {
		if _, ok := v.(float64); ok {
			res["tot_ms"] = v.(float64) * float64(1000)
		}
	}

	//lets check codes, remember codes replied in int, but submitted as strings
	state := HEALTH_STATE_DOWN
	str_http_code := fmt.Sprintf("%d", res["http_code"].(int))
	for _, v := range hchttp.OkCode {
		if str_http_code == v {
			//we are good
			state = HEALTH_STATE_UP
		}
	}

	//if string is specified check more but not for Header only
	if state == HEALTH_STATE_UP && hchttp.OkString != "" && !okstringinbody {
		res["msg"] = fmt.Sprintf("string \"%s\" not found", hchttp.OkString)
		state = HEALTH_STATE_DOWN
	}

	//if header specified check header
	if state == HEALTH_STATE_UP && hchttp.OkHeader != "" && !okstringheader {
		res["msg"] = fmt.Sprintf("header \"%s\" not found", hchttp.OkHeader)
		state = HEALTH_STATE_DOWN
	}

	res["state"] = state
	result <- res

	return nil
}
