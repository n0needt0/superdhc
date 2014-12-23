/**
* HTTP DHC4 Module by AY
**/

package dhc4

import (
	"errors"
	"fmt"
	"github.com/parnurzeal/gorequest"
	"strconv"
	"strings"
	"time"
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

	//set up
	request := gorequest.New().Timeout(time.Duration(hchttp.Timeout)*time.Second).Set("User-Agent", USER_AGENT)

	uri := fmt.Sprintf("%s://%s/%s", hchttp.Proto, hchttp.Host, strings.TrimLeft(hchttp.Url, "/"))

	if (hchttp.Port != 80 && hchttp.Proto == "http") || (hchttp.Port != 443 && hchttp.Proto == "https") {
		uri = fmt.Sprintf("%s://%s:%d/%s", hchttp.Proto, hchttp.Host, hchttp.Port, strings.TrimLeft(hchttp.Url, "/"))
	}

	for _, v := range hchttp.Headers {

		header := strings.Split(v, ":")
		if len(header) == 2 {
			request.Set(header[0], header[1])
		}
	}

	switch hchttp.Request {
	case "get":
		request.Get(uri)
	case "post":
		request.Post(uri).Query("test=test")
	case "head":
		request.Head(uri)
	}

	start := makeTimestamp()

	response, body, errs := request.End()
	if errs != nil {
		msg := fmt.Sprintf("HTTP to %s result %+v", uri, errs)
		res["msg"] = msg
		result <- res
		return nil
	}

	testtime := makeTimestamp() - start

	res["http_code"] = response.StatusCode
	res["nsl_ms"] = testtime / 1000 //convert to msed
	res["con_ms"] = res["nsl_ms"]
	res["tfb_ms"] = res["nsl_ms"]
	res["tot_ms"] = res["nsl_ms"]

	//lets check codes, remember codes replied in int, but submitted as strings
	state := HEALTH_STATE_DOWN
	str_http_code := fmt.Sprintf("%d", response.StatusCode)
	for _, v := range hchttp.OkCode {
		if str_http_code == v {
			//we are good
			state = HEALTH_STATE_UP
		}
	}
	//if string is specified check more but not for Header only
	okstringinbody := false
	if hchttp.OkString != "" && strings.Contains(body, hchttp.OkString) {
		okstringinbody = true
	}
	if state == HEALTH_STATE_UP && hchttp.OkString != "" && !okstringinbody {
		res["msg"] = fmt.Sprintf("string \"%s\" not found", hchttp.OkString)
		state = HEALTH_STATE_DOWN
	}

	//if header specified check header
	//remember header in response comes as map[string]string
	//yet can be specified as string

	okstringheader := false
	if hchttp.OkHeader != "" {

		hdr := strings.Split(hchttp.OkHeader, ":")

		for k, v := range response.Header {

			//if passed as proper header
			if len(hdr) == 2 && k == strings.TrimSpace(hdr[0]) && v[0] == strings.TrimSpace(hdr[1]) {
				okstringheader = true
			}

			//if passed as some string
			if len(hdr) == 1 && strings.Contains(fmt.Sprintf("%s : %s", k, v[0]), hchttp.OkHeader) {
				okstringheader = true
			}
		}
	}

	if state == HEALTH_STATE_UP && hchttp.OkHeader != "" && !okstringheader {
		res["msg"] = fmt.Sprintf("header \"%s\" not found", hchttp.OkHeader)
		state = HEALTH_STATE_DOWN
	}

	res["state"] = state
	result <- res

	return nil
}
