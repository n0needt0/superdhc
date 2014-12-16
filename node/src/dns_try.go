package main

import (
	dhc4 "./dhc4"
	"fmt"
	"os"
	"time"
)

var test_dns_records = []string{"DNS_A", "DNS_CNAME", "DNS_HINFO", "DNS_MX", "DNS_NS", "DNS_PTR", "DNS_SOA", "DNS_TXT", "DNS_AAAA", "DNS_SRV", "DNS_NAPTR", "DNS_A6", "DNS_ALL", "DNS_ANY"}

func main() {
	reschan := make(chan map[string]interface{})
	for _, rt := range test_dns_records {
		meta := map[string]interface{}{"host": "google.com", "record_type": rt, "timeout": 10}
		hcdns, err := dhc4.NewDns(meta)
		if err != nil {
			fmt.Printf("%s", err)
			os.Exit(0)
		}

		go hcdns.DoTest(reschan)
		testing := true
		for testing {
			select {
			case res := <-reschan:
				fmt.Printf("\n\n%+v\n\n", meta)
				for i, v := range res {
					fmt.Printf("DNS RES: %+v, %+v\n", i, v)
				}
				testing = false
			case <-time.After(time.Duration(hcdns.Timeout) * time.Second):
				fmt.Printf("DNS: %s :%s timeout %d sec", hcdns.Host, hcdns.RecType)
				testing = false
			}
		}
	}

}
