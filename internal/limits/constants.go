package limits

import "time"

// Size limits for request processing
const (
	MaxEventSize = 4 * 1024 // 4KB max event payload
	MaxPathLen   = 2048     // max path length
	MaxRefLen    = 2048     // max referrer length
	MaxWidth     = 10000    // max reasonable screen width
)

// Timeout constants
const (
	HTTPReadTimeout     = 10 * time.Second // HTTP server read timeout
	HTTPWriteTimeout    = 10 * time.Second // HTTP server write timeout
	HTTPIdleTimeout     = 60 * time.Second // HTTP server idle timeout
	ShutdownTimeout     = 30 * time.Second // graceful shutdown timeout
	UniquesUpdatePeriod = 10 * time.Second // HLL gauge update interval
)
