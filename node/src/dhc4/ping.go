/**
*	PING DHC4 Module by AY
**/

package dhc4

import (
	"GoStats/stats"
	"errors"
	"fmt"
	"github.com/tatsushid/go-fastping"
	"net"
	"strconv"
	"strings"
	"time"
)

//ping stuff
type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

type HcPing struct {
	Host    string
	Timeout int     //timeout sec
	Plossok float32 //percent loss
	Packets int     //we use 4 iterations to test by default up to 10 otherwise
	Size    int     //packet byte size 10 to 100
	Res     map[string]interface{}
}

func NewPing(meta map[string]interface{}) (*HcPing, error) {
	hcping := &HcPing{
		"",
		TIMEOUT_SEC,
		OK_PACKET_LOSS_P,
		PACKET_CNT,
		PACKET_SIZE,
		make(map[string]interface{}),
	}

	hcping.Res["state"] = HEALTH_STATE_DOWN //it is not working unless noted otherwise
	hcping.Res["loss_%"] = float64(100)

	err := hcping.loadMeta(meta)
	if err != nil {
		return hcping, err
	}
	return hcping, nil
}

func (hcping *HcPing) loadMeta(meta map[string]interface{}) error {
	//required settings
	if el, ok := meta["host"]; !ok {
		//no host fail
		return errors.New("PING: Invalid host value")
	} else {
		v := el.(string)
		hcping.Host = v
	}

	//get optional settings
	if el, ok := meta["timeout"]; ok == true {
		v := el.(int)
		if v < 2 || v > 10 {
			v = 2
		}
		hcping.Timeout = v
	}
	//parse value
	if el, ok := meta["ok"]; ok == true {
		//parse interface
		if el, ok := el.(map[string]interface{}); ok == true {
			//parse string out of map
			if el, ok := el["ploss"]; ok == true {
				//parce out a float
				//type ensure
				if el, ok := el.(string); ok == true {
					//convert string to float
					if v, err := strconv.ParseFloat(el, 32); err == nil {
						v := float32(v)
						if v < 0 || v > 100 {
							v = 2.5
						}
						hcping.Plossok = v
					}
				}
			}
		}
	}

	if el, ok := meta["packets"]; ok == true {
		if v, err := strconv.Atoi(el.(string)); err == nil {

			if v < 1 || v > 10 {
				v = 4
			}
			hcping.Packets = v
		}
	}

	if el, ok := meta["size"]; ok == true {
		if v, err := strconv.Atoi(el.(string)); err == nil {
			if v < 2 || v > 92 {
				v = 35
			}
			hcping.Size = v
		}
	}
	return nil
}

func (hcping *HcPing) DoTest(result chan map[string]interface{}) error {

	res := make(map[string]interface{})

	packetsDelivered := 0

	//this guy will held our stats for us
	var rt_stats stats.Stats
	dns_start := makeTimestamp()

	netProto := "ip4:icmp"
	if strings.Index(hcping.Host, ":") != -1 {
		netProto = "ip6:ipv6-icmp"
	}

	res["step"] = "dns"

	IpAddr, err := net.ResolveIPAddr(netProto, hcping.Host)
	if err != nil {
		res["msg"] = fmt.Sprintf("PING:DNS: %s", err)
		result <- res
		return nil
	}

	res["dns_ms"] = makeTimestamp() - dns_start
	res["step"] = "test"

	p := fastping.NewPinger()

	results := make(map[string]*response)
	results[IpAddr.String()] = nil

	p.AddIPAddr(IpAddr)

	onRecv, onIdle := make(chan *response), make(chan bool)
	p.OnRecv = func(addr *net.IPAddr, t time.Duration) {
		onRecv <- &response{addr: addr, rtt: t}
	}
	p.OnIdle = func() {
		onIdle <- true
	}

	p.MaxRTT = time.Second
	i := 0
	p.RunLoop()

loop:

	for i < hcping.Packets {
		select {

		case res := <-onRecv:
			if _, ok := results[res.addr.String()]; ok {
				results[res.addr.String()] = res
			}
		case <-onIdle:
			for host, r := range results {
				if r == nil {
					rt_stats.Update(0)
					log.Warning("PING %s : unreachable %v", host, time.Now())

				} else {
					rt_ms := int(r.rtt.Nanoseconds() / int64(time.Millisecond))
					rt_stats.Update(float64(rt_ms))
					packetsDelivered++
					log.Debug("PING %d: %s : %v %v", i, host, r.rtt.Nanoseconds()/int64(time.Millisecond), time.Now())
				}
				results[host] = nil
				i++
			}
		case <-p.Done():
			if err := p.Err(); err != nil {
				msg := fmt.Sprintf("PING failed: %s", err)
				log.Warning(msg)
				res["msg"] = msg
			}
			break loop
		}
	}
	p.Stop()

	res["sent"] = hcping.Packets
	res["sent_ok"] = packetsDelivered

	loss := float32(100 * (hcping.Packets - packetsDelivered) / hcping.Packets)
	res["loss_%"] = loss
	if loss < hcping.Plossok {
		res["state"] = HEALTH_STATE_UP
	} else {
		res["msg"] = fmt.Sprintf("%f%% packet loss", loss)
	}

	res["rt_min"] = rt_stats.Min()
	res["rt_max"] = rt_stats.Max()
	res["rt_avg"] = rt_stats.Mean()
	res["rt_std"] = rt_stats.SampleStandardDeviation()
	result <- res

	return nil
}
