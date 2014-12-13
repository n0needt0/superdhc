package dhc4

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type DnsResolver struct {
}

func NewDnsResolver() *DnsResolver {
	return &DnsResolver{}
}

func lookupDns(host string, lookup chan interface{}) {
	time.Sleep(time.Duration(50) * time.Millisecond)
	netProto := "ip4:icmp"

	if strings.Index(host, ":") != -1 {
		netProto = "ip6:ipv6-icmp"
	}

	IP, err := net.ResolveIPAddr(netProto, host)
	if err != nil {
		lookup <- err
	}

	lookup <- IP

}

func (d *DnsResolver) ResolveIPWithTimeout(host string, timeout_ms int) (*net.IPAddr, error) {

	//see if it is ip
	if ip := net.ParseIP(host); ip != nil {
		netProto := "ip4:icmp"

		if strings.Index(host, ":") != -1 {
			netProto = "ip6:ipv6-icmp"
		}

		return net.ResolveIPAddr(netProto, host)

	}

	//otherwise try safe dns

	resolved := make(chan interface{})
	go lookupDns(host, resolved)
	//sit and wait
	for {
		select {
		case msg := <-resolved:
			switch msg.(type) {
			case *net.IPAddr: //good enough
				return msg.(*net.IPAddr), nil
			case error: //error
				return &net.IPAddr{}, msg.(error)
			default: //unkn0own type
				return &net.IPAddr{}, errors.New("wrong message from resolver")
			}
		case <-time.After(time.Duration(timeout_ms) * time.Millisecond):
			return &net.IPAddr{}, errors.New(fmt.Sprintf("%s dns timeout %d ms", host, timeout_ms))
		}
	}

}
