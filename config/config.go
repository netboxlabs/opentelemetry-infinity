package config

import "time"

// Status represents the status of the service
type Status struct {
	StartTime time.Time     `json:"start_time"`
	UpTime    time.Duration `json:"up_time"`
	Version   string        `json:"version"`
}

// Policy represents the configuration of the opentelemetry collector
type Policy struct {
	FeatureGates []string               `yaml:"feature_gates"`
	Set          map[string]string      `yaml:"set"`
	Config       map[string]interface{} `yaml:"config"`
}

// Config represents the configuration of the opentelemetry collector
type Config struct {
	Debug         bool   `mapstructure:"otlpinf_debug"`
	SelfTelemetry bool   `mapstructure:"otlpinf_self_telemetry"`
	ServerHost    string `mapstructure:"otlpinf_server_host"`
	ServerPort    uint64 `mapstructure:"otlpinf_server_port"`
}
