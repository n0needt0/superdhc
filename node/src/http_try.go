package main

import (
	dhc4 "./dhc4"
	"fmt"
	"github.com/jaekwon/go-prelude/colors"
	"os"
	"strings"
	"time"
)

//test cases

//http
//get
//post
//head
//https
//get
//post
//head
//heades
//urls
//postfields

//results
//test code
//test string
//test header

func main() {
	t := map[string]interface{}{}
	t["get_by_code_ok"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}}}
	t["get_by_code_fail"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/get", "ok": map[string]interface{}{"rcode": []int{2001}}}
	t["get_by_string_ok"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/get?musikpusi", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "musikpusi"}}
	t["get_by_string_fail"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "musikpusi!"}}
	t["get_by_header_ok"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "Content-Type: application/json"}}
	t["get_by_header_fail"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "mus1kpus1k"}}

	t["get_ssl_by_code_ok"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}}}
	t["get_ssl_by_code_fail"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/get", "ok": map[string]interface{}{"rcode": []int{2001}}}
	t["get_ssl_by_string_ok"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/get?musikpusi", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "musikpusi"}}
	t["get_ssl_by_string_fail"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "musikpusi!"}}
	t["get_ssl_by_header_ok"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "Content-Type: application/json"}}
	t["get_ssl_by_header_fail"] = map[string]interface{}{"request": "get", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/get", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "mus1kpus1k"}}

	t["head_by_code_ok"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{200, 206}}}
	t["head_by_code_fail"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{2001}}}
	t["head_by_header_ok"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "Content-Type: application/json"}}
	t["head_by_header_fail"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "mus1kpus1k"}}

	t["head_ssl_by_code_ok"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{200, 206}}}
	t["head_ssl_by_code_fail"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{2001}}}
	t["head_ssl_by_header_ok"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "Content-Type: application/json"}}
	t["head_ssl_by_header_fail"] = map[string]interface{}{"request": "head", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/headers", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "mus1kpus1k"}}

	t["post_by_code_ok"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}}}
	t["post_by_code_fail"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/post", "ok": map[string]interface{}{"rcode": []int{2001}}}
	t["post_by_string_ok"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "test=test"}}
	t["post_by_string_fail"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "musikpusi!"}}
	t["post_by_header_ok"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "Content-Type: application/json"}}
	t["post_by_header_fail"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 80, "proto": "http", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "mus1kpus1k"}}

	t["post_ssl_by_code_ok"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}}}
	t["post_ssl_by_code_fail"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/post", "ok": map[string]interface{}{"rcode": []int{2001}}}
	t["post_ssl_by_string_ok"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "test=test"}}
	t["post_ssl_by_string_fail"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "string": "musikpusi!"}}
	t["post_ssl_by_header_ok"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "Content-Type: application/json"}}
	t["post_ssl_by_header_fail"] = map[string]interface{}{"request": "post", "host": "httpbin.org", "timeout": 10, "port": 443, "proto": "https", "url": "/post", "ok": map[string]interface{}{"rcode": []int{200, 206}, "header": "mus1kpus1k"}}

	//https get
	//t["getsec"] = map[string]interface{}{"request": "get", "host": "www.google.com", "timeout": 10, "port": 443, "proto": "https", "url": "/", "ok": map[string]interface{}{"rcode": []int{200, 206}}}

	reschan := make(chan map[string]interface{})

	for test := range t {
		meta := t[test]
		//fmt.Printf("\n %+v", meta)

		hchttp, err := dhc4.NewHttp(meta.(map[string]interface{}))
		if err != nil {
			fmt.Printf("%s", err)
			os.Exit(0)
		}

		go hchttp.DoTest(reschan)
		testing := true
		for testing {
			select {
			case res := <-reschan:
				//fmt.Printf("\n\n%+v\n\n", meta)
				pass := "fail"
				for i, v := range res {
					//					fmt.Printf("HTTP RES: %+v, %+v\n", i, v)
					if i == "state" && v.(int) == 1 {

						if strings.Contains(test, "_ok") {
							pass = "pass"
						}

						if strings.Contains(test, "_fail") {
							pass = "fail"
						}
					}

					if i == "state" && v.(int) == 0 {

						if strings.Contains(test, "_ok") {
							pass = "fail"
						}

						if strings.Contains(test, "_fail") {
							pass = "pass"
						}
					}
				}
				if pass == "pass" {
					fmt.Printf(" %s : %v\n****************************************\n", test, colors.Green(pass))
				} else {
					fmt.Printf(" %s : %v\n****************************************\n", test, colors.Red(pass))
				}
				testing = false
			case <-time.After(time.Duration(hchttp.Timeout) * time.Second):
				fmt.Printf("HTTP: %s :%s timeout %d sec", hchttp.Host, hchttp.Url, hchttp.Timeout)
				testing = false
			}
		}
	}

}
