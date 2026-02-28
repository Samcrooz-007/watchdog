package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Configuration structure
type Config struct {
	Site   SiteConfig   `yaml:"site"`
	Limits LimitsConfig `yaml:"limits"`
	Server ServerConfig `yaml:"server"`
}

// Site-specific settings
type SiteConfig struct {
	Domain       string        `yaml:"domain"`
	SaltRotation string        `yaml:"salt_rotation"`
	Collect      CollectConfig `yaml:"collect"`
	CustomEvents []string      `yaml:"custom_events"`
	Path         PathConfig    `yaml:"path"`
}

// Which dimensions to collect
type CollectConfig struct {
	Pageviews bool   `yaml:"pageviews"`
	Country   bool   `yaml:"country"`
	Device    bool   `yaml:"device"`
	Referrer  string `yaml:"referrer"`
}

// Path normalization options
type PathConfig struct {
	StripQuery              bool `yaml:"strip_query"`
	StripFragment           bool `yaml:"strip_fragment"`
	CollapseNumericSegments bool `yaml:"collapse_numeric_segments"`
	MaxSegments             int  `yaml:"max_segments"`
	NormalizeTrailingSlash  bool `yaml:"normalize_trailing_slash"`
}

// Cardinality limits
type LimitsConfig struct {
	MaxPaths           int `yaml:"max_paths"`
	MaxEventsPerMinute int `yaml:"max_events_per_minute"`
}

// Server endpoints
type ServerConfig struct {
	ListenAddr    string `yaml:"listen_addr"`
	MetricsPath   string `yaml:"metrics_path"`
	IngestionPath string `yaml:"ingestion_path"`
}

// YAML configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Check required fields and sets defaults
// FIXME: in the future we need to validate in the config parser
func (c *Config) Validate() error {
	// Validate site domain is required
	if c.Site.Domain == "" {
		return fmt.Errorf("site.domain is required")
	}

	// Validate salt_rotation if provided
	if c.Site.SaltRotation != "" && c.Site.SaltRotation != "daily" && c.Site.SaltRotation != "hourly" {
		return fmt.Errorf("site.salt_rotation must be 'daily' or 'hourly'")
	}

	// Validate max_paths is positive
	if c.Limits.MaxPaths <= 0 {
		return fmt.Errorf("limits.max_paths must be greater than 0")
	}

	// Set server defaults if not provided
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = ":8080"
	}
	if c.Server.MetricsPath == "" {
		c.Server.MetricsPath = "/metrics"
	}
	if c.Server.IngestionPath == "" {
		c.Server.IngestionPath = "/api/event"
	}

	return nil
}
