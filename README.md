# Cloudwatch exporter

This program is used to export Cloudwatch metrics to InfluxDB.
It can observe multiple namespaces each of which will be handled by a separate thread.
For each namespace multiple metrics to watch can be defined and the dimensions used to filter them executes
regular expressions. This means it's possible to define something like `MYAUTOSCALING-.*` for `AutoScalingGroupName`
and that would match every AutoScalingGroupName that starts with `MYAUTOSCALING-.*`.

**WARNING: as flexible as this program could be, it will come with costs. It's been optimised so that it only
fetches the list of metrics to export every now and caches it, but each metric must then be fetched separately due
to how the AWS API works. This means that the cost for running this service is proportional to how many metrics
are matched by the dimension filters.**

For more informatino about AWS costs: https://aws.amazon.com/cloudwatch/pricing/

## Configuration
The following is a configuration example:
```
---
---
rate: 200
region: "eu-west-1"
fetch_interval: 180
namespaces: 
  "AWS/SQS": 
    metrics: 
    - name: "ApproximateNumberOfMessagesVisible"
      dimensions: 
        QueueName: "^MYQUEUENAME$"
  "AWS/EC2": 
    metrics: 
    - name: "CPUUtilization"
      dimensions: 
        AutoScalingGroupName: "CLUSTERNAME-.*"
    - name: "CPUUtilization"
      dimensions: 
        AutoScalingGroupName: "SECONDCLUSTERNAME-.*"
    - name: "DiskReadBytes"
      dimensions: 
        AutoScalingGroupName: "SECONDCLUSTERNAME-.*"
output:
  proto: "http"
  address: "http://localhost:8086"
  database: "telegraf"
  user: "admin"
  password: "admin"
  interval: 60
```

- rate: defines the ratelimiter to make sure the application doesn't get rate limited by the AWS API
- region: the AWS region to fetch the metrics from
- fetch_interval: the interval in seconds of when to fetch the metrics from the AWS API
- namespaces: the namespaces to watch. Each namespace will have multiple metrics/dimensions
- output: the InfluxDB info for the output

## Testing locally
In the `extra` directory there's a docker-compose file which sets up a minimal environment with InfluxDB and Grafana.
Once that's been set up the cloudwatch-exporter can be run with `make build-run`.
Then grafana will be available at `localhost:3000` with `admin/admin` as username and password.
The data source needs to be configured to influxdb, source `http://extra_influxdb_1:8086`.