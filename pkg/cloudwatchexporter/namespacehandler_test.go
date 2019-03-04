package cloudwatchexporter

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/amartorelli/cloudwatch-exporter/pkg/conf"
	"github.com/amartorelli/cloudwatch-exporter/pkg/output"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

var (
	metricName       = "CPUUtilization"
	secondMetricName = "DiskReadBytes"
	namespace        = "AWS/EC2"
	dimName          = "AutoScalingGroupName"
	dimValue         = "spoofed_asg"
	secondDimName    = "InstanceType"
	secondDimValue   = "i-1234567890"
	badRegex         = "ECS-CLUSTER-[0-9"
	exampleConf      = conf.Namespace{
		Metrics: []conf.Metric{
			{Name: "CPUUtilization", Dimensions: map[string]string{"AutoScalingGroupName": "spoofed.*", "InstanceType": "i-1234567890"}},
			{Name: "DiskReadBytes", Dimensions: map[string]string{"AutoScalingGroupName": "spoofed.*", "InstanceType": "i-1234567890"}},
			{Name: "ApproximateNumberOfMessages", Dimensions: map[string]string{"QueueName": "TEST-QUEUE"}},
			{Name: "AgeOfOldestMessage"},
		},
	}
)

func TestIsMetricMonitored(t *testing.T) {
	var tt = []struct {
		metric string
		result bool
	}{
		{"CPUUtilization", true},
		{"CPUUTILIZATION", false},
		{"ApproximateNumberOfMessages", true},
		{"MemoryUtilization", false},
	}

	rl := time.NewTicker(500 * time.Millisecond)
	nsh := newNamespaceHandler("AWS/EC2", exampleConf, nil, rl, 0, nil)

	for _, tc := range tt {
		if nsh.isMetricMonitored(tc.metric) != tc.result {
			t.Errorf("%s presence should be %v", tc.metric, tc.result)
		}
	}
}

func TestGetNamespaceMetrics(t *testing.T) {
	var tt = []struct {
		metric     string
		dimensions []dimension
	}{
		{"CPUUtilization", []dimension{
			{name: "AutoScalingGroupName", value: "FOO-ASG"},
			{name: "ImageId", value: "i-1234567890"},
		}},
		{"ApproximateNumberOfMessages", []dimension{
			{name: "QueueName", value: "FOO-ASG"},
		}},
		{"AgeOfOldestMessage", []dimension{}},
	}

	metricConfig := []conf.Metric{
		{Name: "CPUUtilization", Dimensions: map[string]string{"AutoScalingGroupName": "FOO-ASG", "ImageId": "i-1234567890"}},
		{Name: "ApproximateNumberOfMessages", Dimensions: map[string]string{"QueueName": "FOO-ASG"}},
		{Name: "AgeOfOldestMessage"},
	}

	actualConfig := getNamespaceMetrics(metricConfig)
	for _, tc := range tt {
		var found bool
		for _, m := range actualConfig {
			if tc.metric == m.name {
				found = true
				sort.Slice(tc.dimensions, func(i, j int) bool { return tc.dimensions[i].name > tc.dimensions[j].name })
				sort.Slice(m.dimensions, func(i, j int) bool { return m.dimensions[i].name > m.dimensions[j].name })

				if !reflect.DeepEqual(tc.dimensions, m.dimensions) {
					t.Errorf("expecting metric %s dimenstions %s to be equal to %s", tc.metric, tc.dimensions, m.dimensions)
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("expecting to find metric %s", tc.metric)
		}
	}
}

func TestMetricMatchesDefinition(t *testing.T) {
	var tt = []struct {
		cwMetric  *cloudwatch.Metric
		metricDef metric
		matched   bool
		isError   bool
		msg       string
	}{
		{
			&cloudwatch.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
				},
			},
			metric{
				name:       "CPUReservation",
				dimensions: []dimension{dimension{name: "AutoScalingGroupName", value: "spoofed_asg"}},
			},
			false,
			false,
			"metric should not match because metric name is different",
		},
		{
			&cloudwatch.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
				},
			},
			metric{
				name:       "CPUUtilization",
				dimensions: []dimension{dimension{name: "AutoScalingGroupName", value: "spoofed_asg"}},
			},
			true,
			false,
			"metric should match",
		},
		{
			&cloudwatch.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: []*cloudwatch.Dimension{},
			},
			metric{
				name:       "CPUUtilization",
				dimensions: []dimension{dimension{name: "AutoScalingGroupName", value: "spoofed_asg"}},
			},
			false,
			false,
			"metric should not match because the cloudwatch metric does not contain a matching dimension",
		},
		{
			&cloudwatch.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
					&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
				},
			},
			metric{
				name:       "CPUUtilization",
				dimensions: []dimension{dimension{name: "AutoScalingGroupName", value: "spoofed_asg"}},
			},
			true,
			false,
			"metric should match because the dimensions in the configuration match the dimensions coming from the api",
		},
		{
			&cloudwatch.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
					&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
				},
			},
			metric{
				name:       "CPUUtilization",
				dimensions: []dimension{},
			},
			true,
			false,
			"metric should match even if there are no dimensions specified in the config",
		},
		{
			&cloudwatch.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
					&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
				},
			},
			metric{
				name:       "CPUUtilization",
				dimensions: []dimension{dimension{name: "AutoScalingGroupName", value: badRegex}},
			},
			false,
			true,
			"this should raise an error because parsing the regular expression throws an error",
		},
	}

	for _, tc := range tt {
		matched, err := metricMatchesDefinition(tc.cwMetric, tc.metricDef)

		if err == nil && tc.isError == true {
			t.Errorf("expecting an error but got nil")
		}

		if err != nil && tc.isError == false {
			t.Errorf("unexpected error received: %v", err)
		}

		if matched != tc.matched {
			t.Error(tc.msg)
		}
	}
}

