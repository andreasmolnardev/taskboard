// Package reminders contains reminder scheduling rules without transport or database concerns.
package reminders

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type TargetKind string

const (
	TargetEvent TargetKind = "event"
	TargetTask  TargetKind = "task"
)

type Channel string

const (
	ChannelInApp Channel = "in-app"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"

	// ProcessingLease bounds how long a claimed reminder may remain stuck after
	// a worker exits before saving its result.
	ProcessingLease = 5 * time.Minute
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusRetry      Status = "retry"
	StatusDelivered  Status = "delivered"
	StatusFailed     Status = "failed"
)

var ErrInvalidReminder = errors.New("reminders: invalid reminder")

// Reminder is one delivery for one absolute event or task instant. Offset is
// relative to At; negative values schedule before it and positive values after it.
type Reminder struct {
	Key           string
	TargetKind    TargetKind
	TargetID      string
	At            time.Time
	Offset        time.Duration
	DueAt         time.Time
	Channel       Channel
	Status        Status
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	DeliveredAt   time.Time
}

// New builds a reminder from an already resolved instant. Absolute arithmetic
// makes the result independent of process timezone and daylight-saving changes.
func New(kind TargetKind, targetID string, at time.Time, offset time.Duration, channel Channel) (Reminder, error) {
	if (kind != TargetEvent && kind != TargetTask) || targetID == "" || at.IsZero() || !validChannel(channel) {
		return Reminder{}, ErrInvalidReminder
	}

	at = at.UTC()
	due := at.Add(offset)
	return Reminder{
		Key:           DeduplicationKey(kind, targetID, at, offset, channel),
		TargetKind:    kind,
		TargetID:      targetID,
		At:            at,
		Offset:        offset,
		DueAt:         due,
		Channel:       channel,
		Status:        StatusPending,
		NextAttemptAt: due,
	}, nil
}

// DeduplicationKey is stable across locations that represent the same instant.
func DeduplicationKey(kind TargetKind, targetID string, at time.Time, offset time.Duration, channel Channel) string {
	value := string(kind) + "\x00" + targetID + "\x00" +
		strconv.FormatInt(at.UTC().UnixNano(), 10) + "\x00" +
		strconv.FormatInt(int64(offset), 10) + "\x00" + string(channel)
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validChannel(channel Channel) bool {
	// Only in-app delivery has a registered sender. Do not persist reminders
	// for transports that the scheduler cannot deliver.
	return channel == ChannelInApp
}

// RetryPolicy defines bounded exponential retry delays.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 1 || p.BaseDelay <= 0 || p.MaxDelay < p.BaseDelay {
		return fmt.Errorf("reminders: invalid retry policy")
	}
	return nil
}

// Backoff returns the delay after the given failed attempt, starting at one.
func (p RetryPolicy) Backoff(attempt int) time.Duration {
	if attempt <= 1 {
		return p.BaseDelay
	}
	delay := p.BaseDelay
	for i := 1; i < attempt; i++ {
		if delay >= p.MaxDelay/2 {
			return p.MaxDelay
		}
		delay *= 2
	}
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

func (r Reminder) delivered(now time.Time) Reminder {
	r.Attempts++
	r.Status = StatusDelivered
	r.DeliveredAt = now.UTC()
	r.NextAttemptAt = time.Time{}
	r.LastError = ""
	return r
}

func (r Reminder) deliveryFailed(now time.Time, deliveryErr error, policy RetryPolicy) Reminder {
	r.Attempts++
	r.LastError = deliveryErr.Error()
	if r.Attempts >= policy.MaxAttempts {
		r.Status = StatusFailed
		r.NextAttemptAt = time.Time{}
		return r
	}
	r.Status = StatusRetry
	r.NextAttemptAt = now.UTC().Add(policy.Backoff(r.Attempts))
	return r
}
