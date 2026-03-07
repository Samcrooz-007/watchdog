package aggregate

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/limits"
)

var prometheusLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9_/:.-]*$`)

// Records analytics events as Prometheus metrics
type MetricsAggregator struct {
	pathRegistry  *PathRegistry
	eventRegistry *CustomEventRegistry
	cfg           *config.Config
	pageviews     *prometheus.CounterVec
	customEvents  *prometheus.CounterVec
	pathOverflow  prometheus.Counter
	refOverflow   prometheus.Counter
	eventOverflow prometheus.Counter
	dailyUniques  prometheus.Gauge
	estimator     *UniquesEstimator
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// Creates a new metrics aggregator with dynamic labels based on config
func NewMetricsAggregator(
	pathRegistry *PathRegistry,
	eventRegistry *CustomEventRegistry,
	cfg *config.Config,
) *MetricsAggregator {
	// Build label names based on what's enabled in config
	labels := []string{"path"} // path is always included

	if cfg.Site.Collect.Country {
		labels = append(labels, "country")
	}

	if cfg.Site.Collect.Device {
		labels = append(labels, "device")
	}

	if cfg.Site.Collect.Referrer != "off" {
		labels = append(labels, "referrer")
	}

	if cfg.Site.Collect.Domain {
		labels = append(labels, "domain")
	}

	pageviews := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "web_pageviews_total",
			Help: "Total number of pageviews",
		},
		labels,
	)

	customEvents := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "web_custom_events_total",
			Help: "Total number of custom events",
		},
		[]string{"event"},
	)

	pathOverflow := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "web_path_overflow_total",
			Help: "Paths rejected due to cardinality limit",
		},
	)

	refOverflow := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "web_referrer_overflow_total",
			Help: "Referrers rejected due to cardinality limit",
		},
	)

	eventOverflow := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "web_event_overflow_total",
			Help: "Custom events rejected due to cardinality limit",
		},
	)

	dailyUniques := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "web_daily_unique_visitors",
			Help: "Estimated unique visitors today (HyperLogLog)",
		},
	)

	m := &MetricsAggregator{
		pathRegistry:  pathRegistry,
		eventRegistry: eventRegistry,
		cfg:           cfg,
		pageviews:     pageviews,
		customEvents:  customEvents,
		pathOverflow:  pathOverflow,
		refOverflow:   refOverflow,
		eventOverflow: eventOverflow,
		dailyUniques:  dailyUniques,
		estimator:     NewUniquesEstimator(cfg.Site.SaltRotation),
		stopChan:      make(chan struct{}),
	}

	// Start background goroutine to update HLL gauge periodically
	if cfg.Site.SaltRotation != "" {
		m.wg.Add(1)
		go m.updateUniquesGauge()
	}

	return m
}

// Background goroutine to update the unique visitors gauge periodically
// instead of on every request. This should help with performance.
func (m *MetricsAggregator) updateUniquesGauge() {
	defer m.wg.Done()
	ticker := time.NewTicker(limits.UniquesUpdatePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			estimate := m.estimator.Estimate()
			m.dailyUniques.Set(float64(estimate))
		case <-m.stopChan:
			return
		}
	}
}

// Stop gracefully shuts down the background goroutine
func (m *MetricsAggregator) Stop() {
	close(m.stopChan)
}

// sanitizeLabel ensures label values are valid for Prometheus
func sanitizeLabel(label string) string {
	if !prometheusLabelPattern.MatchString(label) {
		return "other"
	}
	if len(label) > 100 {
		return "other"
	}
	return label
}

// Records a pageview with the configured dimensions
func (m *MetricsAggregator) RecordPageview(path, country, device, referrer, domain string) {
	// Build label values in the same order as label names
	labels := prometheus.Labels{"path": sanitizeLabel(path)}

	if m.cfg.Site.Collect.Country {
		labels["country"] = sanitizeLabel(country)
	}

	if m.cfg.Site.Collect.Device {
		labels["device"] = sanitizeLabel(device)
	}

	if m.cfg.Site.Collect.Referrer != "off" {
		labels["referrer"] = sanitizeLabel(referrer)
	}

	if m.cfg.Site.Collect.Domain {
		labels["domain"] = sanitizeLabel(domain)
	}

	m.pageviews.With(labels).Inc()
}

// Records a custom event with cardinality protection
func (m *MetricsAggregator) RecordCustomEvent(eventName string) {
	// Use registry to enforce cardinality limit
	sanitized := sanitizeLabel(eventName)
	accepted := m.eventRegistry.Add(sanitized)

	if accepted == "other" {
		m.eventOverflow.Inc()
	}

	m.customEvents.With(prometheus.Labels{"event": accepted}).Inc()
}

// Records a path that was rejected due to cardinality limits
func (m *MetricsAggregator) RecordPathOverflow() {
	m.pathOverflow.Inc()
}

// Records a referrer that was rejected due to cardinality limits
func (m *MetricsAggregator) RecordReferrerOverflow() {
	m.refOverflow.Inc()
}

// Adds a visitor to the unique visitor estimator.
// Only tracks if salt_rotation is configured
func (m *MetricsAggregator) AddUnique(ip, userAgent string) {
	if m.cfg.Site.SaltRotation == "" {
		return // only track if salt rotation is enabled
	}

	m.estimator.Add(ip, userAgent)
	// NOTE: Gauge is updated in background goroutine, not here
}

// Registers all metrics with the provided Prometheus registry
func (m *MetricsAggregator) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(m.pageviews)
	reg.MustRegister(m.customEvents)
	reg.MustRegister(m.pathOverflow)
	reg.MustRegister(m.refOverflow)
	reg.MustRegister(m.eventOverflow)
	reg.MustRegister(m.dailyUniques)
}

// LoadState restores HLL state from disk if it exists
func (m *MetricsAggregator) LoadState() error {
	if m.cfg.Site.SaltRotation == "" {
		return nil // State persistence not enabled
	}
	return m.estimator.Load(m.cfg.Server.StatePath)
}

// Shutdown performs graceful shutdown operations
func (m *MetricsAggregator) Shutdown(ctx context.Context) error {
	// Signal goroutine to stop
	m.Stop()

	// Wait for goroutine to finish, respecting context deadline
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Goroutine finished successfully
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}

	// Persist HLL state if configured
	if m.cfg.Site.SaltRotation != "" {
		return m.estimator.Save(m.cfg.Server.StatePath)
	}
	return nil
}