func TestAddMetricsToFetch(t *testing.T) {
	var tt = []struct {
		cwMetrics              []*cloudwatch.Metric
		expectedMetrics        []string
		expectedMetricsToFetch map[string][][]*cloudwatch.Dimension
		isError                bool
		msg                    string
	}{
		{
			[]*cloudwatch.Metric{
				&cloudwatch.Metric{
					Namespace:  &namespace,
					MetricName: &metricName,
					Dimensions: []*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
						&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
					},
				},
			},
			[]string{"CPUUtilization"},
			map[string][][]*cloudwatch.Dimension{
				"CPUUtilization": [][]*cloudwatch.Dimension{
					[]*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
					},
				},
			},
			false,
			"with the provided conf it should return the CPUUtilization metric with autoscaling group dimension",
		},
		{
			[]*cloudwatch.Metric{
				&cloudwatch.Metric{
					Namespace:  &namespace,
					MetricName: &metricName,
					Dimensions: []*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
					},
				},
			},
			[]string{},
			map[string][][]*cloudwatch.Dimension{},
			false,
			"with the provided conf it should return no metric because the dimensions aren't found in the metric",
		},
		{
			[]*cloudwatch.Metric{
				&cloudwatch.Metric{
					Namespace:  &namespace,
					MetricName: &metricName,
					Dimensions: []*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
						&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
					},
				},
				&cloudwatch.Metric{
					Namespace:  &namespace,
					MetricName: &secondDimName,
					Dimensions: []*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
						&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
					},
				},
			},
			[]string{"CPUUtilization"},
			map[string][][]*cloudwatch.Dimension{
				"CPUUtilization": [][]*cloudwatch.Dimension{
					[]*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
					},
				},
			},
			false,
			"with the provided conf it should return the CPUUtilization metric with autoscaling group dimension",
		},
		{
			[]*cloudwatch.Metric{
				&cloudwatch.Metric{
					Namespace:  &namespace,
					MetricName: &metricName,
					Dimensions: []*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
						&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
					},
				},
				&cloudwatch.Metric{
					Namespace:  &namespace,
					MetricName: &secondMetricName,
					Dimensions: []*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
						&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
					},
				},
			},
			[]string{"CPUUtilization", "DiskReadBytes"},
			map[string][][]*cloudwatch.Dimension{
				"CPUUtilization": [][]*cloudwatch.Dimension{
					[]*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
					},
				},
				"DiskReadBytes": [][]*cloudwatch.Dimension{
					[]*cloudwatch.Dimension{
						&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
					},
				},
			},
			false,
			"with the provided conf it should return the CPUUtilization metric with autoscaling group dimension",
		},
	}

	rl := time.NewTicker(500 * time.Millisecond)

	for _, tc := range tt {
		nsh := newNamespaceHandler("AWS/EC2", exampleConf, nil, rl, 0, nil)
		// check error
		err := nsh.addMetricsToFetch(tc.cwMetrics)
		if err == nil && tc.isError == true {
			t.Errorf("expecting an error but got nil")
		}

		if err != nil && tc.isError == false {
			t.Errorf("unexpected error received: %v", err)
		}

		// check keys are what we expect
		keys := make([]string, 0)
		for key := range nsh.metricsToFetch {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		sort.Strings(tc.expectedMetrics)
		if !reflect.DeepEqual(keys, tc.expectedMetrics) {
			t.Errorf("expected metrics %v, received %v", tc.expectedMetrics, keys)
		}

		// for each key we populated in the metricsToFetch we check the dimensions
		// are what we expect
		var allMatched bool
		for _, k := range keys {
			allMatched = true
			for _, mtfDims := range nsh.metricsToFetch[k] {
				matched := checkAllDimensions(mtfDims, tc.expectedMetricsToFetch[k])
				if !matched {
					allMatched = false
					break
				}
			}
			if !allMatched {
				allMatched = false
				break
			}
		}
		if !allMatched && len(keys) > 0 {
			t.Error(tc.msg)
		}
	}
}

