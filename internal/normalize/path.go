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

	// Split into segments, first element is *always* empty for paths starting with '/'
	segments := strings.Split(path, "/")
	if len(segments) > 0 && segments[0] == "" {
		segments = segments[1:]
	}

	// Remove empty segments (from double slashes)
	filtered := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			filtered = append(filtered, seg)
		}
	}
	segments = filtered

	// Resolve . and .. segments
	resolved := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "." {
			// Skip current directory
			continue
		} else if seg == ".." {
			// Go up one level if possible
			if len(resolved) > 0 {
				resolved = resolved[:len(resolved)-1]
			}
			// If already at root, skip ..
		} else {
			resolved = append(resolved, seg)
		}
	}
	segments = resolved

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
