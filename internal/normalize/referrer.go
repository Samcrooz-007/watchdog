package normalize

import (
	"net"
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

	// Check if hostname is an IP address
	if ip := net.ParseIP(hostname); ip != nil {
		// Private IPv4 ranges (RFC1918)
		if ip.IsPrivate() {
			return true
		}
		// Additional localhost checks for IP formats
		if ip.IsLoopback() {
			return true
		}
		// Link-local addresses
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
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
		// If public suffix lookup fails (malformed/unknown TLD), return "other"
		// to prevent unbounded cardinality from malicious referrers
		return "other"
	}

	return eTLDPlus1
}
