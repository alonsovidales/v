package main

import (
	"fmt"
	"github.com/alonsovidales/pit/cfg"
	"github.com/alonsovidales/pit/log"
	"github.com/alonsovidales/v/charont"
	"github.com/alonsovidales/v/hades"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Execute: v <env> [log|nolog] [collect|train|play] <train_file>")
		return
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	runningMode := os.Args[3]
	cfg.Init("v", os.Args[1])

	if os.Args[2] == "log" {
		log.SetLogger(
			log.Levels[cfg.GetStr("logger", "level")],
			cfg.GetStr("logger", "log_file"),
			cfg.GetInt("logger", "max_log_size_mb"),
		)
	}

	var collector charont.Int
	var err error

	if runningMode != "train" {
		collector, err = charont.InitOandaApi(
			cfg.GetStr("oanda", "token"),
			int(cfg.GetInt("oanda", "account-id")),
			strings.Split(cfg.GetStr("oanda", "currencies"), ","),
			cfg.GetStr("oanda", "exanges-log"),
		)
		if err != nil {
			log.Fatal("The API connection can't be loaded")
		}
	} else {
		if len(os.Args) < 4 {
			fmt.Println("<train_file> not specified")
		}
		collector = charont.GetMock(
			os.Args[4],
			1000,
			strings.Split(cfg.GetStr("oanda", "currencies"), ","),
			int(cfg.GetInt("mock", "http-port")),
		)
	}
	if runningMode != "collect" {
		hades.GetHades(
			int(cfg.GetInt("traders-window", "total")),
			int(cfg.GetInt("traders-window", "from-size")),
			collector,
			int(cfg.GetInt("traders-window", "units-to-use")),
			int(cfg.GetInt("traders-window", "min-samples-to-consider")),
			int(cfg.GetInt("traders-window", "last-ops-to-considerer")),
			int(cfg.GetInt("traders-window", "max-traders-that-can-play")))
	}

	log.Info("System started...")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
	// Block until a signal is received.
	<-c

	log.Info("Stopping all the services")
	collector.CloseAllOpenOrders()
}