package cloudwatchexporter

import "github.com/aws/aws-sdk-go/service/cloudwatch"

// CloudwatchClient is the CW client interface we can use to mock
type CloudwatchClient interface {
	ListMetrics(*cloudwatch.ListMetricsInput) (*cloudwatch.ListMetricsOutput, error)
	GetMetricStatistics(*cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error)
}
