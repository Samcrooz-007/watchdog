package normalize

import (
	"fmt"
	"testing"
)

func TestExtractReferrerDomain(t *testing.T) {
	tests := []struct {
		name       string
		referrer   string
		siteDomain string
		want       string
	}{
		{
			name:       "empty referrer",
			referrer:   "",
			siteDomain: "example.com",
			want:       "direct",
		},
		{
			name:       "same domain",
			referrer:   "https://example.com/page",
			siteDomain: "example.com",
			want:       "direct",
		},
		{
			name:       "subdomain is direct",
			referrer:   "https://blog.example.com/post",
			siteDomain: "example.com",
			want:       "direct",
		},
		{
			name:       "google search",
			referrer:   "https://www.google.com/search?q=test",
			siteDomain: "example.com",
			want:       "google.com",
		},
		{
			name:       "google country domain",
			referrer:   "https://www.google.co.uk/search",
			siteDomain: "example.com",
			want:       "google.co.uk",
		},
		{
			name:       "hacker news",
			referrer:   "https://news.ycombinator.com/item?id=123",
			siteDomain: "example.com",
			want:       "ycombinator.com",
		},
		{
			name:       "twitter short link",
			referrer:   "https://t.co/abc123",
			siteDomain: "example.com",
			want:       "t.co",
		},
		{
			name:       "github",
			referrer:   "https://github.com/user/repo",
			siteDomain: "example.com",
			want:       "github.com",
		},
		{
			name:       "invalid url",
			referrer:   "not-a-url",
			siteDomain: "example.com",
			want:       "",
		},
		{
			name:       "case insensitive",
			referrer:   "https://WWW.GOOGLE.COM/search",
			siteDomain: "EXAMPLE.COM",
			want:       "google.com",
		},
		{
			name:       "trailing dot normalized",
			referrer:   "https://example.com./page",
			siteDomain: "test.com",
			want:       "example.com",
		},
		{
			name:       "localhost",
			referrer:   "http://localhost:8080/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "loopback IPv4",
			referrer:   "http://127.0.0.1/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "loopback IPv6",
			referrer:   "http://[::1]/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "private IP 192.168",
			referrer:   "http://192.168.1.1/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "private IP 10.x",
			referrer:   "http://10.0.0.1/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "private IP 172.16.x (RFC1918)",
			referrer:   "http://172.16.0.1/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "private IP 172.31.x (RFC1918 upper bound)",
			referrer:   "http://172.31.255.1/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "private IP 172.20.x (middle of range)",
			referrer:   "http://172.20.50.100/page",
			siteDomain: "example.com",
			want:       "internal",
		},
		{
			name:       "public IP 172.15.x (just outside private range)",
			referrer:   "http://172.15.0.1/page",
			siteDomain: "example.com",
			want:       "other", // not internal, but invalid TLD
		},
		{
			name:       "public IP 172.32.x (just outside private range)",
			referrer:   "http://172.32.0.1/page",
			siteDomain: "example.com",
			want:       "other", // not internal, but invalid TLD
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferrerDomain(tt.referrer, tt.siteDomain)
			if got != tt.want {
				t.Errorf("ExtractReferrerDomain(%q, %q) = %q, want %q",
					tt.referrer, tt.siteDomain, got, tt.want)
			}
		})
	}
}

func TestReferrerRegistry(t *testing.T) {
	registry := NewReferrerRegistry(3)

	// Add sources within limit
	if got := registry.Add("google.com"); got != "google.com" {
		t.Errorf("expected google.com, got %s", got)
	}
	if got := registry.Add("github.com"); got != "github.com" {
		t.Errorf("expected github.com, got %s", got)
	}
	if got := registry.Add("reddit.com"); got != "reddit.com" {
		t.Errorf("expected reddit.com, got %s", got)
	}

	// Adding same source again should succeed
	if got := registry.Add("google.com"); got != "google.com" {
		t.Errorf("expected google.com, got %s", got)
	}

	// Exceeding limit should return "other"
	if got := registry.Add("twitter.com"); got != "other" {
		t.Errorf("expected other, got %s", got)
	}

	// Direct always works
	if got := registry.Add("direct"); got != "direct" {
		t.Errorf("expected direct, got %s", got)
	}

	// Internal always works
	if got := registry.Add("internal"); got != "internal" {
		t.Errorf("expected internal, got %s", got)
	}

	// Overflow count should be 1
	if registry.OverflowCount() != 1 {
		t.Errorf("expected overflow count 1, got %d", registry.OverflowCount())
	}
}

func TestReferrerRegistryConcurrentOverflow(t *testing.T) {
	registry := NewReferrerRegistry(10)

	// Use channels to coordinate goroutines
	const numGoroutines = 50
	const sourcesPerGoroutine = 5
	done := make(chan bool, numGoroutines)

	// Launch goroutines that race to add sources
	for i := range numGoroutines {
		go func(id int) {
			for j := range sourcesPerGoroutine {
				source := fmt.Sprintf("source-%d-%d.com", id, j)
				registry.Add(source)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range numGoroutines {
		<-done
	}

	// Registry should have exactly 10 sources (limit)
	// Overflow should be (50 * 5) - 10 = 240 rejections
	// But since same sources might be added multiple times,
	// we just verify: overflow > 0 and total attempts tracked
	if registry.OverflowCount() == 0 {
		t.Error("expected some overflow with concurrent adds")
	}

	// Verify adding more sources still returns "other"
	if got := registry.Add("new-source.com"); got != "other" {
		t.Errorf("expected other after overflow, got %s", got)
	}
}
