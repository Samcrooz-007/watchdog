//go:build integration

package test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"notashelf.dev/watchdog/internal/aggregate"
	"notashelf.dev/watchdog/internal/api"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/normalize"
)

func TestEndToEnd_BasicFlow(t *testing.T) {
	// Load config
	cfg, err := config.Load("../config.example.yaml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Override domain for test
	cfg.Site.Domains = []string{"test.example.com"}
	cfg.Site.SaltRotation = "daily"
	cfg.Limits.MaxPaths = 100
	cfg.Limits.MaxSources = 50
	cfg.Limits.MaxCustomEvents = 10

	// Initialize components
	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, cfg)

	// Register metrics
	promRegistry := prometheus.NewRegistry()
	metricsAgg.MustRegister(promRegistry)

	// Create handlers
	ingestionHandler := api.NewIngestionHandler(cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)
	metricsHandler := promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{})

	// Test pageview ingestion
	t.Run("PageviewIngestion", func(t *testing.T) {
		events := []string{
			`{"d":"test.example.com","p":"/","r":"","w":1920}`,
			`{"d":"test.example.com","p":"/about","r":"https://google.com","w":768}`,
			`{"d":"test.example.com","p":"/blog/post-1","r":"https://twitter.com","w":1024}`,
		}

		for _, event := range events {
			req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(event))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			ingestionHandler.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
			}
		}

		// Verify paths were registered
		if !pathRegistry.Contains("/") {
			t.Error("expected / to be registered")
		}
		if !pathRegistry.Contains("/about") {
			t.Error("expected /about to be registered")
		}
	})

	// Test custom events
	t.Run("CustomEventIngestion", func(t *testing.T) {
		events := []string{
			`{"d":"test.example.com","p":"/signup","e":"signup","w":1920}`,
			`{"d":"test.example.com","p":"/checkout","e":"purchase","w":1920}`,
		}

		for _, event := range events {
			req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(event))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			ingestionHandler.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Errorf("expected status 204, got %d", w.Code)
			}
		}
	})

	// Test metrics export
	t.Run("MetricsExport", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		metricsHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()

		// Check for expected metrics
		if !strings.Contains(body, "web_pageviews_total") {
			t.Error("expected web_pageviews_total metric")
		}
		if !strings.Contains(body, "web_custom_events_total") {
			t.Error("expected web_custom_events_total metric")
		}
		if !strings.Contains(body, "web_path_overflow_total") {
			t.Error("expected web_path_overflow_total metric")
		}
	})

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metricsAgg.Shutdown(ctx)
}

func TestEndToEnd_CardinalityLimits(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"test.example.com"},
			Collect: config.CollectConfig{
				Pageviews: true,
				Referrer:  "domain",
			},
			Path: config.PathConfig{
				StripQuery: true,
			},
		},
		Limits: config.LimitsConfig{
			MaxPaths:        5,
			MaxSources:      3,
			MaxCustomEvents: 2,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, &cfg)

	promRegistry := prometheus.NewRegistry()
	metricsAgg.MustRegister(promRegistry)

	handler := api.NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	// Send more paths than limit
	t.Run("PathOverflow", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			event := `{"d":"test.example.com","p":"/path-` + string(rune('0'+i)) + `","r":"","w":1920}`
			req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(event))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}

		// Verify overflow counter increased
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()
		promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{}).ServeHTTP(w, req)

		body := w.Body.String()
		if !strings.Contains(body, "web_path_overflow_total") {
			t.Error("expected overflow metric")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metricsAgg.Shutdown(ctx)
}

func TestEndToEnd_GracefulShutdown(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains:      []string{"test.example.com"},
			SaltRotation: "daily",
		},
		Limits: config.LimitsConfig{
			MaxPaths:        100,
			MaxSources:      50,
			MaxCustomEvents: 10,
		},
		Server: config.ServerConfig{
			StatePath: "/tmp/watchdog-test.state",
		},
	}

	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, &cfg)

	// Send some events to populate HLL
	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	handler := api.NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	for i := 0; i < 10; i++ {
		event := `{"d":"test.example.com","p":"/","r":"","w":1920}`
		req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(event))
		req.RemoteAddr = "192.168.1." + string(rune('0'+i)) + ":1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// Shutdown and save state
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := metricsAgg.Shutdown(ctx); err != nil {
		t.Errorf("shutdown failed: %v", err)
	}

	// Verify state file was created
	if _, err := os.Stat("/tmp/watchdog-test.state"); os.IsNotExist(err) {
		t.Error("HLL state file was not created")
	}

	// Cleanup
	os.Remove("/tmp/watchdog-test.state")
}

func TestEndToEnd_InvalidRequests(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"test.example.com"},
		},
		Limits: config.LimitsConfig{
			MaxPaths:        100,
			MaxSources:      50,
			MaxCustomEvents: 10,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, &cfg)

	handler := api.NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{
			name:       "GET request",
			method:     "GET",
			body:       "",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "Invalid JSON",
			method:     "POST",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Wrong domain",
			method:     "POST",
			body:       `{"d":"wrong.com","p":"/","r":"","w":1920}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing path",
			method:     "POST",
			body:       `{"d":"test.example.com","r":"","w":1920}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid width",
			method:     "POST",
			body:       `{"d":"test.example.com","p":"/","r":"","w":99999}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/event", bytes.NewBufferString(tt.body))
			if tt.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metricsAgg.Shutdown(ctx)
}

func TestEndToEnd_RateLimiting(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"test.example.com"},
		},
		Limits: config.LimitsConfig{
			MaxPaths:           100,
			MaxSources:         50,
			MaxCustomEvents:    10,
			MaxEventsPerMinute: 5, // very low limit for testing
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, &cfg)

	handler := api.NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	// Send requests until rate limited
	rateLimited := false
	for i := 0; i < 10; i++ {
		event := `{"d":"test.example.com","p":"/","r":"","w":1920}`
		req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(event))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	if !rateLimited {
		t.Error("expected rate limiting to trigger")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metricsAgg.Shutdown(ctx)
}

func TestEndToEnd_CORS(t *testing.T) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"test.example.com"},
		},
		Limits: config.LimitsConfig{
			MaxPaths:        100,
			MaxSources:      50,
			MaxCustomEvents: 10,
		},
		Security: config.SecurityConfig{
			CORS: config.CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"https://example.com"},
			},
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, &cfg)

	handler := api.NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	// Test OPTIONS preflight
	t.Run("Preflight", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/api/event", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", w.Code)
		}

		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			t.Error("expected CORS headers")
		}
	})

	// Test actual request with CORS
	t.Run("CORSRequest", func(t *testing.T) {
		event := `{"d":"test.example.com","p":"/","r":"","w":1920}`
		req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(event))
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			t.Error("expected CORS headers on POST")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metricsAgg.Shutdown(ctx)
}

func BenchmarkIngestionThroughput(b *testing.B) {
	cfg := config.Config{
		Site: config.SiteConfig{
			Domains: []string{"test.example.com"},
			Path: config.PathConfig{
				StripQuery: true,
			},
		},
		Limits: config.LimitsConfig{
			MaxPaths:        10000,
			MaxSources:      1000,
			MaxCustomEvents: 100,
		},
	}

	pathNorm := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, &cfg)

	handler := api.NewIngestionHandler(&cfg, pathNorm, pathRegistry, refRegistry, metricsAgg)

	event := `{"d":"test.example.com","p":"/","r":"https://google.com","w":1920}`

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/api/event", bytes.NewBufferString(event))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metricsAgg.Shutdown(ctx)
}
