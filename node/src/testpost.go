package main

import (
	curl "github.com/andelf/go-curl"
	"time"
)

const POST_DATA = "a_test_data_only"

var sent = false

func main() {
	// init the curl session
	easy := curl.EasyInit()
	defer easy.Cleanup()

	easy.Setopt(curl.OPT_URL, "https://4.34.84.212/SC/LoginForm.aspx")
	easy.Setopt(curl.OPT_VERBOSE, true)
	easy.Setopt(curl.OPT_SSL_VERIFYHOST, false)
	easy.Setopt(curl.OPT_SSL_VERIFYPEER, false)

	if err := easy.Perform(); err != nil {
		println("ERROR: ", err.Error())
	}

	time.Sleep(10000) // wait gorotine
}
