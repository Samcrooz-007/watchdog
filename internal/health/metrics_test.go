package health

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewCollector_RegistersMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector("v0.1.0", "abc1234", "2026-03-02")

	if err := c.Register(reg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	// Should have at least build_info and uptime
	names := make(map[string]bool)
	for _, m := range metrics {
		names[m.GetName()] = true
	}

	if !names["watchdog_build_info"] {
		t.Error("expected watchdog_build_info metric")
	}
	if !names["watchdog_start_time_seconds"] {
		t.Error("expected watchdog_start_time_seconds metric")
	}
}

func TestNewCollector_BuildInfoLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector("v1.2.3", "deadbeef", "2026-03-02")

	if err := c.Register(reg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	for _, m := range metrics {
		if m.GetName() != "watchdog_build_info" {
			continue
		}

		labels := make(map[string]string)
		for _, l := range m.GetMetric()[0].GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}

		if labels["version"] != "v1.2.3" {
			t.Errorf("expected version label %q, got %q", "v1.2.3", labels["version"])
		}
		if labels["commit"] != "deadbeef" {
			t.Errorf("expected commit label %q, got %q", "deadbeef", labels["commit"])
		}
		if labels["build_date"] != "2026-03-02" {
			t.Errorf(
				"expected build_date label %q, got %q",
				"2026-03-02",
				labels["build_date"],
			)
		}
		return
	}

	t.Error("watchdog_build_info metric not found in gathered metrics")
}

func TestNewCollector_StartTimeIsPositive(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector("v0.1.0", "abc1234", "2026-03-02")

	if err := c.Register(reg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	for _, m := range metrics {
		if m.GetName() != "watchdog_start_time_seconds" {
			continue
		}
		val := m.GetMetric()[0].GetGauge().GetValue()
		if val <= 0 {
			t.Errorf("expected positive start time, got %v", val)
		}
		return
	}

	t.Error("watchdog_start_time_seconds metric not found")
}
