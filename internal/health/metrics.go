package health

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Holds health and runtime metrics for the watchdog process
type Collector struct {
	buildInfo prometheus.Gauge
	startTime prometheus.Gauge
}

// Creates a health metrics collector with build metadata
func NewCollector(version, commit, buildDate string) *Collector {
	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "watchdog_build_info",
		Help: "Build metadata for the running watchdog instance",
		ConstLabels: prometheus.Labels{
			"version":    version,
			"commit":     commit,
			"build_date": buildDate,
		},
	})
	buildInfo.Set(1)

	startTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "watchdog_start_time_seconds",
		Help: "Unix timestamp of when the watchdog process started",
	})
	startTime.Set(float64(time.Now().Unix()))

	return &Collector{
		buildInfo: buildInfo,
		startTime: startTime,
	}
}

// Registers all health metrics plus Go runtime collectors
func (c *Collector) Register(reg prometheus.Registerer) error {
	if err := reg.Register(c.buildInfo); err != nil {
		return err
	}
	if err := reg.Register(c.startTime); err != nil {
		return err
	}
	if err := reg.Register(collectors.NewGoCollector()); err != nil {
		return err
	}
	if err := reg.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return err
	}
	return nil
}
