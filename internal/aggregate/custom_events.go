package aggregate

import "sync"

// Maintains a bounded set of allowed custom event names
type CustomEventRegistry struct {
	mu            sync.RWMutex
	events        map[string]struct{}
	maxEvents     int
	overflowCount int
}

// Creates a registry with a maximum number of unique event names
func NewCustomEventRegistry(maxEvents int) *CustomEventRegistry {
	return &CustomEventRegistry{
		events:    make(map[string]struct{}, maxEvents),
		maxEvents: maxEvents,
	}
}

// Add attempts to add an event name to the registry
// Returns the event name if accepted, "other" if rejected due to limit
func (r *CustomEventRegistry) Add(eventName string) string {
	// Fast path: check with read lock first
	r.mu.RLock()
	if _, exists := r.events[eventName]; exists {
		r.mu.RUnlock()
		return eventName
	}
	r.mu.RUnlock()

	// Slow path: acquire write lock to add
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if _, exists := r.events[eventName]; exists {
		return eventName
	}

	// Check limit
	if len(r.events) >= r.maxEvents {
		r.overflowCount++
		return "other"
	}

	r.events[eventName] = struct{}{}
	return eventName
}

// Returns the number of events rejected due to limit
func (r *CustomEventRegistry) OverflowCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.overflowCount
}

// Contains checks if an event name exists in the registry.
func (r *CustomEventRegistry) Contains(eventName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.events[eventName]
	return exists
}

// Count returns the number of unique events in the registry.
func (r *CustomEventRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.events)
}
