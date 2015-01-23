/**
* DNS DHC4 Module by AY
**/

package dhc4

import (
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"net"
)

var valid_records = []string{"DNS_A", "DNS_CNAME", "DNS_HINFO", "DNS_MX", "DNS_NS", "DNS_PTR", "DNS_SOA", "DNS_TXT", "DNS_AAAA", "DNS_SRV", "DNS_NAPTR", "DNS_A6", "DNS_ALL", "DNS_ANY"}

type HcDns struct {
	Host    string
	RecType string
	Timeout int //timeout sec
	Res     map[string]interface{}
}

func NewDns(meta map[string]interface{}) (*HcDns, error) {
	hcdns := &HcDns{
		"",
		valid_records[len(valid_records)-1],
		TIMEOUT_SEC,
		make(map[string]interface{}),
	}

	hcdns.Res["state"] = HEALTH_STATE_DOWN //it is not working unless noted otherwise

	err := hcdns.loadMeta(meta)
	if err != nil {
		return hcdns, err
	}

	return hcdns, nil
}

func (hcdns *HcDns) loadMeta(meta map[string]interface{}) error {
	//required settings
	if el, ok := meta["host"]; ok {
		if _, ok := el.(string); ok {
			v := el.(string)
			hcdns.Host = v
		} else {
			return errors.New("DNS: Invalid host value")
		}
	} else {
		return errors.New("DNS: Invalid host value")
	}

	hcdns.Timeout = TIMEOUT_SEC
	if el, ok := meta["timeout"]; ok == true {
		if v, ok := el.(int); ok {
			v = el.(int)
			if v > 2 && v < 11 {
				hcdns.Timeout = v
			}
		}
	}

	if el, ok := meta["record_type"]; ok == true {
		if _, ok := el.(string); ok {
			requested := el.(string)
			for _, valid := range valid_records {
				if valid == requested {
					hcdns.RecType = requested
				}
			}
		}
	}

	return nil
}

func (hcdns *HcDns) DoTest(result chan map[string]interface{}) error {

	time_start := makeTimestamp()

	res := make(map[string]interface{})

	config, _ := dns.ClientConfigFromFile("/etc/resolv.conf")

	c := new(dns.Client)

	m := new(dns.Msg)

	q := dns.TypeANY

	switch hcdns.RecType {
	case "DNS_A":
		q = dns.TypeA
	case "DNS_CNAME":
		q = dns.TypeCNAME
	case "DNS_HINFO":
		q = dns.TypeHINFO
	case "DNS_MX":
		q = dns.TypeMX
	case "DNS_NS":
		q = dns.TypeNS
	case "DNS_PTR":
		q = dns.TypePTR
	case "DNS_SOA":
		q = dns.TypeSOA
	case "DNS_TXT":
		q = dns.TypeTXT
	case "DNS_AAAA":
		q = dns.TypeAAAA
	case "DNS_SRV":
		q = dns.TypeSRV
	case "DNS_NAPTR":
		q = dns.TypeSRV
	case "DNS_A6": //NOT used anymore
		q = dns.TypeAAAA
	}

	m.SetQuestion(dns.Fqdn(hcdns.Host), q)

	m.RecursionDesired = true

	r, _, err := c.Exchange(m, net.JoinHostPort(config.Servers[0], config.Port))

	res["time_ms"] = makeTimestamp() - time_start

	if r == nil {
		res["state"] = HEALTH_STATE_DOWN
		msg := fmt.Sprintf("DNS %s %s:%s failed: %s", hcdns.Host, hcdns.RecType, err.Error())
		log.Warning(msg)
		res["msg"] = msg
		result <- res
		return nil
	}

	if r.Rcode != dns.RcodeSuccess {
		res["state"] = HEALTH_STATE_DOWN
		msg := fmt.Sprintf("DNS %s %s invalid answer ", hcdns.Host, hcdns.RecType)
		log.Warning(msg)
		res["msg"] = msg
		result <- res
		return nil

	}
	// Stuff must be in the answer section
	cnt := 0
	dnsrec := ""
	for _, a := range r.Answer {
		cnt++
		dnsrec = dnsrec + a.String() + "\n"
	}
	res["dnsrec"] = dnsrec

	if cnt > 0 {
		res["state"] = HEALTH_STATE_UP
		result <- res
		return nil
	}

	//if we here obviuously nothing returned :()

	res["state"] = HEALTH_STATE_DOWN
	msg := fmt.Sprintf("DNS %s %s invalid answer ", hcdns.Host, hcdns.RecType)
	log.Warning(msg)
	res["msg"] = msg
	result <- res
	return nil
}
