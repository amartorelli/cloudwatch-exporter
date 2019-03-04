package output

import "testing"

func TestStandardiseMetricName(t *testing.T) {
	tt := []struct {
		in       string
		expected string
	}{
		{"AWS/SQS_NumberOfMessages", "AWS_SQS_NumberOfMessages"},
		{"AWS/Kinesis_GetRecords.Records", "AWS_Kinesis_GetRecords_Records"},
		{"AWS/Kinesis_GetRecords.Records.tEst", "AWS_Kinesis_GetRecords_Records_tEst"},
		{"AWS.Kinesis_GetRecords.Records.tEst", "AWS_Kinesis_GetRecords_Records_tEst"},
	}

	for _, tc := range tt {
		m := standardiseMetricName(tc.in)
		if m != tc.expected {
			t.Errorf("standardised version of %s should be %s, received %s", tc.in, tc.expected, m)
		}
	}
}
