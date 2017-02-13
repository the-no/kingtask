package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"

	"github.com/flike/golog"
	"github.com/the-no/kingtask/broker"
	"github.com/the-no/kingtask/config"
)

var configFile *string = flag.String("config", "./etc/broker.yaml", "broker config file")
var logLevel *string = flag.String("log-level", "", "log level [debug|info|warn|error], default error")

const (
	sysLogName = "sys.log"
	MaxLogSize = 1024 * 1024 * 1024
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	if len(*configFile) == 0 {
		fmt.Println("must use a config file")
		return
	}

	cfg, err := config.ParseBrokerConfigFile(*configFile)
	if err != nil {
		fmt.Printf("parse config file error:%v\n", err.Error())
		return
	}

	//when the log file size greater than 1GB, kingtask will generate a new file
	if len(cfg.LogPath) != 0 {
		sysFilePath := path.Join(cfg.LogPath, sysLogName)
		sysFile, err := golog.NewRotatingFileHandler(sysFilePath, MaxLogSize, 1)
		if err != nil {
			fmt.Printf("new log file error:%v\n", err.Error())
			return
		}
		golog.GlobalLogger = golog.New(sysFile, golog.Lfile|golog.Ltime|golog.Llevel)
	}

	if *logLevel != "" {
		setLogLevel(*logLevel)
	} else {
		setLogLevel(cfg.LogLevel)
	}

	var bk *broker.Broker
	bk, err = broker.NewBroker(cfg)
	if err != nil {
		golog.Error("main", "main", err.Error(), 0)
		golog.GlobalLogger.Close()
		bk.Close()
		return
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	go func() {
		sig := <-sc
		golog.Info("main", "main", "Got signal", 0, "signal", sig)
		golog.GlobalLogger.Close()
		bk.Close()
	}()
	golog.Info("main", "main", "Broker start!", 0)
	bk.Run()
}

func setLogLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		golog.GlobalLogger.SetLevel(golog.LevelDebug)
	case "info":
		golog.GlobalLogger.SetLevel(golog.LevelInfo)
	case "warn":
		golog.GlobalLogger.SetLevel(golog.LevelWarn)
	case "error":
		golog.GlobalLogger.SetLevel(golog.LevelError)
	default:
		golog.GlobalLogger.SetLevel(golog.LevelError)
	}
}
