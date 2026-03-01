package api

import (
	"log"
	"net/http"

	"notashelf.dev/watchdog/internal/aggregate"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/normalize"
)

// Handles incoming analytics events
type IngestionHandler struct {
	cfg          config.Config
	pathNorm     *normalize.PathNormalizer
	pathRegistry *aggregate.PathRegistry
	refRegistry  *normalize.ReferrerRegistry
	metricsAgg   *aggregate.MetricsAggregator
}

// Creates a new ingestion handler
func NewIngestionHandler(
	cfg config.Config,
	pathNorm *normalize.PathNormalizer,
	pathRegistry *aggregate.PathRegistry,
	refRegistry *normalize.ReferrerRegistry,
	metricsAgg *aggregate.MetricsAggregator,
) *IngestionHandler {
	return &IngestionHandler{
		cfg:          cfg,
		pathNorm:     pathNorm,
		pathRegistry: pathRegistry,
		refRegistry:  refRegistry,
		metricsAgg:   metricsAgg,
	}
}

func (h *IngestionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse event from request body
	event, err := ParseEvent(r.Body)
	if err != nil {
		log.Printf("Failed to parse event: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Validate event
	if err := event.Validate(h.cfg.Site.Domain); err != nil {
		log.Printf("Event validation failed: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Normalize path and check if path can be added to the registry.
	normalizedPath := h.pathNorm.Normalize(event.Path)
	if !h.pathRegistry.Add(normalizedPath) {
		// Path was rejected due to cardinality limit
		h.metricsAgg.RecordPathOverflow()
		log.Printf("Path overflow: rejected %s", normalizedPath)

		// Still return success to client
		w.WriteHeader(http.StatusNoContent)

		return
	}

	// Process based on event type
	if event.Event != "" {
		// Custom event
		h.metricsAgg.RecordCustomEvent(event.Event)
	} else {
		// Pageview; process with full normalization pipeline
		var country, device, referrer string

		// Device classification
		if h.cfg.Site.Collect.Device {
			device = classifyDevice(event.Width)
		}

		// Referrer classification
		if h.cfg.Site.Collect.Referrer == "domain" {
			refDomain := normalize.ExtractReferrerDomain(event.Referrer, h.cfg.Site.Domain)
			if refDomain != "" {
				referrer = h.refRegistry.Add(refDomain)
			}
		}

		// FIXME: Country would be extracted from IP here. For now, we skip country extraction
		// because I have neither the time nor the patience to look into it. Return later.

		// Record pageview
		h.metricsAgg.RecordPageview(normalizedPath, country, device, referrer)
	}

	// Return success
	w.WriteHeader(http.StatusNoContent)
}

// Classifies screen width into device categories
func classifyDevice(width int) string {
	// FIXME: probably not the best logic for this...
	if width == 0 {
		return "unknown"
	}
	if width < 768 {
		return "mobile"
	}
	if width < 1024 {
		return "tablet"
	}
	return "desktop"
}
