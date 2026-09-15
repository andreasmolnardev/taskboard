package reminders

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrDuplicate = errors.New("reminders: duplicate reminder")

// MemoryStore is a concurrency-safe fake for tests and isolated integration.
type MemoryStore struct {
	mu        sync.Mutex
	reminders map[string]Reminder
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{reminders: make(map[string]Reminder)}
}

func (s *MemoryStore) Add(ctx context.Context, reminder Reminder) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.reminders[reminder.Key]; exists {
		return ErrDuplicate
	}
	s.reminders[reminder.Key] = reminder
	return nil
}

func (s *MemoryStore) ClaimDue(ctx context.Context, now time.Time, limit int) ([]Reminder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, errors.New("reminders: claim limit must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.reminders))
	for key, reminder := range s.reminders {
		if dueForClaim(reminder, now) {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := s.reminders[keys[i]], s.reminders[keys[j]]
		if left.NextAttemptAt.Equal(right.NextAttemptAt) {
			return left.Key < right.Key
		}
		return left.NextAttemptAt.Before(right.NextAttemptAt)
	})
	if len(keys) > limit {
		keys = keys[:limit]
	}

	claimed := make([]Reminder, 0, len(keys))
	for _, key := range keys {
		reminder := s.reminders[key]
		reminder.Status = StatusProcessing
		reminder.NextAttemptAt = now.UTC().Add(ProcessingLease)
		s.reminders[key] = reminder
		claimed = append(claimed, reminder)
	}
	return claimed, nil
}

func (s *MemoryStore) Save(ctx context.Context, reminder Reminder) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.reminders[reminder.Key]; !exists {
		return errors.New("reminders: reminder not found")
	}
	s.reminders[reminder.Key] = reminder
	return nil
}

func dueForClaim(reminder Reminder, now time.Time) bool {
	if reminder.Status == StatusPending || reminder.Status == StatusRetry {
		return !reminder.NextAttemptAt.After(now)
	}
	if reminder.Status == StatusProcessing {
		return reminder.NextAttemptAt.IsZero() || !reminder.NextAttemptAt.After(now)
	}
	return false
}

func (s *MemoryStore) Get(key string) (Reminder, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reminder, exists := s.reminders[key]
	return reminder, exists
}
