package api

import (
	"encoding/json"
	"fmt"
	"io"

	"notashelf.dev/watchdog/internal/limits"
)

// Represents an incoming analytics event
type Event struct {
	Domain   string `json:"d"` // domain
	Path     string `json:"p"` // path
	Referrer string `json:"r"` // referrer URL
	Event    string `json:"e"` // custom event name (empty for pageviews)
	Width    int    `json:"w"` // screen width for device classification
}

// Parses an event from the request body with size limits
func ParseEvent(body io.Reader) (*Event, error) {
	// Limit read size to prevent memory exhaustion
	limited := io.LimitReader(body, limits.MaxEventSize+1)

	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	if len(data) > limits.MaxEventSize {
		return nil, fmt.Errorf("event payload too large")
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("invalid JSON")
	}

	return &event, nil
}

// Validate checks if the event is valid for the given domains
func (e *Event) Validate(allowedDomains []string) error {
	if e.Domain == "" {
		return fmt.Errorf("domain required")
	}

	// Check if domain is in allowed list
	allowed := false
	for _, domain := range allowedDomains {
		if e.Domain == domain {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("domain not allowed")
	}

	if e.Path == "" {
		return fmt.Errorf("path required")
	}

	if len(e.Path) > limits.MaxPathLen {
		return fmt.Errorf("path too long")
	}

	if len(e.Referrer) > limits.MaxRefLen {
		return fmt.Errorf("referrer too long")
	}

	// Validate screen width is in reasonable range
	if e.Width < 0 || e.Width > limits.MaxWidth {
		return fmt.Errorf("invalid width")
	}

	return nil
}
