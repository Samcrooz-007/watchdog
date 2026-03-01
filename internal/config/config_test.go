package config

import (
	"testing"
)

func TestLoadConfig_ValidFile(t *testing.T) {
	cfg, err := Load("../../testdata/config.valid.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(cfg.Site.Domains) == 0 || cfg.Site.Domains[0] != "example.com" {
		t.Errorf("expected domains to contain 'example.com', got %v", cfg.Site.Domains)
	}

	if cfg.Site.SaltRotation != "daily" {
		t.Errorf("expected salt_rotation 'daily', got '%s'", cfg.Site.SaltRotation)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := Load("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestValidate_MaxPathsRequired(t *testing.T) {
	cfg := &Config{
		Site: SiteConfig{
			Domains:      []string{"example.com"},
			SaltRotation: "daily",
		},
		Limits: LimitsConfig{
			MaxPaths: 0, // invalid
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for max_paths=0, got nil")
	}
}
