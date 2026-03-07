package aggregate

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"notashelf.dev/watchdog/internal/config"
)

func TestMetricsAggregator_RecordPageview(t *testing.T) {
	registry := NewPathRegistry(100)
	cfg := config.Config{
		Site: config.SiteConfig{
			Collect: config.CollectConfig{
				Pageviews: true,
				Country:   true,
				Device:    true,
				Referrer:  "domain",
			},
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Record pageview with all dimensions
	agg.RecordPageview("/home", "US", "desktop", "google.com", "")

	// Verify metric was recorded
	expected := `
# HELP web_pageviews_total Total number of pageviews
# TYPE web_pageviews_total counter
web_pageviews_total{country="US",device="desktop",path="/home",referrer="google.com"} 1
`
	if err := testutil.CollectAndCompare(agg.pageviews, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metric value: %v", err)
	}
}

func TestMetricsAggregator_RecordPageview_MinimalDimensions(t *testing.T) {
	registry := NewPathRegistry(100)
	cfg := config.Config{
		Site: config.SiteConfig{
			Collect: config.CollectConfig{
				Pageviews: true,
				Country:   false,
				Device:    false,
				Referrer:  "off",
			},
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Record pageview with only path
	agg.RecordPageview("/home", "", "", "", "")

	// Verify metric was recorded
	expected := `
# HELP web_pageviews_total Total number of pageviews
# TYPE web_pageviews_total counter
web_pageviews_total{path="/home"} 1
`
	if err := testutil.CollectAndCompare(agg.pageviews, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metric value: %v", err)
	}
}

func TestMetricsAggregator_PathOverflow(t *testing.T) {
	// Create registry with limit of 2
	registry := NewPathRegistry(2)
	cfg := config.Config{
		Site: config.SiteConfig{
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Add two paths successfully
	registry.Add("/path1")
	registry.Add("/path2")

	// Try to add third path - should be rejected
	if registry.Add("/path3") {
		t.Error("expected path to be rejected")
	}

	// Record overflow
	agg.RecordPathOverflow()

	// Verify overflow metric
	expected := `
# HELP web_path_overflow_total Paths rejected due to cardinality limit
# TYPE web_path_overflow_total counter
web_path_overflow_total 1
`
	if err := testutil.CollectAndCompare(agg.pathOverflow, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metric value: %v", err)
	}
}

func TestMetricsAggregator_RecordCustomEvent(t *testing.T) {
	registry := NewPathRegistry(100)
	cfg := config.Config{
		Site: config.SiteConfig{
			Collect: config.CollectConfig{
				Pageviews: true,
			},
			CustomEvents: []string{"signup", "purchase"},
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Record custom event
	agg.RecordCustomEvent("signup")

	// Verify metric was recorded
	expected := `
# HELP web_custom_events_total Total number of custom events
# TYPE web_custom_events_total counter
web_custom_events_total{event="signup"} 1
`
	if err := testutil.CollectAndCompare(agg.customEvents, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metric value: %v", err)
	}
}

func TestMetricsAggregator_RecordCustomEvent_MultipleEvents(t *testing.T) {
	registry := NewPathRegistry(100)
	cfg := config.Config{
		Site: config.SiteConfig{
			Collect: config.CollectConfig{
				Pageviews: true,
			},
			CustomEvents: []string{"signup", "purchase"},
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Record multiple events
	agg.RecordCustomEvent("signup")
	agg.RecordCustomEvent("signup")
	agg.RecordCustomEvent("purchase")

	// Verify metrics
	if err := testutil.CollectAndCompare(agg.customEvents, strings.NewReader(`
# HELP web_custom_events_total Total number of custom events
# TYPE web_custom_events_total counter
web_custom_events_total{event="purchase"} 1
web_custom_events_total{event="signup"} 2
`)); err != nil {
		t.Errorf("unexpected metric value: %v", err)
	}
}

func TestMetricsAggregator_MustRegister(t *testing.T) {
	registry := NewPathRegistry(100)
	cfg := config.Config{
		Site: config.SiteConfig{
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
	}

	promRegistry := prometheus.NewRegistry()
	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Register metrics
	agg.MustRegister(promRegistry)

	// Record some metrics to ensure they show up
	agg.RecordPageview("/test", "", "", "", "")
	agg.RecordPathOverflow()

	// Verify metrics can be gathered
	metrics, err := promRegistry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Should have at least pageviews and path overflow metrics
	if len(metrics) < 2 {
		t.Errorf("expected at least 2 metric families, got %d", len(metrics))
	}
}

func TestMetricsAggregator_Shutdown_ConfigurableStatePath(t *testing.T) {
	registry := NewPathRegistry(100)

	// Create temp directory for test
	tmpDir := t.TempDir()
	statePath := tmpDir + "/custom-hll.state"

	cfg := config.Config{
		Site: config.SiteConfig{
			SaltRotation: "daily",
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
		Server: config.ServerConfig{
			StatePath: statePath,
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Add some unique visitors so there's state to save
	agg.AddUnique("192.168.1.1", "Mozilla/5.0")

	// Shutdown should save to configured path
	ctx := context.Background()
	if err := agg.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify file was created at configured path
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Errorf("state file was not created at configured path: %s", statePath)
	}
}

func TestMetricsAggregator_Shutdown_DefaultStatePath(t *testing.T) {
	registry := NewPathRegistry(100)

	cfg := config.Config{
		Site: config.SiteConfig{
			Domains:      []string{"example.com"}, // Required for validation
			SaltRotation: "daily",
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
		Limits: config.LimitsConfig{
			MaxPaths:   1000,
			MaxSources: 500,
		},
		Server: config.ServerConfig{
			// StatePath not set - validation should set default
			StatePath: "",
		},
	}

	// Validate to apply defaults
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}

	// Verify default was applied
	expectedDefault := "/var/lib/watchdog/hll.state"
	if cfg.Server.StatePath != expectedDefault {
		t.Errorf(
			"expected default StatePath %q, got %q",
			expectedDefault,
			cfg.Server.StatePath,
		)
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Add some unique visitors
	agg.AddUnique("192.168.1.1", "Mozilla/5.0")

	// Shutdown should save to default path
	ctx := context.Background()
	if err := agg.Shutdown(ctx); err != nil {
		// Might fail due to permissions on /var/lib, which is OK for this test
		// We're just verifying the code path works
		t.Logf("Shutdown returned error (might be expected): %v", err)
	}
}

func TestMetricsAggregator_Shutdown_RespectsContext(t *testing.T) {
	registry := NewPathRegistry(100)
	tmpDir := t.TempDir()

	cfg := config.Config{
		Site: config.SiteConfig{
			SaltRotation: "daily",
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
		Server: config.ServerConfig{
			StatePath: tmpDir + "/hll.state",
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	// Shutdown should respect context timeout
	err := agg.Shutdown(ctx)
	if err == nil {
		t.Error("expected context deadline exceeded error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf(
			"expected context.DeadlineExceeded, got %v",
			err,
		)
	}
}

func TestMetricsAggregator_Shutdown_WaitsForGoroutine(t *testing.T) {
	registry := NewPathRegistry(100)
	tmpDir := t.TempDir()

	cfg := config.Config{
		Site: config.SiteConfig{
			SaltRotation: "daily",
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
		Server: config.ServerConfig{
			StatePath: tmpDir + "/hll.state",
		},
	}

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// Give the goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Track if goroutine is still running
	done := make(chan struct{})
	go func() {
		ctx := context.Background()
		_ = agg.Shutdown(ctx)
		close(done)
	}()

	// Shutdown should complete quickly (goroutine should stop)
	select {
	case <-done:
		// Success - shutdown completed
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown did not complete within timeout - goroutine not stopping")
	}
}

func TestMetricsAggregator_LoadState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := tmpDir + "/hll.state"

	cfg := config.Config{
		Site: config.SiteConfig{
			SaltRotation: "daily",
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
		Server: config.ServerConfig{
			StatePath: statePath,
		},
	}

	// Create first aggregator and add some visitors
	registry1 := NewPathRegistry(100)
	agg1 := NewMetricsAggregator(registry1, NewCustomEventRegistry(100), &cfg)
	agg1.AddUnique("192.168.1.1", "Mozilla/5.0")
	agg1.AddUnique("192.168.1.2", "Mozilla/5.0")
	agg1.AddUnique("192.168.1.3", "Mozilla/5.0")

	// Get estimate before shutdown
	estimate1 := agg1.estimator.Estimate()
	if estimate1 == 0 {
		t.Fatal("expected non-zero estimate before shutdown")
	}

	// Shutdown to save state
	ctx := context.Background()
	if err := agg1.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify state file was created
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	// Create second aggregator and load state
	registry2 := NewPathRegistry(100)
	agg2 := NewMetricsAggregator(registry2, NewCustomEventRegistry(100), &cfg)

	// Load should restore the state
	if err := agg2.LoadState(); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	// Estimate should match (approximately - HLL is probabilistic)
	estimate2 := agg2.estimator.Estimate()
	if estimate2 != estimate1 {
		t.Errorf("expected estimate %d after load, got %d", estimate1, estimate2)
	}
}

func TestMetricsAggregator_LoadState_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := tmpDir + "/nonexistent.state"

	cfg := config.Config{
		Site: config.SiteConfig{
			SaltRotation: "daily",
			Collect: config.CollectConfig{
				Pageviews: true,
			},
		},
		Server: config.ServerConfig{
			StatePath: statePath,
		},
	}

	registry := NewPathRegistry(100)
	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), &cfg)

	// LoadState should not error if file doesn't exist (first run)
	if err := agg.LoadState(); err != nil {
		t.Errorf("LoadState should not error on missing file, got: %v", err)
	}
}
