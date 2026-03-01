package aggregate

import (
	"sync"
)

// Maintain a bounded set of unique request paths. This prevents metric cardinality explosion by rejecting new paths
// once the configured limit is reached.
type PathRegistry struct {
	mu            sync.RWMutex
	paths         map[string]struct{}
	maxPaths      int
	overflowCount int
}

// Creates a new PathRegistry with the specified maximum number of unique paths.
// Once this limit is reached, subsequent Add() calls for new paths will be rejected.
func NewPathRegistry(maxPaths int) *PathRegistry {
	return &PathRegistry{
		paths:    make(map[string]struct{}, maxPaths),
		maxPaths: maxPaths,
	}
}

// Add attempts to add a path to the registry.
// Returns true if the path was accepted: either already existed or was added,
// false if rejected due to reaching the limit.
func (r *PathRegistry) Add(path string) bool {
	// Fast path: check with read lock first
	r.mu.RLock()
	if _, exists := r.paths[path]; exists {
		r.mu.RUnlock()
		return true
	}
	r.mu.RUnlock()

	// Slow path: acquire write lock to add
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if _, exists := r.paths[path]; exists {
		return true
	}

	// If we haven't reached the limit, add the path
	if len(r.paths) < r.maxPaths {
		r.paths[path] = struct{}{}
		return true
	}

	// Limit reached, reject and increment overflow
	r.overflowCount++
	return false
}

// Contains checks if a path exists in the registry.
func (r *PathRegistry) Contains(path string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.paths[path]
	return exists
}

// Count returns the number of unique paths in the registry.
func (r *PathRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.paths)
}

// Returns the number of paths that were rejected due to the registry being at
// capacity.
func (r *PathRegistry) OverflowCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.overflowCount
}
