package reminders

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Store owns persistence and atomic claiming. ClaimDue must prevent the same
// reminder from being returned to concurrent scheduler runs.
type Store interface {
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]Reminder, error)
	Save(ctx context.Context, reminder Reminder) error
}

// Sender delivers one reminder through its selected channel.
type Sender interface {
	Send(ctx context.Context, reminder Reminder) error
}

type Scheduler struct {
	Store       Store
	Sender      Sender
	RetryPolicy RetryPolicy
	BatchSize   int
}

// RunDue claims one bounded batch and records each delivery result. A delivery
// failure updates reminder state but does not stop other reminders in the batch.
func (s Scheduler) RunDue(ctx context.Context, now time.Time) (int, error) {
	if s.Store == nil || s.Sender == nil || s.BatchSize < 1 {
		return 0, errors.New("reminders: invalid scheduler")
	}
	if err := s.RetryPolicy.Validate(); err != nil {
		return 0, err
	}

	claimed, err := s.Store.ClaimDue(ctx, now.UTC(), s.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("reminders: claim due: %w", err)
	}

	processed := 0
	for _, reminder := range claimed {
		if err := ctx.Err(); err != nil {
			return processed, err
		}

		deliveryErr := s.Sender.Send(ctx, reminder)
		if deliveryErr == nil {
			reminder = reminder.delivered(now)
		} else {
			reminder = reminder.deliveryFailed(now, deliveryErr, s.RetryPolicy)
		}
		if err := s.Store.Save(ctx, reminder); err != nil {
			return processed, fmt.Errorf("reminders: save %s: %w", reminder.Key, err)
		}
		processed++
	}
	return processed, nil
}
