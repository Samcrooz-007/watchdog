package aggregate

import (
	"testing"
)

func TestPathRegistry_Add(t *testing.T) {
	registry := NewPathRegistry(3)

	// Add paths within limit
	if !registry.Add("/api/users") {
		t.Error("Expected first path to be accepted")
	}
	if !registry.Add("/api/posts") {
		t.Error("Expected second path to be accepted")
	}
	if !registry.Add("/api/comments") {
		t.Error("Expected third path to be accepted")
	}

	// Add duplicate path - should succeed
	if !registry.Add("/api/users") {
		t.Error("Expected duplicate path to be accepted")
	}

	// Verify count is still 3
	if count := registry.Count(); count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}

	// Exceed limit
	if registry.Add("/api/photos") {
		t.Error("Expected fourth unique path to be rejected")
	}

	// Verify overflow count
	if overflow := registry.OverflowCount(); overflow != 1 {
		t.Errorf("Expected overflow count 1, got %d", overflow)
	}

	// Add another path beyond limit
	if registry.Add("/api/videos") {
		t.Error("Expected fifth unique path to be rejected")
	}

	// Verify overflow count incremented
	if overflow := registry.OverflowCount(); overflow != 2 {
		t.Errorf("Expected overflow count 2, got %d", overflow)
	}

	// Verify count is still 3
	if count := registry.Count(); count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func TestPathRegistry_Contains(t *testing.T) {
	registry := NewPathRegistry(3)

	registry.Add("/api/users")
	registry.Add("/api/posts")

	if !registry.Contains("/api/users") {
		t.Error("Expected /api/users to be in registry")
	}

	if !registry.Contains("/api/posts") {
		t.Error("Expected /api/posts to be in registry")
	}

	if registry.Contains("/api/comments") {
		t.Error("Expected /api/comments to NOT be in registry")
	}
}

func TestPathRegistry_Count(t *testing.T) {
	registry := NewPathRegistry(5)

	if count := registry.Count(); count != 0 {
		t.Errorf("Expected initial count 0, got %d", count)
	}

	registry.Add("/api/users")
	if count := registry.Count(); count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}

	registry.Add("/api/posts")
	if count := registry.Count(); count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}

	// Add duplicate - count should not change
	registry.Add("/api/users")
	if count := registry.Count(); count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}
