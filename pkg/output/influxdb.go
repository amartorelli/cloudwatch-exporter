package output

import (
	"errors"
	"fmt"
	"strings"
	"time"

	client "github.com/influxdata/influxdb/client/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

var (
	mPoints     = "points"
	mPointsErrs = "points_errors"
)

var (
	mPointsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "points_total",
			Help: "The total number of errors making API calls to Cloudwatch",
		},
		[]string{"output", "status"},
	)

	mPointsTiming = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "points_duration_seconds",
			Help: "The total duration for Cloudwatch API calls",
		},
		[]string{"output", "status"},
	)
)

// InfluxDB is the structure where we manage the metrics for the application
type InfluxDB struct {
	name       string
	database   string
	client     client.Client
	batch      client.BatchPoints
	interval   int
	tags       map[string]string
	stop       chan struct{}
	hasStopped bool
}

var (
	// ErrUnsupportedProtocol is returned when the metrics protocol is not HTTP nor UDP
	ErrUnsupportedProtocol = errors.New("unsupported protocol")
)

// NewInfluxDBOutput returns a new Metrics object
func NewInfluxDBOutput(name, proto, addr, db, user, pass string, interval int) (*InfluxDB, error) {
	var c client.Client
	var err error

	switch strings.ToLower(proto) {
	case "http":
		c, err = client.NewHTTPClient(client.HTTPConfig{
			Addr:     addr,
			Username: user,
			Password: pass,
		})
		if err != nil {
			return nil, err
		}
	case "udp":
		c, err = client.NewUDPClient(client.UDPConfig{Addr: addr})
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrUnsupportedProtocol
	}

	if err != nil {
		return nil, err
	}

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  db,
		Precision: "s",
	})
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string, 0)
	tags["name"] = name

	if interval <= 0 {
		interval = 60
	}

	return &InfluxDB{
		name:       name,
		database:   db,
		client:     c,
		batch:      bp,
		interval:   interval,
		tags:       tags,
		stop:       make(chan struct{}, 1),
		hasStopped: false,
	}, nil
}

// Start starts the routine that regularly writes metrics to InfluxDB
func (m *InfluxDB) Start() {
	backoff := 1
	var batchLen float64
	for {
		select {
		case <-m.stop:
			logrus.Infof("%s metrics has received a stop signal, exiting...", m.name)
			m.hasStopped = true
			return
		case <-time.After(time.Duration(m.interval*backoff) * time.Second):
			batchLen = float64(len(m.batch.Points()))
			if batchLen == 0 {
				mPointsGauge.WithLabelValues(m.name, "success").Set(0)
				continue
			}

			logrus.Infof("%s metrics writing batch (%f metrics)", m.name, batchLen)
			err := m.client.Write(m.batch)
			if err != nil {
				mPointsGauge.WithLabelValues(m.name, "error").Set(batchLen)
				logrus.Errorf("%s error writing batch (%f metrics) %s", m.name, batchLen, err)
				// implementing a simple backoff in case writing the batch errors
				if backoff < 5 {
					backoff++
					logrus.Infof("%s metrics, an error occurred sending metrics, increasing interval to %d seconds", m.name, m.interval*backoff)
					continue
				}
			}
			mPointsGauge.WithLabelValues(m.name, "success").Set(batchLen)

			// re-create the batch if the write was successful or in case the backoff got to 5.
			bp, err := client.NewBatchPoints(client.BatchPointsConfig{
				Database:  m.database,
				Precision: "s",
			})
			if err != nil {
				logrus.Error(err)
			}
			m.batch = bp

			// reset interval if everything went ok
			if backoff > 1 {
				backoff = 1
				logrus.Infof("%s metrics, recovered, resetting interval to %d seconds", m.name, m.interval*backoff)
			}
		}
	}
}

// Stop stops the metrics thread
func (m *InfluxDB) Stop() {
	close(m.stop)
	for !m.hasStopped {
		time.Sleep(1 * time.Second)
	}
}

func standardiseMetricName(metric string) string {
	chars := []string{"/", "."}

	metricName := metric
	for _, c := range chars {
		metricName = strings.Replace(metricName, c, "_", -1)
	}
	return metricName
}

// AddPoint adds a point to the metrics batch
func (m *InfluxDB) AddPoint(name, field string, value float64, timestamp *time.Time, tags map[string]string) error {
	fields := map[string]interface{}{
		"value": value,
	}
	metricName := standardiseMetricName(fmt.Sprintf("%s_%s", name, field))

	pt, err := client.NewPoint(metricName, tags, fields, *timestamp)
	if err != nil {
		mPointsGauge.WithLabelValues(m.name, "error").Set(0)
		return err
	}

	m.batch.AddPoint(pt)
	return nil
}
