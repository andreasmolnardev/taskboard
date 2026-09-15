package reminders

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewCalculatesDueFromAbsoluteInstantAcrossDST(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 8, 9, 0, 0, 0, zone)

	reminder, err := New(TargetEvent, "event-1", at, -24*time.Hour, ChannelInApp)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 7, 8, 0, 0, 0, zone)
	if !reminder.DueAt.Equal(want) {
		t.Fatalf("due = %v, want %v", reminder.DueAt, want)
	}
	if reminder.At.Location() != time.UTC || reminder.DueAt.Location() != time.UTC {
		t.Fatalf("times are not canonical UTC: at=%v due=%v", reminder.At, reminder.DueAt)
	}
}

func TestDeduplicationKeyUsesInstantAndScheduleIdentity(t *testing.T) {
	zone := time.FixedZone("other", -5*60*60)
	instant := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	first, _ := New(TargetTask, "task-1", instant, -time.Hour, ChannelInApp)
	second, _ := New(TargetTask, "task-1", instant.In(zone), -time.Hour, ChannelInApp)
	if first.Key != second.Key {
		t.Fatal("same instant produced different keys")
	}
	other, _ := New(TargetTask, "task-1", instant, -2*time.Hour, ChannelEmail)
	if first.Key == other.Key {
		t.Fatal("different offsets produced the same key")
	}
}

func TestNewRejectsInvalidValues(t *testing.T) {
	if _, err := New("note", "id", time.Now(), 0, ChannelPush); !errors.Is(err, ErrInvalidReminder) {
		t.Fatalf("error = %v", err)
	}
	if _, err := New(TargetTask, "", time.Now(), 0, ChannelPush); !errors.Is(err, ErrInvalidReminder) {
		t.Fatalf("error = %v", err)
	}
	if _, err := New(TargetTask, "id", time.Time{}, 0, "sms"); !errors.Is(err, ErrInvalidReminder) {
		t.Fatalf("error = %v", err)
	}
	for _, channel := range []Channel{ChannelEmail, ChannelPush} {
		if _, err := New(TargetTask, "id", time.Now(), 0, channel); !errors.Is(err, ErrInvalidReminder) {
			t.Fatalf("channel %q was accepted: %v", channel, err)
		}
	}
}

func TestRetryPolicyBackoffIsExponentialAndBounded(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 5, BaseDelay: time.Minute, MaxDelay: 3 * time.Minute}
	for attempt, want := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute, 3 * time.Minute} {
		if got := policy.Backoff(attempt + 1); got != want {
			t.Errorf("attempt %d delay = %v, want %v", attempt+1, got, want)
		}
	}
}

func TestSchedulerRetriesThenDelivers(t *testing.T) {
	ctx := context.Background()
	now := mustTime("2026-09-13T10:00:00Z")
	reminder, _ := New(TargetTask, "task-1", now.Add(time.Hour), -time.Hour, ChannelInApp)
	store := NewMemoryStore()
	if err := store.Add(ctx, reminder); err != nil {
		t.Fatal(err)
	}
	sender := &sequenceSender{results: []error{errors.New("offline"), nil}}
	scheduler := Scheduler{
		Store: store, Sender: sender, BatchSize: 10,
		RetryPolicy: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Minute, MaxDelay: time.Hour},
	}

	processed, err := scheduler.RunDue(ctx, now)
	if err != nil || processed != 1 {
		t.Fatalf("first run = %d, %v", processed, err)
	}
	got, _ := store.Get(reminder.Key)
	if got.Status != StatusRetry || got.Attempts != 1 || !got.NextAttemptAt.Equal(now.Add(time.Minute)) || got.LastError != "offline" {
		t.Fatalf("after failure = %#v", got)
	}
	if processed, err = scheduler.RunDue(ctx, now.Add(30*time.Second)); err != nil || processed != 0 {
		t.Fatalf("early run = %d, %v", processed, err)
	}
	if processed, err = scheduler.RunDue(ctx, now.Add(time.Minute)); err != nil || processed != 1 {
		t.Fatalf("retry run = %d, %v", processed, err)
	}
	got, _ = store.Get(reminder.Key)
	if got.Status != StatusDelivered || got.Attempts != 2 || !got.DeliveredAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("after delivery = %#v", got)
	}
}

func TestSchedulerStopsAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	now := mustTime("2026-09-13T10:00:00Z")
	reminder, _ := New(TargetEvent, "event-1", now, 0, ChannelInApp)
	store := NewMemoryStore()
	if err := store.Add(ctx, reminder); err != nil {
		t.Fatal(err)
	}
	scheduler := Scheduler{
		Store: store, Sender: &sequenceSender{fallback: errors.New("no sender")}, BatchSize: 1,
		RetryPolicy: RetryPolicy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: time.Second},
	}
	if _, err := scheduler.RunDue(ctx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.RunDue(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(reminder.Key)
	if got.Status != StatusFailed || got.Attempts != 2 || !got.NextAttemptAt.IsZero() {
		t.Fatalf("final state = %#v", got)
	}
}

func TestMemoryStoreReclaimsExpiredProcessingReminder(t *testing.T) {
	ctx := context.Background()
	now := mustTime("2026-09-13T10:00:00Z")
	reminder, _ := New(TargetTask, "task-1", now, 0, ChannelInApp)
	reminder.Status = StatusProcessing
	reminder.NextAttemptAt = now.Add(-time.Second)
	store := NewMemoryStore()
	if err := store.Add(ctx, reminder); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDue(ctx, now, 1)
	if err != nil || len(claimed) != 1 || claimed[0].Status != StatusProcessing {
		t.Fatalf("recovered claim = %#v, %v", claimed, err)
	}
	if !claimed[0].NextAttemptAt.Equal(now.Add(ProcessingLease)) {
		t.Fatalf("lease = %v, want %v", claimed[0].NextAttemptAt, now.Add(ProcessingLease))
	}
}

func TestMemoryStoreDeduplicatesAndClaimsOnce(t *testing.T) {
	ctx := context.Background()
	now := mustTime("2026-09-13T10:00:00Z")
	reminder, _ := New(TargetTask, "task-1", now, 0, ChannelInApp)
	store := NewMemoryStore()
	if err := store.Add(ctx, reminder); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ctx, reminder); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v", err)
	}
	first, err := store.ClaimDue(ctx, now, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim = %#v, %v", first, err)
	}
	second, err := store.ClaimDue(ctx, now, 1)
	if err != nil || len(second) != 0 {
		t.Fatalf("second claim = %#v, %v", second, err)
	}
}

type sequenceSender struct {
	results  []error
	fallback error
}

func (s *sequenceSender) Send(_ context.Context, _ Reminder) error {
	if len(s.results) == 0 {
		return s.fallback
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result
}

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
