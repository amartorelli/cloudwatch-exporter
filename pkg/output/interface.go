package output

import "time"

// Output provides an output interface to write metrics to
type Output interface {
	Start()
	Stop()
	AddPoint(name, field string, value float64, timestamp *time.Time, tags map[string]string) error
}
