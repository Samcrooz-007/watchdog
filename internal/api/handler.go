package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"notashelf.dev/watchdog/internal/aggregate"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/normalize"
	"notashelf.dev/watchdog/internal/ratelimit"
)

// Handles incoming analytics events
type IngestionHandler struct {
	cfg             *config.Config
	domainMap       map[string]bool // O(1) domain validation
	corsOriginMap   map[string]bool // O(1) CORS origin validation
	pathNorm        *normalize.PathNormalizer
	pathRegistry    *aggregate.PathRegistry
	refRegistry     *normalize.ReferrerRegistry
	metricsAgg      *aggregate.MetricsAggregator
	rateLimiter     *ratelimit.TokenBucket
	rng             *mrand.Rand
	trustedNetworks []*net.IPNet // pre-parsed CIDR networks
}

// Creates a new ingestion handler
func NewIngestionHandler(
	cfg *config.Config,
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

	// Build domain map for O(1) validation
	domainMap := make(map[string]bool, len(cfg.Site.Domains))
	for _, domain := range cfg.Site.Domains {
		domainMap[domain] = true
	}

	// Build CORS origin map for O(1) lookup
	corsOriginMap := make(map[string]bool, len(cfg.Security.CORS.AllowedOrigins))
	for _, origin := range cfg.Security.CORS.AllowedOrigins {
		corsOriginMap[origin] = true
	}

	// Pre-parse trusted proxy CIDRs to avoid re-parsing on each request
	trustedNetworks := make([]*net.IPNet, 0, len(cfg.Security.TrustedProxies))
	for _, cidr := range cfg.Security.TrustedProxies {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			trustedNetworks = append(trustedNetworks, network)
		} else if ip := net.ParseIP(cidr); ip != nil {
			// Single IP - create a /32 or /128 network
			var mask net.IPMask
			if ip.To4() != nil {
				mask = net.CIDRMask(32, 32)
			} else {
				mask = net.CIDRMask(128, 128)
			}
			trustedNetworks = append(trustedNetworks, &net.IPNet{IP: ip, Mask: mask})
		}
	}

	return &IngestionHandler{
		cfg:             cfg,
		domainMap:       domainMap,
		corsOriginMap:   corsOriginMap,
		pathNorm:        pathNorm,
		pathRegistry:    pathRegistry,
		refRegistry:     refRegistry,
		metricsAgg:      metricsAgg,
		rateLimiter:     limiter,
		rng:             mrand.New(mrand.NewSource(time.Now().UnixNano())),
		trustedNetworks: trustedNetworks,
	}
}

func (h *IngestionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Generate or extract request ID for tracing
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateRequestID()
	}
	w.Header().Set("X-Request-ID", requestID)

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

	// Validate event via map lookup
	if err := event.Validate(h.domainMap); err != nil {
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
			device = h.classifyDevice(event.Width, userAgent)
		}

		// Referrer classification
		if h.cfg.Site.Collect.Referrer == "domain" {
			refDomain := normalize.ExtractReferrerDomain(event.Referrer, event.Domain)
			if refDomain != "" {
				accepted := h.refRegistry.Add(refDomain)
				if accepted == "other" {
					h.metricsAgg.RecordReferrerOverflow()
				}
				referrer = accepted
			}
		}

		// Domain tracking (if enabled for multi-site analytics)
		var domain string
		if h.cfg.Site.Collect.Domain {
			domain = event.Domain
		}

		// FIXME: Country would be extracted from IP here. For now, we skip country extraction
		// because I have neither the time nor the patience to look into it. Return later.

		// Record pageview
		h.metricsAgg.RecordPageview(normalizedPath, country, device, referrer, domain)
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
	// This uses map so that it's O(1)
	allowed := h.corsOriginMap["*"] || h.corsOriginMap[origin]

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
	if len(h.trustedNetworks) > 0 {
		trustProxy = h.ipInNetworks(remoteIP, h.trustedNetworks)
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
			if testIP := net.ParseIP(ip); testIP != nil {
				// Only accept this IP if it's NOT from a trusted proxy
				if !h.ipInNetworks(ip, h.trustedNetworks) {
					return ip
				}
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return remoteIP
}

// Checks if an IP address is within any of the trusted networks
func (h *IngestionHandler) ipInNetworks(ip string, networks []*net.IPNet) bool {
	testIP := net.ParseIP(ip)
	if testIP == nil {
		return false
	}

	for _, network := range networks {
		if network.Contains(testIP) {
			return true
		}
	}

	return false
}

// Classifies device using both screen width and User-Agent parsing
// Uses UA hints for better detection, falls back to width breakpoints
func (h *IngestionHandler) classifyDevice(width int, userAgent string) string {
	// First try User-Agent based detection for better accuracy
	ua := strings.ToLower(userAgent)

	// Tablet detection via UA (must come before mobile: Android tablets lack "mobile" keyword)
	if strings.Contains(ua, "tablet") ||
		strings.Contains(ua, "ipad") ||
		(strings.Contains(ua, "android") && !strings.Contains(ua, "mobile")) {
		return "tablet"
	}

	// Mobile detection via UA
	if strings.Contains(ua, "mobile") ||
		strings.Contains(ua, "iphone") ||
		strings.Contains(ua, "ipod") ||
		strings.Contains(ua, "windows phone") ||
		strings.Contains(ua, "blackberry") {
		return "mobile"
	}

	// If UA doesn't provide clear signal, use width breakpoints
	if width > 0 {
		if width < h.cfg.Limits.DeviceBreakpoints.Mobile {
			return "mobile"
		}
		if width < h.cfg.Limits.DeviceBreakpoints.Tablet {
			return "tablet"
		}
		return "desktop"
	}

	// Default to desktop if UA suggests desktop browser
	if userAgent != "" {
		return "desktop"
	}

	return "unknown"
}

// generateRequestID creates a unique request ID for tracing
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
