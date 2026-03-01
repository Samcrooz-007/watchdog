package aggregate

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/axiomhq/hyperloglog"
)

// UniquesEstimator tracks unique visitors using HyperLogLog with daily salt rotation
type UniquesEstimator struct {
	hll        *hyperloglog.Sketch
	currentDay string
	mu         sync.Mutex
}

// NewUniquesEstimator creates a new unique visitor estimator
func NewUniquesEstimator() *UniquesEstimator {
	return &UniquesEstimator{
		hll:        hyperloglog.New(),
		currentDay: dailySalt(time.Now()),
	}
}

// Add records a visitor with privacy-preserving hashing
// Uses IP + UserAgent + daily salt to prevent cross-day correlation
func (u *UniquesEstimator) Add(ip, userAgent string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Check if we need to rotate to a new day
	today := dailySalt(time.Now())
	if today != u.currentDay {
		// Reset HLL for new day
		u.hll = hyperloglog.New()
		u.currentDay = today
	}

	// Hash visitor with daily salt to prevent cross-day tracking
	hash := hashVisitor(ip, userAgent, u.currentDay)
	u.hll.Insert([]byte(hash))
}

// Estimate returns the estimated number of unique visitors
func (u *UniquesEstimator) Estimate() uint64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hll.Estimate()
}

// dailySalt generates a deterministic salt based on the current date
// Same day = same salt, different day = different salt
func dailySalt(t time.Time) string {
	// Use UTC to ensure consistent rotation regardless of timezone
	date := t.UTC().Format("2006-01-02")
	h := sha256.Sum256([]byte("watchdog-salt-" + date))
	return hex.EncodeToString(h[:])
}

// hashVisitor creates a privacy-preserving hash of visitor identity
func hashVisitor(ip, userAgent, salt string) string {
	combined := ip + "|" + userAgent + "|" + salt
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:])
}

// CurrentSalt returns the current salt for testing
func (u *UniquesEstimator) CurrentSalt() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.currentDay
}

// DailySalt is exported for testing
func DailySalt(t time.Time) string {
	return dailySalt(t)
}
