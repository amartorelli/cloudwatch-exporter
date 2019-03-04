package cloudwatchexporter

import (
	"sync"
	"time"

	"github.com/amartorelli/cloudwatch-exporter/pkg/output"

	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

// CloudwatchExporter is the main object of the program
type CloudwatchExporter struct {
	namespaceHandlers []*namespaceHandler
	wg                *sync.WaitGroup
}

type namespaceHandler struct {
	namespace           string
	metrics             []metric
	cwClient            CloudwatchClient
	metricsToFetch      map[string][][]*cloudwatch.Dimension
	metricsToFetchMutex *sync.RWMutex
	fetchInterval       int
	ready               bool
	limiter             *time.Ticker
	tags                map[string]string
	threadTrackers      map[string]*threadTracker
	output              output.Output
}

type metric struct {
	name       string
	dimensions []dimension
}

type dimension struct {
	name  string
	value string
}

type threadTracker struct {
	stop       chan struct{}
	hasStopped bool
}
