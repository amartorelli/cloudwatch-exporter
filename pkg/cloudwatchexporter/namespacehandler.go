package cloudwatchexporter

import (
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/amartorelli/cloudwatch-exporter/pkg/conf"
	"github.com/amartorelli/cloudwatch-exporter/pkg/output"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/sirupsen/logrus"
)

var (
	errDimensionNotFound = errors.New("dimension not found")
)

var (
	mMetricsToFetchCounter = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "metrics_to_fetch_total",
			Help: "The total number of metrics to fetch",
		},
		[]string{"namespace"},
	)

	mUpdateMetricsErr = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "update_metrics_errors_total",
			Help: "The total number of errors when updating the metrics",
		},
		[]string{"namespace"},
	)

	mFetchErr = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fetch_errors_total",
			Help: "The total number of errors when fetching metrics from Cloudwatch",
		},
		[]string{"namespace"},
	)

	mAWSAPICallsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_api_calls_total",
			Help: "The total number of errors making API calls to Cloudwatch",
		},
		[]string{"namespace", "status"},
	)

	mAWSAPICallsTiming = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "aws_api_calls_duration_seconds",
			Help: "The total duration for Cloudwatch API calls",
		},
		[]string{"namespace", "status"},
	)
)

// getNamespaceMetrics initialises the metrics to be fetched by the namespaceHandler
func getNamespaceMetrics(conf []conf.Metric) []metric {
	metrics := make([]metric, 0)
	for _, m := range conf {
		// dimensions
		dims := make([]dimension, 0)
		for k, v := range m.Dimensions {
			dims = append(dims, dimension{name: k, value: v})
		}
		metrics = append(metrics, metric{name: m.Name, dimensions: dims})
	}
	return metrics
}

// newThreadTracker returns a new threadTracker
func newThreadTracker() *threadTracker {
	return &threadTracker{
		stop:       make(chan struct{}, 1),
		hasStopped: false,
	}
}

// newNamespaceHandler returns a new namespaceHandler
func newNamespaceHandler(name string, conf conf.Namespace, cw CloudwatchClient, ratelimiter *time.Ticker, fetchInterval int, output output.Output) *namespaceHandler {
	var hs = make(map[string]*threadTracker, 0)
	hs["manager"] = newThreadTracker()
	hs["handler"] = newThreadTracker()

	if fetchInterval <= 0 {
		fetchInterval = 60
	}

	tags := make(map[string]string)
	tags["namespace"] = name

	return &namespaceHandler{
		namespace:           name,
		metrics:             getNamespaceMetrics(conf.Metrics),
		cwClient:            cw,
		metricsToFetch:      make(map[string][][]*cloudwatch.Dimension, 0),
		metricsToFetchMutex: &sync.RWMutex{},
		fetchInterval:       fetchInterval,
		limiter:             ratelimiter,
		tags:                tags,
		threadTrackers:      hs,
		output:              output,
	}
}

// isMetricMonitored checks if a metric received from Cloudwatch is present in the
// configuration to be monitored.
func (nsh *namespaceHandler) isMetricMonitored(metric string) bool {
	for _, m := range nsh.metrics {
		if m.name == metric {
			return true
		}
	}
	return false
}

// addMetricsToFetch updates the metricsToFetch map with all the metrics we're interested in
func (nsh *namespaceHandler) addMetricsToFetch(mm []*cloudwatch.Metric) error {
	metricsToFetch := make(map[string][][]*cloudwatch.Dimension, 0)
	metricsToFetchCount := 0
	updated := false
	for _, resMetric := range mm {
		metricName := *resMetric.MetricName

		// if not monitored skip to the next metric in the resultset
		if !nsh.isMetricMonitored(metricName) {
			continue
		}

		/*
		   iterate through the config, if the metric name is the same and
		   we match the regexp for all the dimensions in the config for that metric definition
		   we add it to the metricsToFetch
		*/
		for _, metricDef := range nsh.metrics {
			matched, err := metricMatchesDefinition(resMetric, metricDef)
			if err != nil {
				return err
			}
			if !matched {
				continue
			}

			// add the metric to the list of metrics to fetch with the dimensions coming rom the metricDef
			// turn metricDef.dimensions into an array of cloudwatch.Dimension
			dims := make([]*cloudwatch.Dimension, 0)
			for _, d := range metricDef.dimensions {
				value, err := getDimensionValue(d.name, resMetric.Dimensions)
				if err != nil {
					logrus.Debug(err)
				}
				dims = append(dims, &cloudwatch.Dimension{Name: &d.name, Value: &value})
			}

			metricsToFetchCount++
			metricsToFetch[metricName] = append(metricsToFetch[metricName], dims)
			updated = true
		}
		mMetricsToFetchCounter.WithLabelValues(nsh.namespace).Inc()
	}

	if updated {
		nsh.metricsToFetchMutex.Lock()
		nsh.metricsToFetch = metricsToFetch
		nsh.metricsToFetchMutex.Unlock()
	}
	return nil
}

