package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"notashelf.dev/watchdog/internal/aggregate"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/normalize"
)

func TestIngestionHandler_Pageview(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"example.com"},
			Collect: config.CollectConfig{
				Pageviews: true,
				Country:   true,
				Device:    true,
				Referrer:  "domain",
			},
			Path: config.PathConfig{
				StripQuery:              true,
				StripFragment:           true,
				CollapseNumericSegments: true,
				NormalizeTrailingSlash:  true,
			},
		},
		Limits: config.LimitsConfig{
			MaxPaths:   100,
			MaxSources: 50,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, aggregate.NewCustomEventRegistry(100), cfg)

	handler := NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	body := `{"d":"example.com","p":"/home?query=1","r":"https://google.com","w":1920}`
	req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}

	// Verify path was normalized and registered
	if !pathRegistry.Contains("/home") {
		t.Error("expected path to be registered")
	}
}

func TestIngestionHandler_CustomEvent(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"example.com"},
			Collect: config.CollectConfig{
				Pageviews: true,
			},
			CustomEvents: []string{"signup", "purchase"},
			Path: config.PathConfig{
				StripQuery: true,
			},
		},
		Limits: config.LimitsConfig{
			MaxPaths:   100,
			MaxSources: 50,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, aggregate.NewCustomEventRegistry(100), cfg)

	handler := NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	body := `{"d":"example.com","p":"/signup","e":"signup"}`
	req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}
}

func TestIngestionHandler_WrongDomain(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"example.com"},
			Collect: config.CollectConfig{
				Pageviews: true,
			},
			Path: config.PathConfig{},
		},
		Limits: config.LimitsConfig{
			MaxPaths:   100,
			MaxSources: 50,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, aggregate.NewCustomEventRegistry(100), cfg)

	handler := NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	body := `{"d":"wrong.com","p":"/home"}`
	req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestIngestionHandler_MethodNotAllowed(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"example.com"},
		},
		Limits: config.LimitsConfig{
			MaxPaths:   100,
			MaxSources: 50,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, aggregate.NewCustomEventRegistry(100), cfg)

	handler := NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	req := httptest.NewRequest("GET", "/api/event", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestIngestionHandler_InvalidJSON(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"example.com"},
		},
		Limits: config.LimitsConfig{
			MaxPaths:   100,
			MaxSources: 50,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, aggregate.NewCustomEventRegistry(100), cfg)

	handler := NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	body := `{invalid json}`
	req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestIngestionHandler_DeviceClassification(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"example.com"},
			Collect: config.CollectConfig{
				Pageviews: true,
				Device:    true,
			},
			Path: config.PathConfig{},
		},
		Limits: config.LimitsConfig{
			MaxPaths:   100,
			MaxSources: 50,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, aggregate.NewCustomEventRegistry(100), cfg)

	handler := NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	tests := []struct {
		name  string
		width int
	}{
		{"mobile", 375},
		{"tablet", 768},
		{"desktop", 1920},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"d":"example.com","p":"/test","w":%d}`, tt.width)
			req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(body))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
			}
		})
	}
}
