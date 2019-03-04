package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/amartorelli/cloudwatch-exporter/pkg/conf"

	"github.com/amartorelli/cloudwatch-exporter/pkg/cloudwatchexporter"

	"github.com/namsral/flag"

	"github.com/sirupsen/logrus"
)

func overrideWithEnv(conf *conf.CloudwatchExporter) {
	// override output
	if os.Getenv("OUTPUT_ADDRESS") != "" {
		conf.Output.Addr = os.Getenv("OUTPUT_ADDRESS")
	}

	if os.Getenv("OUTPUT_DATABASE") != "" {
		conf.Output.DB = os.Getenv("OUTPUT_DATABASE")
	}

	if os.Getenv("OUTPUT_USER") != "" {
		conf.Output.User = os.Getenv("OUTPUT_USER")
	}

	if os.Getenv("OUTPUT_PASSWORD") != "" {
		conf.Output.Pass = os.Getenv("OUTPUT_PASSWORD")
	}
}

func main() {
	// config flags
	var confFile = flag.String("conf", "./conf.yml", "path to config file")
	var loglevel = flag.String("loglevel", "info", "log level (debug/info/warn/fatal)")
	flag.Parse()

	// logging
	formatter := &logrus.TextFormatter{
		FullTimestamp: true,
	}
	logrus.SetFormatter(formatter)

	switch *loglevel {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "fatal":
		logrus.SetLevel(logrus.FatalLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	// signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// allow the conf path to be loaded from environment variable
	if os.Getenv("CONF") != "" {
		*confFile = os.Getenv("CONF")
	}

	conf, err := cloudwatchexporter.NewCloudwatchExporterConfig(*confFile)
	if err != nil {
		logrus.Fatal(err)
	}

	overrideWithEnv(conf)

	cwe, err := cloudwatchexporter.NewCloudwatchExporter(
		*conf,
		cloudwatchexporter.NewCloudwatchClient(conf.Region),
	)
	if err != nil {
		logrus.Fatal(err)
	}

	// signals handler
	go func() {
		select {
		case sig := <-sigs:
			logrus.Infof("received signal %s, gracefully shutting down...", sig)
			cwe.Stop()
		}
	}()

	cwe.Execute()
}
