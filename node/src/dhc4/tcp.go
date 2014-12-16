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
	if el, ok := meta["host"]; !ok {
		//no host fail
		return errors.New("TCP: Invalid host value")
	} else {
		v := el.(string)
		hctcp.Host = v
	}

	if el, ok := meta["port"]; !ok {
		//no host fail
		return errors.New("TCP: Invalid port value")
	} else {
		v := el.(string)
		hctcp.Port = v
	}

	//get optional settings
	if el, ok := meta["timeout"]; ok == true {
		v := el.(int)
		if v < 2 || v > 10 {
			v = 2
		}
		hctcp.Timeout = v
	}

	if el, ok := meta["proto"]; ok == true {
		v := el.(string)
		hctcp.Proto = v
	} else {
		if strings.Index(hctcp.Host, ":") != -1 {
			hctcp.Proto = "tcp6"
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
		res["msg"] = fmt.Sprintf("TCP:DNS: %s", err)
		result <- res
		return nil
	}

	res["dns_ms"] = makeTimestamp() - dns_start
	res["step"] = "test"

	c, err := net.DialTCP(hctcp.Proto, nil, TCPAddr)
	if err != nil {
		res["state"] = HEALTH_STATE_DOWN
		msg := fmt.Sprintf("TCP %s %s:%s failed: %s", hctcp.Proto, hctcp.Host, hctcp.Port, err.Error())
		log.Warning(msg)
		res["msg"] = msg
		result <- res
	} else {
		c.Close()
		res["state"] = HEALTH_STATE_UP
		result <- res
	}
	return nil
}
