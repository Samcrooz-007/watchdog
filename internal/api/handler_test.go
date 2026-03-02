package api

import (
	"bytes"
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
	metricsAgg := aggregate.NewMetricsAggregator(
		pathRegistry,
		aggregate.NewCustomEventRegistry(100),
		&cfg,
	)

	handler := NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

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
	metricsAgg := aggregate.NewMetricsAggregator(
		pathRegistry,
		aggregate.NewCustomEventRegistry(100),
		&cfg,
	)

	handler := NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

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
	metricsAgg := aggregate.NewMetricsAggregator(
		pathRegistry,
		aggregate.NewCustomEventRegistry(100),
		&cfg,
	)

	handler := NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

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
	metricsAgg := aggregate.NewMetricsAggregator(
		pathRegistry,
		aggregate.NewCustomEventRegistry(100),
		&cfg,
	)

	handler := NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

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
	metricsAgg := aggregate.NewMetricsAggregator(
		pathRegistry,
		aggregate.NewCustomEventRegistry(100),
		&cfg,
	)

	handler := NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	body := `{invalid json}`
	req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func newTestHandler(cfg *config.Config) *IngestionHandler {
	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(
		pathRegistry,
		aggregate.NewCustomEventRegistry(100),
		cfg,
	)
	return NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)
}

func TestClassifyDevice_UA(t *testing.T) {
	cfg := &config.Config{
		Limits: config.LimitsConfig{
			DeviceBreakpoints: config.DeviceBreaks{
				Mobile: 768,
				Tablet: 1024,
			},
		},
	}
	h := newTestHandler(cfg)

	tests := []struct {
		name      string
		width     int
		userAgent string
		want      string
	}{
		// UA takes priority
		{
			name:      "iphone via UA",
			width:     390,
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15",
			want:      "mobile",
		},
		{
			name:      "android phone via UA",
			width:     0,
			userAgent: "Mozilla/5.0 (Linux; Android 13; Pixel 7) Mobile Safari/537.36",
			want:      "mobile",
		},
		{
			name:      "windows phone via UA",
			width:     0,
			userAgent: "Mozilla/5.0 (compatible; MSIE 10.0; Windows Phone 8.0)",
			want:      "mobile",
		},
		{
			name:      "ipad via UA",
			width:     1024,
			userAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15",
			want:      "tablet",
		},
		{
			name:      "android tablet via UA (no mobile keyword)",
			width:     0,
			userAgent: "Mozilla/5.0 (Linux; Android 13; SM-T870) AppleWebKit/537.36",
			want:      "tablet",
		},
		// Falls back to width when UA is desktop
		{
			name:      "desktop UA wide screen",
			width:     1920,
			userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0",
			want:      "desktop",
		},
		{
			name:      "desktop UA narrow width",
			width:     500,
			userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0",
			want:      "mobile",
		},
		// Width-only fallback
		{
			name:      "no UA mobile width",
			width:     375,
			userAgent: "",
			want:      "mobile",
		},
		{
			name:      "no UA tablet width",
			width:     800,
			userAgent: "",
			want:      "tablet",
		},
		{
			name:      "no UA desktop width",
			width:     1440,
			userAgent: "",
			want:      "desktop",
		},
		// Unknown
		{
			name:      "no UA no width",
			width:     0,
			userAgent: "",
			want:      "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.classifyDevice(tt.width, tt.userAgent)
			if got != tt.want {
				t.Errorf("classifyDevice(%d, %q) = %q, want %q",
					tt.width, tt.userAgent, got, tt.want)
			}
		})
	}
}
