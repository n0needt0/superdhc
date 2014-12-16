//THis contains all common parts to dns4 package
package dhc4

import (
	logging "github.com/op/go-logging"
	"time"
)

const (
	HEALTH_STATE_UP   = 1
	HEALTH_STATE_DOWN = 0
	TIMEOUT_SEC       = 6
	OK_PACKET_LOSS_P  = 0
	PACKET_CNT        = 4
	PACKET_SIZE       = 34
)

var log = logging.MustGetLogger("logfile")

func makeTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
