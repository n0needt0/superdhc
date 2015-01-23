package main

import (
	"fmt"
	"github.com/gorhill/cronexpr"
	"time"
)

func main() {
	cron := "*/22 * * * * * *"
	cronexp, err := cronexpr.Parse(cron)

	now := time.Now()

	if err != nil {
		fmt.Printf("error parsing cron %s", err)
	} else {
		//lets find a interval in seconds between 2 proposed stamps
		//this way we know interval difference and if next run is sooner than this difference
		//we make sure to set it to run on Now + diff
		fmt.Printf("CRON spec %s", cron)
		nextN := cronexp.NextN(now, 5)

		for _, v := range nextN {
			fmt.Println(v)
		}

		diffSec := int32(nextN[1].Unix()) - int32(nextN[0].Unix())

		fmt.Printf("CRON diff %d", diffSec)
		//next run now adds minimum number of seconds for next run
		//this is don so fast running tests on slow resources dont kill us

	}
}
