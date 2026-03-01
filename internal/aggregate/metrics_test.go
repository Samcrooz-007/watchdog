package aggregate

import (
	"strings"
	"testing"

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

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), cfg)

	// Record pageview with all dimensions
	agg.RecordPageview("/home", "US", "desktop", "google.com")

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

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), cfg)

	// Record pageview with only path
	agg.RecordPageview("/home", "", "", "")

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

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), cfg)

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

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), cfg)

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

	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), cfg)

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
	agg := NewMetricsAggregator(registry, NewCustomEventRegistry(100), cfg)

	// Register metrics
	agg.MustRegister(promRegistry)

	// Record some metrics to ensure they show up
	agg.RecordPageview("/test", "", "", "")
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
