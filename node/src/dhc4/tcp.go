/**
* TCP DHC4 Module by AY
**/

package dhc4

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

type HcTcp struct {
	Host    string
	Port    string
	Proto   string //"tcp", "tcp6", "udp", , "udp6"
	Timeout int    //timeout sec
	Res     map[string]interface{}
}

func NewTcp(meta map[string]interface{}) (*HcTcp, error) {
	hctcp := &HcTcp{
		"",
		"",
		"tcp4",
		TIMEOUT_SEC,
		make(map[string]interface{}),
	}

	hctcp.Res["state"] = HEALTH_STATE_DOWN //it is not working unless noted otherwise

	err := hctcp.loadMeta(meta)
	if err != nil {
		return hctcp, err
	}
	return hctcp, nil
}

func (hctcp *HcTcp) loadMeta(meta map[string]interface{}) error {
	//required settings
	//required settings
	if el, ok := meta["host"]; ok {
		if _, ok := el.(string); ok {
			v := el.(string)
			hctcp.Host = strings.Trim(v, " ")
		} else {
			return errors.New("Invalid host value")
		}
	} else {
		return errors.New("Invalid host value")
	}

	if el, ok := meta["port"]; !ok {
		//no host fail
		return errors.New("Invalid port value")
	} else {
		v := ""
		if _, ok := el.(string); ok {
			v = el.(string)
		}
		if _, ok := el.(int); ok {
			v = fmt.Sprintf("%d", el.(int))
		}
		hctcp.Port = strings.Trim(v, " ")
	}

	hctcp.Timeout = TIMEOUT_SEC
	if el, ok := meta["timeout"]; ok == true {
		if v, ok := el.(int); ok {
			v = el.(int)
			if v > 2 && v < 11 {
				hctcp.Timeout = v
			}
		}
	}

	if el, ok := meta["proto"]; ok == true {
		if _, ok := el.(string); ok {
			v := el.(string)
			if validateNet(v) {
				hctcp.Proto = v
			}
		}
	}

	return nil
}

func (hctcp *HcTcp) DoTest(result chan map[string]interface{}) error {

	res := make(map[string]interface{})

	dns_start := makeTimestamp()

	res["step"] = "dns"

	TCPAddr, err := net.ResolveTCPAddr(hctcp.Proto, fmt.Sprintf("%s:%s", hctcp.Host, hctcp.Port))
	if err != nil {
		res["msg"] = fmt.Sprintf("TCP error DNS: %s, proto: %s, host: %s, port: %s", err, hctcp.Proto, hctcp.Host, hctcp.Port)
		result <- res
		return nil
	}

	res["dns_ms"] = makeTimestamp() - dns_start
	res["step"] = "test"

	c, err := net.DialTCP(hctcp.Proto, nil, TCPAddr)
	if err != nil {
		res["state"] = HEALTH_STATE_DOWN
		msg := fmt.Sprintf("TCP error: %s, proto: %s, host: %s, port: %s", err.Error(), hctcp.Proto, hctcp.Host, hctcp.Port)
		log.Warning(msg)
		res["time_ms"] = makeTimestamp() - dns_start
		res["msg"] = msg
		result <- res
	} else {
		c.Close()
		res["time_ms"] = makeTimestamp() - dns_start
		res["state"] = HEALTH_STATE_UP
		result <- res
	}
	return nil
}
