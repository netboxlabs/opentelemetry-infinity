package config

import "time"

// ContextKey represents the key for the context
type ContextKey string

// Status represents the status of the service
type Status struct {
	StartTime time.Time     `json:"start_time"`
	UpTime    time.Duration `json:"up_time"`
	Version   string        `json:"version"`
}

// Policy represents the configuration of the opentelemetry collector
type Policy struct {
	Receivers  map[string]interface{} `yaml:"receivers"`
	Processors map[string]interface{} `yaml:"processors,omitempty"`
	Exporters  map[string]interface{} `yaml:"exporters"`
	Extensions map[string]interface{} `yaml:"extensions,omitempty"`
	Service    map[string]interface{} `yaml:"service"`
}

// Config represents the configuration of the opentelemetry collector
type Config struct {
	Debug         bool     `mapstructure:"otlpinf_debug"`
	SelfTelemetry bool     `mapstructure:"otlpinf_self_telemetry"`
	ServerHost    string   `mapstructure:"otlpinf_server_host"`
	ServerPort    uint64   `mapstructure:"otlpinf_server_port"`
	FeatureGates  string   `mapstructure:"feature_gates"`
	Set           []string `mapstructure:"set"`
	LogTimestamps bool     `mapstructure:"otlpinf_log_timestamps"`
}
