package aggregate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/axiomhq/hyperloglog"
)

// Tracks unique visitors using HyperLogLog with configurable salt rotation
type UniquesEstimator struct {
	hll      *hyperloglog.Sketch
	salt     string
	rotation string // "daily" or "hourly"
	mu       sync.Mutex
}

// Creates a new unique visitor estimator
func NewUniquesEstimator(rotation string) *UniquesEstimator {
	return &UniquesEstimator{
		hll:      hyperloglog.New(),
		salt:     generateSalt(time.Now(), rotation),
		rotation: rotation,
	}
}

// Add records a visitor with privacy-preserving hashing
// Uses IP + UserAgent + salt to prevent cross-period correlation
func (u *UniquesEstimator) Add(ip, userAgent string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Check if we need to rotate to a new period
	currentSalt := generateSalt(time.Now(), u.rotation)
	if currentSalt != u.salt {
		// Reset HLL for new period
		u.hll = hyperloglog.New()
		u.salt = currentSalt
	}

	// Hash visitor with salt to prevent cross-period tracking
	hash := hashVisitor(ip, userAgent, u.salt)
	u.hll.Insert([]byte(hash))
}

// Estimate returns the estimated number of unique visitors
func (u *UniquesEstimator) Estimate() uint64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hll.Estimate()
}

// Generates a deterministic salt based on the rotation mode
// Daily: same day = same salt, different day = different salt
// Hourly: same hour = same salt, different hour = different salt
func generateSalt(t time.Time, rotation string) string {
	var key string
	if rotation == "hourly" {
		key = t.UTC().Format("2006-01-02T15")
	} else {
		key = t.UTC().Format("2006-01-02")
	}
	h := sha256.Sum256([]byte("watchdog-salt-" + key))
	return hex.EncodeToString(h[:])
}

// Creates a privacy-preserving hash of visitor identity
func hashVisitor(ip, userAgent, salt string) string {
	var sb strings.Builder
	sb.WriteString(ip)
	sb.WriteString("|")
	sb.WriteString(userAgent)
	sb.WriteString("|")
	sb.WriteString(salt)
	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:])
}

// Returns the current salt for testing
func (u *UniquesEstimator) CurrentSalt() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.salt
}

// Exported for testing
func DailySalt(t time.Time) string {
	return generateSalt(t, "daily")
}

// Save persists the HLL state to disk
func (u *UniquesEstimator) Save(path string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	data, err := u.hll.MarshalBinary()
	if err != nil {
		return err
	}

	// Save both HLL data and current salt
	return os.WriteFile(path, append([]byte(u.salt+"\n"), data...), 0600)
}

// Load restores the HLL state from disk
func (u *UniquesEstimator) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // file not existing is OK (first run)
		}
		return err // other errors should be reported
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	// Parse saved salt and HLL data
	parts := bytes.SplitN(data, []byte("\n"), 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid state file format")
	}

	savedSalt := string(parts[0])
	currentSalt := generateSalt(time.Now(), u.rotation)

	// Only restore if it's the same period
	if savedSalt == currentSalt {
		u.salt = savedSalt
		return u.hll.UnmarshalBinary(parts[1])
	}

	// Different period, start fresh
	u.hll = hyperloglog.New()
	u.salt = currentSalt
	return nil
}
