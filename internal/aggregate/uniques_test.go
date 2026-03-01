package aggregate

import (
	"testing"
	"time"
)

func TestDailySalt(t *testing.T) {
	// Same day should produce same salt
	day1Morning := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	day1Evening := time.Date(2024, 1, 15, 23, 59, 59, 0, time.UTC)

	salt1 := DailySalt(day1Morning)
	salt2 := DailySalt(day1Evening)

	if salt1 != salt2 {
		t.Errorf("Expected same salt for same day, got %s and %s", salt1, salt2)
	}

	// Different day should produce different salt
	day2 := time.Date(2024, 1, 16, 9, 0, 0, 0, time.UTC)
	salt3 := DailySalt(day2)

	if salt1 == salt3 {
		t.Errorf("Expected different salt for different days, got same: %s", salt1)
	}

	// Salt should be deterministic
	salt4 := DailySalt(day1Morning)
	if salt1 != salt4 {
		t.Errorf("Salt should be deterministic, got %s and %s", salt1, salt4)
	}
}

func TestUniquesEstimator(t *testing.T) {
	estimator := NewUniquesEstimator()

	// Initially should be zero
	if count := estimator.Estimate(); count != 0 {
		t.Errorf("Expected initial count of 0, got %d", count)
	}

	// Add a visitor
	estimator.Add("192.168.1.1", "Mozilla/5.0")
	if count := estimator.Estimate(); count != 1 {
		t.Errorf("Expected count of 1 after adding visitor, got %d", count)
	}

	// Adding same visitor should not increase count
	estimator.Add("192.168.1.1", "Mozilla/5.0")
	if count := estimator.Estimate(); count != 1 {
		t.Errorf("Expected count of 1 after adding duplicate, got %d", count)
	}

	// Adding different visitor should increase count
	estimator.Add("192.168.1.2", "Chrome/90.0")
	if count := estimator.Estimate(); count < 2 {
		t.Errorf("Expected count of at least 2 after adding different visitor, got %d", count)
	}

	// Test with many unique visitors
	for i := range 1000 {
		ip := "10.0." + string(rune(i/256)) + "." + string(rune(i%256))
		estimator.Add(ip, "TestAgent")
	}

	count := estimator.Estimate()
	// HyperLogLog has ~2% error rate, so 1000 uniques should be 980-1020
	if count < 980 || count > 1020 {
		t.Errorf("Expected count around 1000, got %d (error rate too high)", count)
	}
}

func TestUniquesEstimatorDailyRotation(t *testing.T) {
	// This test verifies that salts are different on different days
	// The actual rotation happens in Add() based on current time

	// Verify that different days produce different salts
	day1 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC)

	salt1 := DailySalt(day1)
	salt2 := DailySalt(day2)

	if salt1 == salt2 {
		t.Error("Expected different salts for different days")
	}

	// Verify estimator uses current day's salt
	estimator := NewUniquesEstimator()
	currentSalt := estimator.CurrentSalt()
	expectedSalt := DailySalt(time.Now())

	if currentSalt != expectedSalt {
		t.Errorf("Expected estimator to use current day's salt, got %s, expected %s", currentSalt, expectedSalt)
	}
}

func TestHashVisitor(t *testing.T) {
	// Same inputs should produce same hash
	hash1 := hashVisitor("192.168.1.1", "Mozilla/5.0", "salt123")
	hash2 := hashVisitor("192.168.1.1", "Mozilla/5.0", "salt123")

	if hash1 != hash2 {
		t.Error("Expected same hash for same inputs")
	}

	// Different IP should produce different hash
	hash3 := hashVisitor("192.168.1.2", "Mozilla/5.0", "salt123")
	if hash1 == hash3 {
		t.Error("Expected different hash for different IP")
	}

	// Different user agent should produce different hash
	hash4 := hashVisitor("192.168.1.1", "Chrome/90.0", "salt123")
	if hash1 == hash4 {
		t.Error("Expected different hash for different user agent")
	}

	// Different salt should produce different hash
	hash5 := hashVisitor("192.168.1.1", "Mozilla/5.0", "salt456")
	if hash1 == hash5 {
		t.Error("Expected different hash for different salt")
	}
}
