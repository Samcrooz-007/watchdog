package normalize

import "sync"

// Bounded set of observed referrer domains.
type ReferrerRegistry struct {
	mu            sync.RWMutex
	sources       map[string]struct{}
	maxSources    int
	overflowCount int
}

func NewReferrerRegistry(maxSources int) *ReferrerRegistry {
	return &ReferrerRegistry{
		sources:    make(map[string]struct{}, maxSources),
		maxSources: maxSources,
	}
}

// Attempt to add a referrer source to the registry. Returns the source to use ("other" if rejected).
func (r *ReferrerRegistry) Add(source string) string {
	if source == "direct" || source == "internal" {
		return source
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Already exists
	if _, exists := r.sources[source]; exists {
		return source
	}

	// Check limit
	if len(r.sources) >= r.maxSources {
		r.overflowCount++
		return "other"
	}

	r.sources[source] = struct{}{}
	return source
}

func (r *ReferrerRegistry) OverflowCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.overflowCount
}
