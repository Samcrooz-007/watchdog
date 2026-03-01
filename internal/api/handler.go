package api

import (
	"math/rand"
	"net"
	"net/http"
	"strings"

	"notashelf.dev/watchdog/internal/aggregate"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/normalize"
	"notashelf.dev/watchdog/internal/ratelimit"
)

// Handles incoming analytics events
type IngestionHandler struct {
	cfg          config.Config
	pathNorm     *normalize.PathNormalizer
	pathRegistry *aggregate.PathRegistry
	refRegistry  *normalize.ReferrerRegistry
	metricsAgg   *aggregate.MetricsAggregator
	rateLimiter  *ratelimit.TokenBucket
	rng          *rand.Rand
}

// Creates a new ingestion handler
func NewIngestionHandler(
	cfg config.Config,
	pathNorm *normalize.PathNormalizer,
	pathRegistry *aggregate.PathRegistry,
	refRegistry *normalize.ReferrerRegistry,
	metricsAgg *aggregate.MetricsAggregator,
) *IngestionHandler {
	var limiter *ratelimit.TokenBucket
	if cfg.Limits.MaxEventsPerMinute > 0 {
		limiter = ratelimit.NewTokenBucket(
			cfg.Limits.MaxEventsPerMinute,
			cfg.Limits.MaxEventsPerMinute,
			60_000_000_000, // 1 minute in nanoseconds
		)
	}

	return &IngestionHandler{
		cfg:          cfg,
		pathNorm:     pathNorm,
		pathRegistry: pathRegistry,
		refRegistry:  refRegistry,
		metricsAgg:   metricsAgg,
		rateLimiter:  limiter,
		rng:          rand.New(rand.NewSource(42)),
	}
}

func (h *IngestionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Apply CORS headers to actual request
	h.handleCORS(w, r)

	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check rate limit
	if h.rateLimiter != nil && !h.rateLimiter.Allow() {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Apply sampling (0.0 or 1.0 = no sampling, < 1.0 = sample)
	if h.cfg.Site.Sampling > 0.0 && h.cfg.Site.Sampling < 1.0 {
		if h.rng.Float64() > h.cfg.Site.Sampling {
			// Sampled out, return success but don't track
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	// Check context cancellation
	if r.Context().Err() != nil {
		return
	}

	// Parse event from request body
	event, err := ParseEvent(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Validate event
	if err := event.Validate(h.cfg.Site.Domain); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Normalize path and check if path can be added to the registry.
	normalizedPath := h.pathNorm.Normalize(event.Path)
	if !h.pathRegistry.Add(normalizedPath) {
		// Path was rejected due to cardinality limit
		h.metricsAgg.RecordPathOverflow()

		// Still return success to client
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Extract visitor identity for unique tracking
	ip := h.extractIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Track unique visitor if salt rotation is enabled
	h.metricsAgg.AddUnique(ip, userAgent)

	// Process based on event type
	if event.Event != "" {
		// Custom event
		h.metricsAgg.RecordCustomEvent(event.Event)
	} else {
		// Pageview; process with full normalization pipeline
		var country, device, referrer string

		// Device classification
		if h.cfg.Site.Collect.Device {
			device = h.classifyDevice(event.Width)
		}

		// Referrer classification
		if h.cfg.Site.Collect.Referrer == "domain" {
			refDomain := normalize.ExtractReferrerDomain(event.Referrer, h.cfg.Site.Domain)
			if refDomain != "" {
				accepted := h.refRegistry.Add(refDomain)
				if accepted == "other" {
					h.metricsAgg.RecordReferrerOverflow()
				}
				referrer = accepted
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

// Adds CORS headers if enabled in config
func (h *IngestionHandler) handleCORS(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Security.CORS.Enabled {
		return
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	// Check if origin is allowed
	allowed := false
	for _, allowedOrigin := range h.cfg.Security.CORS.AllowedOrigins {
		if allowedOrigin == "*" || allowedOrigin == origin {
			allowed = true
			break
		}
	}

	if allowed {
		if origin == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")
	}
}

// Extracts the client IP from the requests. Only trusts proxy headers if source
// IP is in trusted_proxies list
func (h *IngestionHandler) extractIP(r *http.Request) string {
	// Get the direct connection IP
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have port (shouldn't happen, but handle it anyway)
		remoteIP = r.RemoteAddr
	}

	// Check if we should trust proxy headers
	trustProxy := false
	if len(h.cfg.Security.TrustedProxies) > 0 {
		for _, trustedCIDR := range h.cfg.Security.TrustedProxies {
			if h.ipInCIDR(remoteIP, trustedCIDR) {
				trustProxy = true
				break
			}
		}
	}

	// If not trusting proxy, return direct IP
	if !trustProxy {
		return remoteIP
	}

	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the rightmost IP that's not from a trusted proxy
		ips := strings.Split(xff, ",")
		for i := len(ips) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(ips[i])
			if !h.ipInCIDR(ip, "0.0.0.0/0") {
				continue
			}
			return ip
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return remoteIP
}

// Checks if an IP address is within a CIDR range
func (h *IngestionHandler) ipInCIDR(ip, cidr string) bool {
	// Parse the IP address
	testIP := net.ParseIP(ip)
	if testIP == nil {
		return false
	}

	// Parse the CIDR
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		// If it's not a CIDR, try as a single IP
		cidrIP := net.ParseIP(cidr)
		if cidrIP == nil {
			return false
		}
		return testIP.Equal(cidrIP)
	}

	return network.Contains(testIP)
}

// Classifies screen width into device categories using configured breakpoints
// FIXME: we need a more robust mechanism for classifying devices. Breakpoints
// are the only ones I can think of *right now* but I'm positive there are better
// mechanisns. We'll get to this later.
func (h *IngestionHandler) classifyDevice(width int) string {
	if width == 0 {
		return "unknown"
	}
	if width < h.cfg.Limits.DeviceBreakpoints.Mobile {
		return "mobile"
	}
	if width < h.cfg.Limits.DeviceBreakpoints.Tablet {
		return "tablet"
	}
	return "desktop"
}
