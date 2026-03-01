package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Configuration structure
type Config struct {
	Site     SiteConfig     `yaml:"site"`
	Limits   LimitsConfig   `yaml:"limits"`
	Server   ServerConfig   `yaml:"server"`
	Security SecurityConfig `yaml:"security"`
}

// Site-specific settings
type SiteConfig struct {
	Domains      []string      `yaml:"domains"` // list of allowed domains
	SaltRotation string        `yaml:"salt_rotation"`
	Collect      CollectConfig `yaml:"collect"`
	CustomEvents []string      `yaml:"custom_events"`
	Path         PathConfig    `yaml:"path"`
	Sampling     float64       `yaml:"sampling"` // 0.0-1.0, 1.0 = track all traffic
}

// Which dimensions to collect
type CollectConfig struct {
	Pageviews bool   `yaml:"pageviews"`
	Country   bool   `yaml:"country"`
	Device    bool   `yaml:"device"`
	Referrer  string `yaml:"referrer"`
	Domain    bool   `yaml:"domain"` // track domain as metric dimension (for multi-site)
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
	MaxPaths           int          `yaml:"max_paths"`
	MaxEventsPerMinute int          `yaml:"max_events_per_minute"`
	MaxSources         int          `yaml:"max_sources"`
	MaxCustomEvents    int          `yaml:"max_custom_events"`
	DeviceBreakpoints  DeviceBreaks `yaml:"device_breakpoints"`
}

// Device classification breakpoints
type DeviceBreaks struct {
	Mobile int `yaml:"mobile"` // < mobile = mobile
	Tablet int `yaml:"tablet"` // < tablet = tablet, else desktop
}

// Security settings
type SecurityConfig struct {
	TrustedProxies []string   `yaml:"trusted_proxies"` // IPs/CIDRs to trust X-Forwarded-For from
	CORS           CORSConfig `yaml:"cors"`
	MetricsAuth    AuthConfig `yaml:"metrics_auth"`
}

// CORS configuration
type CORSConfig struct {
	Enabled        bool     `yaml:"enabled"`
	AllowedOrigins []string `yaml:"allowed_origins"` // ["*"] or specific domains
}

// Authentication for metrics endpoint
type AuthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
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
func (c *Config) Validate() error {
	// Site validation
	if len(c.Site.Domains) == 0 {
		return fmt.Errorf("site.domains is required")
	}

	// Validate salt_rotation
	if c.Site.SaltRotation != "" && c.Site.SaltRotation != "daily" && c.Site.SaltRotation != "hourly" {
		return fmt.Errorf("site.salt_rotation must be 'daily' or 'hourly'")
	}

	// Validate referrer setting
	if c.Site.Collect.Referrer != "" && c.Site.Collect.Referrer != "off" &&
		c.Site.Collect.Referrer != "domain" && c.Site.Collect.Referrer != "url" {
		return fmt.Errorf("site.collect.referrer must be 'off', 'domain', or 'url'")
	}

	// Validate sampling rate
	if c.Site.Sampling < 0.0 || c.Site.Sampling > 1.0 {
		return fmt.Errorf("site.sampling must be between 0.0 and 1.0")
	}
	if c.Site.Sampling == 0.0 {
		c.Site.Sampling = 1.0 // default to 100%
	}

	// Limits validation
	if c.Limits.MaxPaths <= 0 {
		return fmt.Errorf("limits.max_paths must be greater than 0")
	}

	if c.Limits.MaxSources <= 0 {
		return fmt.Errorf("limits.max_sources must be greater than 0")
	}

	if c.Limits.MaxEventsPerMinute < 0 {
		return fmt.Errorf("limits.max_events_per_minute cannot be negative")
	}

	if c.Limits.MaxCustomEvents <= 0 {
		c.Limits.MaxCustomEvents = 100 // Default
	}

	if c.Site.Path.MaxSegments < 0 {
		return fmt.Errorf("site.path.max_segments cannot be negative")
	}

	// Set device breakpoint defaults
	if c.Limits.DeviceBreakpoints.Mobile == 0 {
		c.Limits.DeviceBreakpoints.Mobile = 768
	}
	if c.Limits.DeviceBreakpoints.Tablet == 0 {
		c.Limits.DeviceBreakpoints.Tablet = 1024
	}

	// Server defaults
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = ":8080"
	}
	if c.Server.MetricsPath == "" {
		c.Server.MetricsPath = "/metrics"
	}
	if c.Server.IngestionPath == "" {
		c.Server.IngestionPath = "/api/event"
	}

	if c.Security.MetricsAuth.Enabled {
		if c.Security.MetricsAuth.Username == "" || c.Security.MetricsAuth.Password == "" {
			return fmt.Errorf("security.metrics_auth: username and password required when enabled")
		}
	}

	if c.Security.CORS.Enabled && len(c.Security.CORS.AllowedOrigins) == 0 {
		return fmt.Errorf("security.cors: allowed_origins required when enabled")
	}

	return nil
}
