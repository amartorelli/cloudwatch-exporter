package cloudwatchexporter

import "github.com/aws/aws-sdk-go/service/cloudwatch"

// MockCloudwatchClient is used to mock a Cloudwatch Client in the unit tests
type MockCloudwatchClient struct {
	datapoints []*cloudwatch.Datapoint
}

// NewMockCloudwatchClient returns an instance of MockCloudwatchClient
func NewMockCloudwatchClient() *MockCloudwatchClient {
	return &MockCloudwatchClient{}
}

// SetDatapoints resets the datapoints of the mock client to the ones passed to the function
func (mcc *MockCloudwatchClient) SetDatapoints(dps []*cloudwatch.Datapoint) {
	mcc.datapoints = dps
}

// ListMetrics is only implemented to match the interface
func (mcc *MockCloudwatchClient) ListMetrics(*cloudwatch.ListMetricsInput) (*cloudwatch.ListMetricsOutput, error) {
	return nil, nil
}

// GetMetricStatistics returns fake AWS metrics
func (mcc *MockCloudwatchClient) GetMetricStatistics(*cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error) {
	return &cloudwatch.GetMetricStatisticsOutput{Datapoints: mcc.datapoints}, nil
}
