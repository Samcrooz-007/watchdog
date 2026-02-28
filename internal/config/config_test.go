package config

import (
	"testing"
)

func TestLoadConfig_ValidFile(t *testing.T) {
	cfg, err := Load("../../testdata/config.valid.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Site.Domain != "example.com" {
		t.Errorf("expected domain 'example.com', got '%s'", cfg.Site.Domain)
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
			Domain:       "example.com",
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
