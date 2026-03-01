package normalize

import (
	"strings"
	"testing"

	"notashelf.dev/watchdog/internal/config"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name  string
		cfg   config.PathConfig
		input string
		want  string
	}{
		{
			name: "strip query string",
			cfg: config.PathConfig{
				StripQuery: true,
			},
			input: "/page?utm_source=twitter&id=123",
			want:  "/page",
		},
		{
			name: "strip fragment",
			cfg: config.PathConfig{
				StripFragment: true,
			},
			input: "/page#section",
			want:  "/page",
		},
		{
			name: "collapse numeric segments",
			cfg: config.PathConfig{
				CollapseNumericSegments: true,
			},
			input: "/users/12345/profile",
			want:  "/users/:id/profile",
		},
		{
			name: "limit segments",
			cfg: config.PathConfig{
				MaxSegments: 3,
			},
			input: "/a/b/c/d/e/f",
			want:  "/a/b/c",
		},
		{
			name: "combined normalization",
			cfg: config.PathConfig{
				StripQuery:              true,
				StripFragment:           true,
				CollapseNumericSegments: true,
				MaxSegments:             5,
			},
			input: "/posts/2024/12/25/my-post?ref=home#comments",
			want:  "/posts/:id/:id/:id/my-post",
		},
		{
			name: "root path unchanged",
			cfg: config.PathConfig{
				StripQuery: true,
			},
			input: "/",
			want:  "/",
		},
		{
			name:  "empty path becomes root",
			cfg:   config.PathConfig{},
			input: "",
			want:  "/",
		},
		{
			name:  "path traversal with ..",
			cfg:   config.PathConfig{},
			input: "/a/../b",
			want:  "/b",
		},
		{
			name:  "path traversal with .",
			cfg:   config.PathConfig{},
			input: "/a/./b",
			want:  "/a/b",
		},
		{
			name:  "complex traversal",
			cfg:   config.PathConfig{},
			input: "/a/b/../c/./d",
			want:  "/a/c/d",
		},
		{
			name:  "traversal beyond root",
			cfg:   config.PathConfig{},
			input: "/../../../etc",
			want:  "/etc",
		},
		{
			name:  "double slashes",
			cfg:   config.PathConfig{},
			input: "/a//b///c",
			want:  "/a/b/c",
		},
		{
			name: "trailing slash normalization",
			cfg: config.PathConfig{
				NormalizeTrailingSlash: true,
			},
			input: "/users/",
			want:  "/users",
		},
		{
			name: "root trailing slash preserved",
			cfg: config.PathConfig{
				NormalizeTrailingSlash: true,
			},
			input: "/",
			want:  "/",
		},
		{
			name:  "very long path",
			cfg:   config.PathConfig{},
			input: "/" + strings.Repeat("a", 2050),
			want:  "/", // reject overly long paths
		},
		{
			name:  "dot segments only",
			cfg:   config.PathConfig{},
			input: "/./././",
			want:  "/",
		},
		{
			name:  "parent segments only",
			cfg:   config.PathConfig{},
			input: "/../..",
			want:  "/",
		},
		{
			name: "combined: traversal, slashes, and trailing slash",
			cfg: config.PathConfig{
				NormalizeTrailingSlash: true,
			},
			input: "/a//b/../c/./d/",
			want:  "/a/c/d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewPathNormalizer(tt.cfg)
			got := n.Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"123", true},
		{"0", true},
		{"abc", false},
		{"12abc", false},
		{"", false},
		{"2024-12-25", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isNumeric(tt.input)
			if got != tt.want {
				t.Errorf("isNumeric(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
