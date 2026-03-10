package normalize

import (
	"strings"

	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/limits"
)

type PathNormalizer struct {
	cfg       config.PathConfig
	maxLength int
}

func NewPathNormalizer(cfg config.PathConfig) *PathNormalizer {
	return &PathNormalizer{
		cfg:       cfg,
		maxLength: limits.MaxPathLen,
	}
}

func (n *PathNormalizer) Normalize(path string) string {
	// Reject paths that are too long; don't bypass normalization
	if len(path) > n.maxLength {
		return "/"
	}

	if path == "" {
		return "/"
	}

	// Strip query string
	if n.cfg.StripQuery {
		if idx := strings.IndexByte(path, '?'); idx != -1 {
			path = path[:idx]
		}
	}

	// Strip fragment
	if n.cfg.StripFragment {
		if idx := strings.IndexByte(path, '#'); idx != -1 {
			path = path[:idx]
		}
	}

	// Ensure leading slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Process segments in-place to minimize allocations
	// Split into segments, first element is *always* empty for paths starting with '/'
	segments := strings.Split(path, "/")

	// Process segments in a single pass: remove empty, resolve . and ..
	writeIdx := 0
	for i := 0; i < len(segments); i++ {
		seg := segments[i]

		// Skip empty segments (from double slashes or leading /)
		if seg == "" {
			continue
		}

		if seg == "." {
			// Skip current directory
			continue
		} else if seg == ".." {
			// Go up one level if possible
			if writeIdx > 0 {
				writeIdx--
			}
			// If already at root, skip ..
		} else {
			// Keep this segment
			segments[writeIdx] = seg
			writeIdx++
		}
	}
	segments = segments[:writeIdx]

	// Collapse numeric segments
	if n.cfg.CollapseNumericSegments {
		for i, seg := range segments {
			if isNumeric(seg) {
				segments[i] = ":id"
			}
		}
	}

	// Limit segments
	if n.cfg.MaxSegments > 0 && len(segments) > n.cfg.MaxSegments {
		segments = segments[:n.cfg.MaxSegments]
	}

	// Reconstruct path
	var result string
	if len(segments) == 0 {
		result = "/"
	} else {
		result = "/" + strings.Join(segments, "/")
	}

	// Strip trailing slash if configured (except root)
	if n.cfg.NormalizeTrailingSlash && result != "/" && strings.HasSuffix(result, "/") {
		result = strings.TrimSuffix(result, "/")
	}

	return result
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