func checkAllDimensions(dims []*cloudwatch.Dimension, dimsList [][]*cloudwatch.Dimension) bool {
	found := false
	for _, dims := range dimsList {
		f := true
		for _, srcD := range dims {
			everySrcDimInCmp := true
			for _, cmpD := range dims {
				if *srcD.Name == *cmpD.Name && *srcD.Value == *cmpD.Value {
					everySrcDimInCmp = everySrcDimInCmp && true
				}
			}
			f = f && everySrcDimInCmp
		}
		found = found || f
	}
	return found
}

func TestDimensionsToMap(t *testing.T) {
	var tt = []struct {
		dims    []*cloudwatch.Dimension
		dimsMap map[string]string
	}{
		{
			make([]*cloudwatch.Dimension, 0),
			make(map[string]string, 0),
		},
		{
			[]*cloudwatch.Dimension{
				&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
			},
			map[string]string{
				dimName: dimValue,
			},
		},
		{
			[]*cloudwatch.Dimension{
				&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
				&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
			},
			map[string]string{
				dimName: dimValue,
			},
		},
		{
			[]*cloudwatch.Dimension{
				&cloudwatch.Dimension{Name: &dimName, Value: &dimValue},
				&cloudwatch.Dimension{Name: &secondDimName, Value: &secondDimValue},
			},
			map[string]string{
				dimName:       dimValue,
				secondDimName: secondDimValue,
			},
		},
	}

	for _, tc := range tt {
		m := dimensionsToMap(tc.dims)
		if !reflect.DeepEqual(m, tc.dimsMap) {
			t.Errorf("expecting %v to be %v", m, tc.dimsMap)
		}
	}
}

func TestFetchMetric(t *testing.T) {
	mockClient := NewMockCloudwatchClient()
	mockOutput := output.NewMockOutput()
	now := time.Now()
	nsh := &namespaceHandler{
		namespace:           namespace,
		metrics:             []metric{},
		cwClient:            mockClient,
		metricsToFetch:      make(map[string][][]*cloudwatch.Dimension, 0),
		metricsToFetchMutex: &sync.RWMutex{},
		fetchInterval:       0,
		limiter:             nil,
		threadTrackers:      nil,
		output:              mockOutput,
	}

	var tt = []struct {
		dps            []*cloudwatch.Datapoint
		metric         string
		dims           []*cloudwatch.Dimension
		expectedPoints []output.MockPoint
	}{
		{
			[]*cloudwatch.Datapoint{},
			"CPUUtilization",
			[]*cloudwatch.Dimension{},
			[]output.MockPoint{},
		},
		{
			[]*cloudwatch.Datapoint{
				&cloudwatch.Datapoint{Maximum: aws.Float64(4.3), Timestamp: &now},
			},
			"CPUUtilization",
			[]*cloudwatch.Dimension{},
			[]output.MockPoint{
				{Name: namespace, Field: "CPUUtilization", Value: float64(4.3), Timestamp: &now, Tags: make(map[string]string, 0)},
			},
		},
		{
			[]*cloudwatch.Datapoint{
				&cloudwatch.Datapoint{Maximum: aws.Float64(4.3), Timestamp: &now},
				&cloudwatch.Datapoint{Maximum: aws.Float64(6.5), Timestamp: &now},
			},
			"CPUUtilization",
			[]*cloudwatch.Dimension{},
			[]output.MockPoint{
				{Name: namespace, Field: "CPUUtilization", Value: float64(4.3), Timestamp: &now, Tags: make(map[string]string, 0)},
				{Name: namespace, Field: "CPUUtilization", Value: float64(6.5), Timestamp: &now, Tags: make(map[string]string, 0)},
			},
		},
	}

	for _, tc := range tt {
		mockOutput.ResetBatch()
		mockClient.SetDatapoints(tc.dps)
		nsh.fetchDatapoints(tc.metric, tc.dims)
		if !allPointsPresent(mockOutput.Batch, tc.expectedPoints) {
			t.Errorf("discrepancy between the datapoints:\n%+v\n%+v\n", mockOutput.Batch, tc.expectedPoints)
		}
	}
}

func allPointsPresent(batch, expected []output.MockPoint) bool {
	sort.Slice(batch, func(i, j int) bool {
		return fmt.Sprintf("%s%f", batch[i].Name, batch[i].Value) > fmt.Sprintf("%s%f", batch[j].Name, batch[j].Value)
	})
	sort.Slice(expected, func(i, j int) bool {
		return fmt.Sprintf("%s%f", expected[i].Name, expected[i].Value) > fmt.Sprintf("%s%f", expected[j].Name, expected[j].Value)
	})
	return reflect.DeepEqual(batch, expected)
}
