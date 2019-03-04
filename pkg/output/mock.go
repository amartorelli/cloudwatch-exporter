package output

import "time"

// MockOutput is used to mock the output in unit tests
type MockOutput struct {
	Batch []MockPoint
}

// MockPoint mocks an InfluxDB point
type MockPoint struct {
	Name      string
	Field     string
	Value     float64
	Timestamp *time.Time
	Tags      map[string]string
}

// NewMockOutput returns a new MockOutput
func NewMockOutput() *MockOutput {
	return &MockOutput{Batch: make([]MockPoint, 0)}
}

// Start does nothing, it's defined purely to match the interface
func (mo *MockOutput) Start() {}

// Stop does nothing, it's defined purely to match the interface
func (mo *MockOutput) Stop() {}

// AddPoint adds a point to the batch
func (mo *MockOutput) AddPoint(name, field string, value float64, timestamp *time.Time, tags map[string]string) error {
	mo.Batch = append(mo.Batch, MockPoint{Name: name, Field: field, Value: value, Timestamp: timestamp, Tags: tags})
	return nil
}

// ResetBatch empties the batch
func (mo *MockOutput) ResetBatch() {
	mo.Batch = make([]MockPoint, 0)
}
