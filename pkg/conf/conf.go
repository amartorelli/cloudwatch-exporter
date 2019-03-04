package conf

// CloudwatchExporter is the global configuration for the program
type CloudwatchExporter struct {
	Rate          int                  `yaml:"rate"`
	Region        string               `yaml:"region"`
	Namespaces    map[string]Namespace `yaml:"namespaces"`
	Output        Output               `yaml:"output"`
	FetchInterval int                  `yaml:"fetch_interval"`
}

// Namespace represents a Namespace configuration
type Namespace struct {
	Metrics []Metric `yaml:"metrics"`
}

// Metric represents the configuration for a metric that CloudwatchExporter will fetch
type Metric struct {
	Name       string            `yaml:"name"`
	Dimensions map[string]string `yaml:"dimensions"`
}

// Output represents the InfluxDB metrics for the CloudwatchExporter
type Output struct {
	Proto    string `yaml:"proto"`
	Addr     string `yaml:"address"`
	DB       string `yaml:"database"`
	User     string `yaml:"user"`
	Pass     string `yaml:"password"`
	Interval int    `yaml:"interval"`
}
