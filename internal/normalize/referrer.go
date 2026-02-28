package normalize

import (
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Returns true for localhost, loopback IPs, and private IPs.
func isInternalHost(hostname string) bool {
	if hostname == "" {
		return false
	}

	// Localhost variants
	if hostname == "localhost" ||
		strings.HasPrefix(hostname, "localhost.") ||
		strings.HasPrefix(hostname, "127.") ||
		hostname == "::1" {
		return true
	}

	// Private IPv4 ranges (RFC1918)
	if strings.HasPrefix(hostname, "10.") ||
		strings.HasPrefix(hostname, "192.168.") ||
		strings.HasPrefix(hostname, "172.16.") ||
		strings.HasPrefix(hostname, "172.17.") ||
		strings.HasPrefix(hostname, "172.18.") ||
		strings.HasPrefix(hostname, "172.19.") ||
		strings.HasPrefix(hostname, "172.20.") ||
		strings.HasPrefix(hostname, "172.21.") ||
		strings.HasPrefix(hostname, "172.22.") ||
		strings.HasPrefix(hostname, "172.23.") ||
		strings.HasPrefix(hostname, "172.24.") ||
		strings.HasPrefix(hostname, "172.25.") ||
		strings.HasPrefix(hostname, "172.26.") ||
		strings.HasPrefix(hostname, "172.27.") ||
		strings.HasPrefix(hostname, "172.28.") ||
		strings.HasPrefix(hostname, "172.29.") ||
		strings.HasPrefix(hostname, "172.30.") ||
		strings.HasPrefix(hostname, "172.31.") {
		return true
	}

	// IPv6 loopback and local
	if strings.HasPrefix(hostname, "::1") ||
		strings.HasPrefix(hostname, "fe80::") ||
		strings.HasPrefix(hostname, "fc00::") ||
		strings.HasPrefix(hostname, "fd00::") {
		return true
	}

	return false
}

// Extracts the eTLD+1 domain from a referrer URL.
// Returns "direct" for empty or same-domain referrers.
// Returns empty string for invalid URLs.
func ExtractReferrerDomain(referrer, siteDomain string) string {
	if referrer == "" {
		return "direct"
	}

	u, err := url.Parse(referrer)
	if err != nil {
		return ""
	}

	hostname := strings.ToLower(u.Hostname())
	hostname = strings.TrimSuffix(hostname, ".") // remove trailing dot
	if hostname == "" {
		return ""
	}

	// Check for internal/localhost traffic
	if isInternalHost(hostname) {
		return "internal"
	}

	// Same domain check
	siteDomainLower := strings.ToLower(siteDomain)
	if hostname == siteDomainLower || strings.HasSuffix(hostname, "."+siteDomainLower) {
		return "direct"
	}

	// Extract eTLD+1 (effective top-level domain + 1 label); e.g.
	// - "www.google.co.uk" -> "google.co.uk"
	// - "news.ycombinator.com" -> "ycombinator.com"
	eTLDPlus1, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		// If public suffix lookup fails, use hostname as-is
		return hostname
	}

	return eTLDPlus1
}