func getDimensionValue(dimName string, dimensions []*cloudwatch.Dimension) (string, error) {
	for _, d := range dimensions {
		if *d.Name == dimName {
			return *d.Value, nil
		}
	}
	return "", errDimensionNotFound
}

// updateMetricsToFetch triggers a reload of the metrics that need to be fetched
func (nsh *namespaceHandler) updateMetricsToFetch() error {
	logrus.Infof("updating metrics to fetch for namespace %s", nsh.namespace)
	var nt *string
	cwMetrics := make([]*cloudwatch.Metric, 0)
	for {
		start := time.Now()
		results, err := nsh.cwClient.ListMetrics(&cloudwatch.ListMetricsInput{
			Namespace: aws.String(nsh.namespace),
			NextToken: nt,
		})
		if err != nil {
			mAWSAPICallsTiming.WithLabelValues(nsh.namespace, "error").Observe(time.Since(start).Seconds())
			mAWSAPICallsCounter.WithLabelValues(nsh.namespace, "error").Inc()
			logrus.Errorf("%s, waiting 5s before retrying", err)
			time.Sleep(5 * time.Second)
			continue
		}
		mAWSAPICallsCounter.WithLabelValues(nsh.namespace, "success").Inc()
		nt = results.NextToken

		cwMetrics = append(cwMetrics, results.Metrics...)

		if nt == nil {
			break
		}
	}
	if err := nsh.addMetricsToFetch(cwMetrics); err != nil {
		return err
	}
	nsh.ready = true
	return nil
}

// manageMetricsToFetch reloads the metrics to fetch every 30 minutes
func (nsh *namespaceHandler) manageMetricsToFetch() {
	err := nsh.updateMetricsToFetch()
	if err != nil {
		mUpdateMetricsErr.WithLabelValues(nsh.namespace).Inc()
		logrus.Errorf("error updating metrics for handler %s: %s", nsh.namespace, err)
	}
	logrus.Debug(nsh.metricsToFetch)

	for {
		select {
		case <-time.After(30 * time.Minute):
			logrus.Infof("updating metrics to fetch for namespace %s", nsh.namespace)
			err := nsh.updateMetricsToFetch()
			if err != nil {
				mUpdateMetricsErr.WithLabelValues(nsh.namespace).Inc()
				logrus.Errorf("error updating metrics for handler %s: %s", nsh.namespace, err)
			}
			logrus.Debug(nsh.metricsToFetch)
		case <-nsh.threadTrackers["manager"].stop:
			nsh.threadTrackers["manager"].hasStopped = true
			logrus.Infof("manager for namespace %s stopped", nsh.namespace)
			return
		}
	}
}

