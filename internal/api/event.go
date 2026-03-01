package api

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	maxEventSize = 4 * 1024 // 4KB
	maxPathLen   = 2048
	maxRefLen    = 2048
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
	limited := io.LimitReader(body, maxEventSize+1)

	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	if len(data) > maxEventSize {
		return nil, fmt.Errorf("event payload too large: %d bytes (max %d)", len(data), maxEventSize)
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return &event, nil
}

// Validate checks if the event is valid for the given domain
func (e *Event) Validate(expectedDomain string) error {
	if e.Domain == "" {
		return fmt.Errorf("domain is required")
	}

	if e.Domain != expectedDomain {
		return fmt.Errorf("domain mismatch: got %q, expected %q", e.Domain, expectedDomain)
	}

	if e.Path == "" {
		return fmt.Errorf("path is required")
	}

	if len(e.Path) > maxPathLen {
		return fmt.Errorf("path too long: %d bytes (max %d)", len(e.Path), maxPathLen)
	}

	if len(e.Referrer) > maxRefLen {
		return fmt.Errorf("referrer too long: %d bytes (max %d)", len(e.Referrer), maxRefLen)
	}

	return nil
}
