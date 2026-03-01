package api

import (
	"strings"
	"testing"
)

func TestParseEvent_Valid(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Event
	}{
		{
			name: "pageview with all fields",
			body: `{"d":"example.com","p":"/home","r":"https://google.com","e":"","w":1024}`,
			want: Event{
				Domain:   "example.com",
				Path:     "/home",
				Referrer: "https://google.com",
				Event:    "",
				Width:    1024,
			},
		},
		{
			name: "custom event",
			body: `{"d":"example.com","p":"/signup","r":"","e":"signup","w":0}`,
			want: Event{
				Domain:   "example.com",
				Path:     "/signup",
				Referrer: "",
				Event:    "signup",
				Width:    0,
			},
		},
		{
			name: "minimal fields",
			body: `{"d":"example.com","p":"/"}`,
			want: Event{
				Domain: "example.com",
				Path:   "/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEvent(strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("ParseEvent() error = %v", err)
			}

			if got.Domain != tt.want.Domain {
				t.Errorf("Domain = %v, want %v", got.Domain, tt.want.Domain)
			}
			if got.Path != tt.want.Path {
				t.Errorf("Path = %v, want %v", got.Path, tt.want.Path)
			}
			if got.Referrer != tt.want.Referrer {
				t.Errorf("Referrer = %v, want %v", got.Referrer, tt.want.Referrer)
			}
			if got.Event != tt.want.Event {
				t.Errorf("Event = %v, want %v", got.Event, tt.want.Event)
			}
			if got.Width != tt.want.Width {
				t.Errorf("Width = %v, want %v", got.Width, tt.want.Width)
			}
		})
	}
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid json",
			body: `{invalid json}`,
		},
		{
			name: "empty body",
			body: ``,
		},
		{
			name: "truncated json",
			body: `{"d":"example.com"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEvent(strings.NewReader(tt.body))
			if err == nil {
				t.Error("expected error for invalid JSON, got nil")
			}
		})
	}
}

func TestParseEvent_TooLarge(t *testing.T) {
	// Create a payload larger than maxEventSize (4KB)
	largeBody := `{"d":"example.com","p":"` + strings.Repeat("a", 5000) + `"}`

	_, err := ParseEvent(strings.NewReader(largeBody))
	if err == nil {
		t.Error("expected error for too large payload, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got: %v", err)
	}
}

func TestValidateEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		domains []string
		wantErr bool
	}{
		{
			name: "valid pageview",
			event: Event{
				Domain: "example.com",
				Path:   "/home",
			},
			domains: []string{"example.com"},
			wantErr: false,
		},
		{
			name: "valid custom event",
			event: Event{
				Domain: "example.com",
				Path:   "/signup",
				Event:  "signup",
			},
			domains: []string{"example.com"},
			wantErr: false,
		},
		{
			name: "wrong domain",
			event: Event{
				Domain: "wrong.com",
				Path:   "/home",
			},
			domains: []string{"example.com"},
			wantErr: true,
		},
		{
			name: "empty domain",
			event: Event{
				Domain: "",
				Path:   "/home",
			},
			domains: []string{"example.com"},
			wantErr: true,
		},
		{
			name: "empty path",
			event: Event{
				Domain: "example.com",
				Path:   "",
			},
			domains: []string{"example.com"},
			wantErr: true,
		},
		{
			name: "path too long",
			event: Event{
				Domain: "example.com",
				Path:   "/" + strings.Repeat("a", 3000),
			},
			domains: []string{"example.com"},
			wantErr: true,
		},
		{
			name: "referrer too long",
			event: Event{
				Domain:   "example.com",
				Path:     "/home",
				Referrer: strings.Repeat("a", 3000),
			},
			domains: []string{"example.com"},
			wantErr: true,
		},
		{
			name: "valid long path (under limit)",
			event: Event{
				Domain: "example.com",
				Path:   "/" + strings.Repeat("a", 2000),
			},
			domains: []string{"example.com"},
			wantErr: false,
		},
		{
			name: "multi-site valid domain 1",
			event: Event{
				Domain: "site1.com",
				Path:   "/home",
			},
			domains: []string{"site1.com", "site2.com"},
			wantErr: false,
		},
		{
			name: "multi-site valid domain 2",
			event: Event{
				Domain: "site2.com",
				Path:   "/about",
			},
			domains: []string{"site1.com", "site2.com"},
			wantErr: false,
		},
		{
			name: "multi-site invalid domain",
			event: Event{
				Domain: "site3.com",
				Path:   "/home",
			},
			domains: []string{"site1.com", "site2.com"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate(tt.domains)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
