//THis contains all common parts to dns4 package
package dhc4

import (
	logging "github.com/op/go-logging"
	"time"
)

const (
	DHC_VER           = "4"
	USER_AGENT        = "FortiDirector/4 HealthCheck/4.0 (https://www.fortidirector.com/)"
	HEALTH_STATE_UP   = 1
	HEALTH_STATE_DOWN = 0

	HEALTH_STATE_NOT_CONFIGURED = -1
	HEALTH_STATE_NO_RESULTS     = 2
	HEALTH_STATE_NOT_ENABLED    = 3
	HEALTH_STATE_FLOPPING       = 4
	TIMEOUT_SEC                 = 10
	OK_PACKET_LOSS_P            = 0
	DEFAULT_PACKET_CNT          = 4
	DEFAULT_PACKET_SIZE         = 34
	DEFAULT_DNS_REC             = "DNS_ANY"
	DEFAULT_CRON_1MIN           = "* */1 * * * * *"
	UNKNOWN                     = "UNKNOWN"
	ONE_MINUTE_SEC              = 60
	SEC_30                      = 30
	SEC_15                      = 15
	SEC_10                      = 10
)

var log = logging.MustGetLogger("logfile")

func validateNet(strnet string) bool {
	if strnet == "tcp" || strnet == "tcp4" || strnet == "ipv6" {
		return true
	}
	return false
}

func makeTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
