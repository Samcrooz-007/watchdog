package aggregate

import (
	"github.com/prometheus/client_golang/prometheus"
	"notashelf.dev/watchdog/internal/config"
)

// Records analytics events as Prometheus metrics
type MetricsAggregator struct {
	registry     *PathRegistry
	cfg          config.Config
	pageviews    *prometheus.CounterVec
	customEvents *prometheus.CounterVec
	pathOverflow prometheus.Counter
	dailyUniques prometheus.Gauge
	estimator    *UniquesEstimator
}

// Creates a new metrics aggregator with dynamic labels based on config
func NewMetricsAggregator(registry *PathRegistry, cfg config.Config) *MetricsAggregator {
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

	dailyUniques := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "web_daily_unique_visitors",
			Help: "Estimated unique visitors today (HyperLogLog)",
		},
	)

	return &MetricsAggregator{
		registry:     registry,
		cfg:          cfg,
		pageviews:    pageviews,
		customEvents: customEvents,
		pathOverflow: pathOverflow,
		dailyUniques: dailyUniques,
		estimator:    NewUniquesEstimator(),
	}
}

// Records a pageview with the configured dimensions
func (m *MetricsAggregator) RecordPageview(path, country, device, referrer string) {
	// Build label values in the same order as label names
	labels := prometheus.Labels{"path": path}

	if m.cfg.Site.Collect.Country {
		labels["country"] = country
	}

	if m.cfg.Site.Collect.Device {
		labels["device"] = device
	}

	if m.cfg.Site.Collect.Referrer != "off" {
		labels["referrer"] = referrer
	}

	m.pageviews.With(labels).Inc()
}

// Records a custom event
func (m *MetricsAggregator) RecordCustomEvent(eventName string) {
	m.customEvents.With(prometheus.Labels{"event": eventName}).Inc()
}

// Records a path that was rejected due to cardinality limits
func (m *MetricsAggregator) RecordPathOverflow() {
	m.pathOverflow.Inc()
}

// Adds a visitor to the unique visitor estimator. Only tracks if salt_rotation is configured
func (m *MetricsAggregator) AddUnique(ip, userAgent string) {
	if m.cfg.Site.SaltRotation == "" {
		return // only track if salt rotation is enabled
	}

	m.estimator.Add(ip, userAgent)
	m.dailyUniques.Set(float64(m.estimator.Estimate()))
}

// Registers all metrics with the provided Prometheus registry
func (m *MetricsAggregator) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(m.pageviews)
	reg.MustRegister(m.customEvents)
	reg.MustRegister(m.pathOverflow)
	reg.MustRegister(m.dailyUniques)
}
