package watchdog

import (
	"strings"
	"testing"
)

func TestSanitizePathForLog(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		maxLen int // expected max length check
	}{
		{
			name:   "normal path",
			input:  "/web/beacon.js",
			want:   "/web/beacon.js",
			maxLen: 200,
		},
		{
			name:   "path with newlines",
			input:  "/test\nmalicious",
			want:   `/test\nmalicious`,
			maxLen: 200,
		},
		{
			name:   "path with carriage return",
			input:  "/test\rmalicious",
			want:   `/test\rmalicious`,
			maxLen: 200,
		},
		{
			name:   "path with tabs",
			input:  "/test\tmalicious",
			want:   `/test\tmalicious`,
			maxLen: 200,
		},
		{
			name:   "path with null bytes",
			input:  "/test\x00malicious",
			want:   `/test\x00malicious`,
			maxLen: 200,
		},
		{
			name:   "path with quotes",
			input:  `/test"malicious`,
			want:   `/test\"malicious`,
			maxLen: 200,
		},
		{
			name:   "path with backslash",
			input:  `/test\malicious`,
			want:   `/test\\malicious`,
			maxLen: 200,
		},
		{
			name:   "control characters",
			input:  "/test\x01\x02\x1fmalicious",
			want:   `/test\x01\x02\x1fmalicious`,
			maxLen: 200,
		},
		{
			name:   "truncation at 200 chars",
			input:  "/" + strings.Repeat("a", 250),
			want:   "/" + strings.Repeat("a", 199) + "...",
			maxLen: 203, // 200 chars + "..." = 203
		},
		{
			name:   "empty string",
			input:  "",
			want:   "",
			maxLen: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizePathForLog(tt.input)
			if got != tt.want {
				t.Errorf("sanitizePathForLog(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("sanitizePathForLog(%q) length = %d, exceeds max %d", tt.input, len(got), tt.maxLen)
			}
		})
	}
}

func TestSanitizePathForLog_LogInjectionPrevention(t *testing.T) {
	// Log injection attempts should be neutralized
	maliciousPaths := []string{
		"/api\nINFO: fake log entry",
		"/test\r\nERROR: fake error",
		"/.git/config\x00", // null byte injection
	}

	for _, path := range maliciousPaths {
		sanitized := sanitizePathForLog(path)
		// Check that newlines are escaped, not literal
		if strings.Contains(sanitized, "\n") || strings.Contains(sanitized, "\r") {
			t.Errorf("sanitizePathForLog(%q) contains literal newlines: %q", path, sanitized)
		}
		// Check that null bytes are escaped
		if strings.Contains(sanitized, "\x00") {
			t.Errorf("sanitizePathForLog(%q) contains literal null byte: %q", path, sanitized)
		}
	}
}

func BenchmarkSanitizePathForLog(b *testing.B) {
	path := "/test/path/with\nnewlines\rand\ttabs"
	for b.Loop() {
		_ = sanitizePathForLog(path)
	}
}
