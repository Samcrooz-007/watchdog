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
	saltKey  string // cached time key to avoid regeneration
	mu       sync.Mutex
}

// Creates a new unique visitor estimator
func NewUniquesEstimator(rotation string) *UniquesEstimator {
	now := time.Now()
	return &UniquesEstimator{
		hll:      hyperloglog.New(),
		salt:     generateSalt(now, rotation),
		rotation: rotation,
		saltKey:  getSaltKey(now, rotation),
	}
}

// Add records a visitor with privacy-preserving hashing
// Uses IP + UserAgent + salt to prevent cross-period correlation
func (u *UniquesEstimator) Add(ip, userAgent string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Check if we need to rotate to a new period
	now := time.Now()
	currentKey := getSaltKey(now, u.rotation)
	if currentKey != u.saltKey {
		// Reset HLL for new period
		u.hll = hyperloglog.New()
		u.salt = generateSaltFromKey(currentKey)
		u.saltKey = currentKey
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

// Returns the time-based key for salt generation without hashing
func getSaltKey(t time.Time, rotation string) string {
	if rotation == "hourly" {
		return t.UTC().Format("2006-01-02T15")
	}
	return t.UTC().Format("2006-01-02")
}

// Creates a salt from a pre-computed key
func generateSaltFromKey(key string) string {
	h := sha256.Sum256([]byte("watchdog-salt-" + key))
	return hex.EncodeToString(h[:])
}

// Generates a deterministic salt based on the rotation mode
// Daily: same day = same salt, different day = different salt
// Hourly: same hour = same salt, different hour = different salt
func generateSalt(t time.Time, rotation string) string {
	return generateSaltFromKey(getSaltKey(t, rotation))
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
// Format: saltKey\nsalt\nHLLdata
func (u *UniquesEstimator) Save(path string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	data, err := u.hll.MarshalBinary()
	if err != nil {
		return err
	}

	// Save saltKey, salt, and HLL data
	var buf bytes.Buffer
	buf.WriteString(u.saltKey)
	buf.WriteByte('\n')
	buf.WriteString(u.salt)
	buf.WriteByte('\n')
	buf.Write(data)

	return os.WriteFile(path, buf.Bytes(), 0600)
}

// Load restores the HLL state from disk
// Supports both new format (saltKey\nsalt\nHLLdata) and old format (salt\nHLLdata)
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

	// Try new format first: saltKey\nsalt\nHLLdata
	parts := bytes.SplitN(data, []byte("\n"), 3)
	if len(parts) == 3 {
		savedSaltKey := string(parts[0])
		savedSalt := string(parts[1])
		hllData := parts[2]

		now := time.Now()
		currentKey := getSaltKey(now, u.rotation)

		// Only restore if it's the same period
		if savedSaltKey == currentKey {
			u.salt = savedSalt
			u.saltKey = savedSaltKey
			return u.hll.UnmarshalBinary(hllData)
		}

		// Different period, start fresh
		u.hll = hyperloglog.New()
		u.salt = generateSaltFromKey(currentKey)
		u.saltKey = currentKey
		return nil
	}

	// Try old format for backward compatibility: salt\nHLLdata
	parts = bytes.SplitN(data, []byte("\n"), 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid state file format")
	}

	savedSalt := string(parts[0])
	now := time.Now()
	currentKey := getSaltKey(now, u.rotation)
	currentSalt := generateSaltFromKey(currentKey)

	// Only restore if it's the same period
	if savedSalt == currentSalt {
		u.salt = savedSalt
		u.saltKey = currentKey
		return u.hll.UnmarshalBinary(parts[1])
	}

	// Different period, start fresh
	u.hll = hyperloglog.New()
	u.salt = currentSalt
	u.saltKey = currentKey
	return nil
}
