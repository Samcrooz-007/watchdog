package limits

// Size limits for request processing
const (
	MaxEventSize = 4 * 1024 // 4KB max event payload
	MaxPathLen   = 2048     // max path length
	MaxRefLen    = 2048     // max referrer length
	MaxWidth     = 10000    // max reasonable screen width
)
