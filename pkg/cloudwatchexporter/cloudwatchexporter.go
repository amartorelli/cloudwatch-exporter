package cloudwatchexporter

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/amartorelli/cloudwatch-exporter/pkg/conf"
	"github.com/amartorelli/cloudwatch-exporter/pkg/output"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// NewCloudwatchExporterConfig returns a string of the config file for
// CloudwatchExporter read from a file
func NewCloudwatchExporterConfig(path string) (*conf.CloudwatchExporter, error) {
	conf := &conf.CloudwatchExporter{}
	c, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(c, &conf)
	if err != nil {
		return nil, err
	}

	return conf, nil
}

// NewCloudwatchClient returns a new implementation of a cloudwatchClient
func NewCloudwatchClient(region string) CloudwatchClient {
	// create Cloudwatch client
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	if region != "" {
		sess.Config.Region = aws.String(region)
	}
	return cloudwatch.New(sess)
}

// NewCloudwatchExporter returns a CloudwatchExporter object
func NewCloudwatchExporter(conf conf.CloudwatchExporter, cwClient CloudwatchClient) (*CloudwatchExporter, error) {
	// limiter
	interval := 1 / float64(conf.Rate) * 1000
	rl := time.NewTicker(time.Duration(interval) * time.Millisecond)

	nshs := make([]*namespaceHandler, 0)

	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":9099", nil)

	for name, ns := range conf.Namespaces {
		o, err := output.NewInfluxDBOutput(fmt.Sprintf("output-%s", name), conf.Output.Proto, conf.Output.Addr, conf.Output.DB, conf.Output.User, conf.Output.Pass, conf.Output.Interval)
		if err != nil {
			return nil, err
		}
		nshs = append(nshs, newNamespaceHandler(name, ns, cwClient, rl, conf.FetchInterval, o))
	}

	return &CloudwatchExporter{namespaceHandlers: nshs, wg: &sync.WaitGroup{}}, nil
}

// Execute starts the namespacehandlers
func (cwe *CloudwatchExporter) Execute() {
	logrus.Info("starting namespaces...")
	for _, nsh := range cwe.namespaceHandlers {
		cwe.wg.Add(1)
		go nsh.manageMetricsToFetch()
		go nsh.startHandler()
		go nsh.output.Start()
	}

	cwe.wg.Wait()
}

// Stop terminates the program
func (cwe *CloudwatchExporter) Stop() {
	for _, nsh := range cwe.namespaceHandlers {
		nsh.stopHandler()
		cwe.wg.Done()
	}
}
