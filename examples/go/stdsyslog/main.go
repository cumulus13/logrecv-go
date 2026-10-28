//go:build !windows && !plan9

package main

import (
	"log"
	"log/syslog"
)

func main() {
	w, err := syslog.Dial("udp", "127.0.0.1:5514", syslog.LOG_USER|syslog.LOG_DEBUG, "my-go-app")
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()

	w.Debug("cache miss")
	w.Info("user logged in")
	w.Warning("disk almost full")
	w.Err("payment failed")
	w.Crit("queue stalled")
}