// stopHandler triggers a stop for the namespaceHandler
func (nsh *namespaceHandler) stopHandler() {
	logrus.Infof("called stop on %s", nsh.namespace)

	close(nsh.threadTrackers["manager"].stop)
	close(nsh.threadTrackers["handler"].stop)
	nsh.output.Stop()

	for {
		stopped := true
		for _, v := range nsh.threadTrackers {
			stopped = stopped && v.hasStopped
		}
		if stopped {
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func dimensionsToMap(dims []*cloudwatch.Dimension) map[string]string {
	m := make(map[string]string, 0)
	for _, d := range dims {
		m[*d.Name] = *d.Value
	}
	return m
}

// fetchDatapoints handles the fetch of a singular metric in a namespace
func (nsh *namespaceHandler) fetchDatapoints(metric string, dims []*cloudwatch.Dimension) error {
	start := time.Now()
	params := &cloudwatch.GetMetricStatisticsInput{
		StartTime:  aws.Time(time.Now().Add(-10 * time.Minute)),
		EndTime:    aws.Time(time.Now()),
		MetricName: &metric,
		Namespace:  &nsh.namespace,
		Period:     aws.Int64(int64(60)),
		Dimensions: dims,
		Statistics: []*string{
			aws.String(cloudwatch.StatisticMaximum),
		},
	}

	resp, err := nsh.cwClient.GetMetricStatistics(params)
	if err != nil {
		mAWSAPICallsCounter.WithLabelValues(nsh.namespace, "error").Inc()
		mAWSAPICallsTiming.WithLabelValues(nsh.namespace, "error").Observe(time.Since(start).Seconds())
		logrus.Error(err)
	}
	mAWSAPICallsCounter.WithLabelValues(nsh.namespace, "success").Inc()
	mAWSAPICallsTiming.WithLabelValues(nsh.namespace, "success").Observe(time.Since(start).Seconds())

	for _, d := range resp.Datapoints {
		value := d.Maximum
		err := nsh.output.AddPoint(nsh.namespace, metric, *value, d.Timestamp, dimensionsToMap(dims))
		if err != nil {
			return err
		}
		logrus.Debugf("[%s] %s: %v", nsh.namespace, metric, value)
	}

	return nil
}

// fetch fetches metrics for this namespace handler
func (nsh *namespaceHandler) fetch() {
	for !nsh.ready {
		time.Sleep(3 * time.Second)
	}

	nsh.metricsToFetchMutex.RLock()
	for metricName, dimensions := range nsh.metricsToFetch {
		for _, dims := range dimensions {
			<-nsh.limiter.C
			err := nsh.fetchDatapoints(metricName, dims)
			if err != nil {
				mFetchErr.WithLabelValues(nsh.namespace).Inc()
				logrus.Errorf("error fetching metric %s for namespace %s: %s", metricName, nsh.namespace, err)
			}
		}
	}
	nsh.metricsToFetchMutex.RUnlock()
}

// startHandler is the main function that triggers a fetch every 5 minutes
// or stops in case it received a stop signal.
func (nsh *namespaceHandler) startHandler() {
	logrus.Infof("started fetching for namespace %s", nsh.namespace)
	nsh.fetch()

	for {
		select {
		case <-nsh.threadTrackers["handler"].stop:
			logrus.Infof("handler for namespace %s stopped", nsh.namespace)
			nsh.threadTrackers["handler"].hasStopped = true
			return
		case <-time.Tick(time.Duration(nsh.fetchInterval) * time.Second):
			nsh.fetch()
		}
	}
}

// metricMatchesDefinition checks if a Cloudwatch metric matches a metric definition in the config
func metricMatchesDefinition(cwMetric *cloudwatch.Metric, metricDef metric) (bool, error) {
	// dimMatchRegex holds the regular expression to apply to a single dimension, and keeps track if
	// it's been matched by the current analysed Cloudwatch metric cwMetric.
	type dimMatchRegex struct {
		matched bool
		regexp  string
	}

	if *cwMetric.MetricName != metricDef.name {
		return false, nil
	}

	/* for each dimension in the config, the regexp for the dim must match the dim in the api response.
	Initialise the bool checks map that keeps track of which dimensions have been matched for this metric.
	checks contains a map of dimension, so that we make sure that every dimension for this
	metric, that's being compared, matches.
	regexp is the regular expression we must apply to the dimension to check it's a match. In case the check for
	the dimension matches, its matched flag will be set to true.
	Once finished, if all dimMatchRegex's matched flag is true, the metric can go to the fetchedMetrics map.
	*/

	// init the checks helper structure
	checks := make(map[string]*dimMatchRegex, 0)
	for _, d := range metricDef.dimensions {
		checks[d.name] = &dimMatchRegex{matched: false, regexp: d.value}
	}

	// iterate over each dimension received from the API
	for _, cd := range cwMetric.Dimensions {
		_, ok := checks[*cd.Name]
		if !ok {
			continue
		}

		matched, err := regexp.MatchString(checks[*cd.Name].regexp, *cd.Value)
		if err != nil {
			return false, err
		}
		if matched {
			checks[*cd.Name].matched = true
		}
	}

	matched := true
	for _, v := range checks {
		matched = matched && v.matched
	}

	return matched, nil
}
