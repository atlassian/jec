package main

import (
	"flag"
	"fmt"
	"github.com/atlassian/jec/conf"
	"github.com/atlassian/jec/queue"
	"github.com/atlassian/jec/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

var metricAddr = flag.String("jec-metrics", "7070", "The address to listen on for HTTP requests.")
var defaultLogFilepath = filepath.Join("/var", "log", "jec", "jec"+strconv.Itoa(os.Getpid())+".log")

var JECVersion string
var JECCommitVersion string

func main() {

	logrus.SetFormatter(conf.PrepareLogFormat())

	err := os.Chmod(filepath.Join("/var", "log", "jec"), 0744)
	if err != nil {
		logrus.Warn(err)
	}

	logger := &lumberjack.Logger{
		Filename:  defaultLogFilepath,
		MaxSize:   10, // MB
		MaxAge:    10, // Days
		LocalTime: true,
	}

	logrus.SetOutput(io.MultiWriter(os.Stdout, logger))

	logrus.Infof("JEC version is %s", JECVersion)
	logrus.Infof("JEC commit version is %s", JECCommitVersion)

	go util.CheckLogFile(logger, time.Second*10)

	configuration, err := conf.Read()
	if err != nil {
		logrus.Fatalf("Could not read configuration: %s", err)
	}

	logrus.SetLevel(configuration.LogrusLevel)

	flag.Parse()
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		logrus.Infof("JEC-metrics serves in http://localhost:%s/metrics.", *metricAddr)
		logrus.Error("JEC-metrics error: ", http.ListenAndServe(":"+*metricAddr, nil))
	}()

	queueProcessor := queue.NewProcessor(configuration)
	queue.UserAgentHeader = fmt.Sprintf("%s/%s %s (%s/%s)", JECVersion, JECCommitVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH)

	go func() {
		if configuration.AppName != "" {
			logrus.Infof("%s is starting.", configuration.AppName)
		}
		err = queueProcessor.Start()
		if err != nil {
			logrus.Fatalln(err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-signals:
		logrus.Infof("JEC will be stopped gracefully.")
		err := queueProcessor.Stop()
		if err != nil {
			logrus.Fatalln(err)
		}
	}

	os.Exit(0)
}
