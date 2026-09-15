package app

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const (
	appPasswordFailureWindow = 15 * time.Minute
	appPasswordFailureLimit  = 3
	appPasswordInitialBlock  = time.Second
	appPasswordMaxBlock      = time.Minute
	appPasswordMaxEntries    = 4096
)

type appPasswordFailure struct {
	count       int
	lastFailure time.Time
	blockedTo   time.Time
}

type appPasswordThrottle struct {
	mu      sync.Mutex
	entries map[string]appPasswordFailure
	now     func() time.Time
}

var appPasswordLimiter = newAppPasswordThrottle(time.Now)

func newAppPasswordThrottle(now func() time.Time) *appPasswordThrottle {
	return &appPasswordThrottle{
		entries: make(map[string]appPasswordFailure),
		now:     now,
	}
}

func appPasswordThrottleKey(kind, value string) string {
	digest := sha256.Sum256([]byte(value))
	return kind + ":" + hex.EncodeToString(digest[:])
}

func (t *appPasswordThrottle) allowed(keys ...string) bool {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()

	allowed := true
	for _, key := range keys {
		if key == "" {
			continue
		}
		state, found := t.entries[key]
		if !found {
			continue
		}
		if !now.Before(state.lastFailure.Add(appPasswordFailureWindow)) {
			delete(t.entries, key)
			continue
		}
		if now.Before(state.blockedTo) {
			allowed = false
		}
	}
	return allowed
}

func (t *appPasswordThrottle) recordFailure(keys ...string) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, key := range keys {
		if key == "" {
			continue
		}
		state, found := t.entries[key]
		if !found {
			if len(t.entries) >= appPasswordMaxEntries {
				t.removeOldest()
			}
			state = appPasswordFailure{}
		}
		if !state.lastFailure.IsZero() && !now.Before(state.lastFailure.Add(appPasswordFailureWindow)) {
			state = appPasswordFailure{}
		}
		state.count++
		state.lastFailure = now
		if state.count >= appPasswordFailureLimit {
			state.blockedTo = now.Add(appPasswordBlockDuration(state.count))
		}
		t.entries[key] = state
	}
}

func appPasswordBlockDuration(failures int) time.Duration {
	duration := appPasswordInitialBlock
	for count := appPasswordFailureLimit; count < failures; count++ {
		if duration >= appPasswordMaxBlock/2 {
			return appPasswordMaxBlock
		}
		duration *= 2
	}
	return duration
}

func (t *appPasswordThrottle) reset(keys ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, key := range keys {
		if key != "" {
			delete(t.entries, key)
		}
	}
}

func (t *appPasswordThrottle) removeOldest() {
	var oldestKey string
	var oldest time.Time
	for key, state := range t.entries {
		if oldestKey == "" || state.lastFailure.Before(oldest) {
			oldestKey = key
			oldest = state.lastFailure
		}
	}
	if oldestKey != "" {
		delete(t.entries, oldestKey)
	}
}
