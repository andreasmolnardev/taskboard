package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/slopstack/slopstack/apps/server/internal/reminders"
)

const (
	notificationCollection = "notifications"
	reminderCollection     = "reminders"
)

// EnsureNotificationCollections creates or repairs the persistence used by the
// notification routes and reminder scheduler. It is exported for the migration.
func EnsureNotificationCollections(app core.App) error {
	ownerRule := "@request.auth.id = owner"
	definitions := []struct {
		name    string
		fields  []core.Field
		indexes []struct {
			name, columns string
			unique        bool
		}
	}{
		{
			name: notificationCollection,
			fields: []core.Field{
				&core.TextField{Name: "owner", Required: true},
				&core.TextField{Name: "title", Required: true},
				&core.TextField{Name: "body"},
				&core.TextField{Name: "target_kind"},
				&core.TextField{Name: "target_id"},
				&core.TextField{Name: "reminder_key", Required: true},
				&core.BoolField{Name: "read"},
				&core.DateField{Name: "read_at"},
				&core.DateField{Name: "snoozed_until"},
				&core.DateField{Name: "dismissed_at"},
			},
			indexes: []struct {
				name, columns string
				unique        bool
			}{{"idx_notifications_reminder_key", "reminder_key", true}, {"idx_notifications_owner_read", "owner, read", false}},
		},
		{
			name: reminderCollection,
			fields: []core.Field{
				&core.TextField{Name: "owner", Required: true},
				&core.TextField{Name: "key", Required: true},
				&core.TextField{Name: "target_kind", Required: true},
				&core.TextField{Name: "target_id", Required: true},
				&core.DateField{Name: "at", Required: true},
				&core.NumberField{Name: "offset", OnlyInt: true},
				&core.DateField{Name: "due_at", Required: true},
				&core.TextField{Name: "channel", Required: true},
				&core.TextField{Name: "status", Required: true},
				&core.NumberField{Name: "attempts", OnlyInt: true},
				&core.DateField{Name: "next_attempt_at"},
				&core.TextField{Name: "last_error"},
				&core.DateField{Name: "delivered_at"},
			},
			indexes: []struct {
				name, columns string
				unique        bool
			}{{"idx_reminders_key", "key", true}, {"idx_reminders_due", "status, next_attempt_at", false}, {"idx_reminders_owner", "owner", false}},
		},
	}

	for _, definition := range definitions {
		collection, err := app.FindCollectionByNameOrId(definition.name)
		if err != nil {
			collection = core.NewBaseCollection(definition.name)
		}
		for _, field := range definition.fields {
			if collection.Fields.GetByName(field.GetName()) == nil {
				collection.Fields.Add(field)
			}
		}
		collection.ListRule = &ownerRule
		collection.ViewRule = &ownerRule
		collection.CreateRule = &ownerRule
		collection.UpdateRule = &ownerRule
		collection.DeleteRule = &ownerRule
		for _, index := range definition.indexes {
			if collection.GetIndex(index.name) == "" {
				collection.AddIndex(index.name, index.unique, index.columns, "")
			}
		}
		if err := app.Save(collection); err != nil {
			return fmt.Errorf("save %s collection: %w", definition.name, err)
		}
	}
	return nil
}

type notificationResponse struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Body         string `json:"body,omitempty"`
	TargetKind   string `json:"targetKind,omitempty"`
	TargetID     string `json:"targetId,omitempty"`
	Read         bool   `json:"read"`
	ReadAt       string `json:"readAt,omitempty"`
	SnoozedUntil string `json:"snoozedUntil,omitempty"`
	DismissedAt  string `json:"dismissedAt,omitempty"`
	Created      string `json:"created"`
}

// registerNotificationRoutes wires notification actions and reminder creation.
func registerNotificationRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/notifications", listNotificationsRoute)
	e.Router.PATCH("/api/notifications/{id}/read", readNotificationRoute)
	e.Router.POST("/api/notifications/{id}/snooze", snoozeNotificationRoute)
	e.Router.POST("/api/notifications/{id}/dismiss", dismissNotificationRoute)
	e.Router.POST("/api/notifications/read-all", readAllNotificationsRoute)
	e.Router.POST("/api/reminders", createReminderRoute)
}

func listNotificationsRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	now, _ := types.ParseDateTime(time.Now().UTC())
	records, err := event.App.FindRecordsByFilter(notificationCollection, "owner = {:owner} && dismissed_at = '' && (snoozed_until = '' || snoozed_until <= {:now})", "-created", 0, 0, dbx.Params{"owner": user.Id, "now": now})
	if err != nil {
		return err
	}
	items := make([]notificationResponse, 0, len(records))
	for _, record := range records {
		items = append(items, notificationFromRecord(record))
	}
	return event.JSON(http.StatusOK, items)
}

func readNotificationRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	record, err := event.App.FindRecordById(notificationCollection, event.Request.PathValue("id"))
	if err != nil || record.GetString("owner") != user.Id {
		return event.NotFoundError("notification not found", nil)
	}
	if !record.GetBool("read") {
		record.Set("read", true)
		record.Set("read_at", time.Now().UTC())
		if err := event.App.Save(record); err != nil {
			return err
		}
	}
	return event.JSON(http.StatusOK, notificationFromRecord(record))
}

type notificationActionRequest struct {
	Until string `json:"until"`
}

func notificationForUser(event *core.RequestEvent) (*core.Record, error) {
	user, err := CurrentUser(event)
	if err != nil {
		return nil, err
	}
	record, err := event.App.FindRecordById(notificationCollection, event.Request.PathValue("id"))
	if err != nil || record.GetString("owner") != user.Id {
		return nil, event.NotFoundError("notification not found", nil)
	}
	return record, nil
}

func snoozeNotificationRoute(event *core.RequestEvent) error {
	record, err := notificationForUser(event)
	if err != nil {
		return err
	}
	var input notificationActionRequest
	if err := json.NewDecoder(event.Request.Body).Decode(&input); err != nil {
		return event.BadRequestError("until must be an RFC3339 timestamp", err)
	}
	until, err := time.Parse(time.RFC3339, input.Until)
	if err != nil || !until.After(time.Now()) {
		return event.BadRequestError("until must be a future RFC3339 timestamp", nil)
	}
	record.Set("snoozed_until", until.UTC())
	record.Set("read", false)
	record.Set("read_at", "")
	if err := event.App.Save(record); err != nil {
		return err
	}
	return event.JSON(http.StatusOK, notificationFromRecord(record))
}

func dismissNotificationRoute(event *core.RequestEvent) error {
	record, err := notificationForUser(event)
	if err != nil {
		return err
	}
	record.Set("dismissed_at", time.Now().UTC())
	if err := event.App.Save(record); err != nil {
		return err
	}
	return event.JSON(http.StatusOK, notificationFromRecord(record))
}

func createReminderRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	var input struct {
		TargetKind    string `json:"targetKind"`
		TargetID      string `json:"targetId"`
		At            string `json:"at"`
		OffsetMinutes *int   `json:"offsetMinutes"`
		Channel       string `json:"channel"`
	}
	if err := json.NewDecoder(event.Request.Body).Decode(&input); err != nil {
		return event.BadRequestError("invalid reminder body", err)
	}
	collectionName, err := reminderTargetCollection(reminders.TargetKind(input.TargetKind))
	if err != nil {
		return event.BadRequestError("invalid reminder target", err)
	}
	target, err := event.App.FindRecordById(collectionName, input.TargetID)
	if err != nil || target.GetString("owner") != user.Id {
		return event.NotFoundError("reminder target not found", nil)
	}
	at, err := time.Parse(time.RFC3339, input.At)
	if err != nil {
		return event.BadRequestError("at must be an RFC3339 timestamp", err)
	}
	offset := 0
	if input.OffsetMinutes != nil {
		offset = *input.OffsetMinutes
	}
	channel := reminders.Channel(input.Channel)
	if channel == "" {
		channel = reminders.ChannelInApp
	}
	if channel != reminders.ChannelInApp {
		return event.BadRequestError(fmt.Sprintf("reminder channel %q is not supported", channel), nil)
	}
	reminder, err := reminders.New(reminders.TargetKind(input.TargetKind), input.TargetID, at, time.Duration(offset)*time.Minute, channel)
	if err != nil {
		return event.BadRequestError("invalid reminder", err)
	}
	if err := (PocketBaseReminderStore{App: event.App}).Add(event.Request.Context(), reminder); err != nil {
		if errors.Is(err, reminders.ErrDuplicate) {
			return event.JSON(http.StatusConflict, map[string]string{"error": "reminder already exists"})
		}
		return err
	}
	return event.JSON(http.StatusCreated, map[string]any{"key": reminder.Key, "dueAt": reminder.DueAt})
}

func readAllNotificationsRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	count, err := markAllNotificationsRead(event.App, user.Id, time.Now().UTC())
	if err != nil {
		return err
	}
	return event.JSON(http.StatusOK, map[string]int{"updated": count})
}

func markAllNotificationsRead(app core.App, owner string, now time.Time) (int, error) {
	records, err := app.FindRecordsByFilter(notificationCollection, "owner = {:owner} && read = false", "", 0, 0, dbx.Params{"owner": owner})
	if err != nil {
		return 0, err
	}
	for _, record := range records {
		record.Set("read", true)
		record.Set("read_at", now.UTC())
		if err := app.Save(record); err != nil {
			return 0, err
		}
	}
	return len(records), nil
}

func notificationFromRecord(record *core.Record) notificationResponse {
	response := notificationResponse{
		ID: record.Id, Title: record.GetString("title"), Body: record.GetString("body"),
		TargetKind: record.GetString("target_kind"), TargetID: record.GetString("target_id"),
		Read: record.GetBool("read"), Created: record.GetDateTime("created").String(),
	}
	if readAt := record.GetDateTime("read_at"); !readAt.IsZero() {
		response.ReadAt = readAt.String()
	}
	if snoozed := record.GetDateTime("snoozed_until"); !snoozed.IsZero() {
		response.SnoozedUntil = snoozed.String()
	}
	if dismissed := record.GetDateTime("dismissed_at"); !dismissed.IsZero() {
		response.DismissedAt = dismissed.String()
	}
	return response
}

// PocketBaseReminderStore adapts PocketBase records to the reminders domain.
type PocketBaseReminderStore struct {
	App     core.App
	Channel reminders.Channel
}

// Add persists a reminder and resolves its owner from the target record.
func (s PocketBaseReminderStore) Add(ctx context.Context, reminder reminders.Reminder) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if reminder.Channel != reminders.ChannelInApp {
		return fmt.Errorf("reminders: unsupported delivery channel %q", reminder.Channel)
	}
	collectionName, err := reminderTargetCollection(reminder.TargetKind)
	if err != nil {
		return err
	}
	target, err := s.App.FindRecordById(collectionName, reminder.TargetID)
	if err != nil {
		return fmt.Errorf("find reminder target: %w", err)
	}
	collection, err := s.App.FindCollectionByNameOrId(reminderCollection)
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	setReminderRecord(record, reminder)
	record.Set("owner", target.GetString("owner"))
	if err := s.App.Save(record); err != nil {
		matches, findErr := s.App.FindRecordsByFilter(reminderCollection, "key = {:key}", "", 1, 0, dbx.Params{"key": reminder.Key})
		if findErr == nil && len(matches) != 0 {
			return reminders.ErrDuplicate
		}
		return err
	}
	return nil
}

func (s PocketBaseReminderStore) ClaimDue(ctx context.Context, now time.Time, limit int) ([]reminders.Reminder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, errors.New("reminders: claim limit must be positive")
	}
	filter := "((status = 'pending' || status = 'retry') && next_attempt_at <= {:now}) || (status = 'processing' && (next_attempt_at = '' || next_attempt_at <= {:now}))"
	parsedNow, err := types.ParseDateTime(now.UTC())
	if err != nil {
		return nil, err
	}
	params := dbx.Params{"now": parsedNow}
	if s.Channel != "" {
		filter += " && channel = {:channel}"
		params["channel"] = string(s.Channel)
	}

	claimed := make([]reminders.Reminder, 0, limit)
	err = s.App.RunInTransaction(func(txApp core.App) error {
		records, err := txApp.FindRecordsByFilter(reminderCollection, filter, "next_attempt_at,key", limit, 0, params)
		if err != nil {
			return err
		}
		for _, record := range records {
			reminder := reminderFromRecord(record)
			reminder.Status = reminders.StatusProcessing
			reminder.NextAttemptAt = now.UTC().Add(reminders.ProcessingLease)
			setReminderRecord(record, reminder)
			if err := txApp.Save(record); err != nil {
				return err
			}
			claimed = append(claimed, reminder)
		}
		return nil
	})
	return claimed, err
}

func (s PocketBaseReminderStore) Save(ctx context.Context, reminder reminders.Reminder) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	records, err := s.App.FindRecordsByFilter(reminderCollection, "key = {:key}", "", 2, 0, dbx.Params{"key": reminder.Key})
	if err != nil {
		return err
	}
	if len(records) != 1 {
		return errors.New("reminders: reminder not found")
	}
	setReminderRecord(records[0], reminder)
	return s.App.Save(records[0])
}

func setReminderRecord(record *core.Record, reminder reminders.Reminder) {
	record.Set("key", reminder.Key)
	record.Set("target_kind", string(reminder.TargetKind))
	record.Set("target_id", reminder.TargetID)
	record.Set("at", reminder.At.UTC())
	record.Set("offset", int64(reminder.Offset))
	record.Set("due_at", reminder.DueAt.UTC())
	record.Set("channel", string(reminder.Channel))
	record.Set("status", string(reminder.Status))
	record.Set("attempts", reminder.Attempts)
	setOptionalDate(record, "next_attempt_at", reminder.NextAttemptAt)
	record.Set("last_error", reminder.LastError)
	setOptionalDate(record, "delivered_at", reminder.DeliveredAt)
}

func setOptionalDate(record *core.Record, field string, value time.Time) {
	if value.IsZero() {
		record.Set(field, "")
		return
	}
	record.Set(field, value.UTC())
}

func reminderFromRecord(record *core.Record) reminders.Reminder {
	return reminders.Reminder{
		Key: record.GetString("key"), TargetKind: reminders.TargetKind(record.GetString("target_kind")),
		TargetID: record.GetString("target_id"), At: record.GetDateTime("at").Time(),
		Offset: time.Duration(record.GetInt("offset")), DueAt: record.GetDateTime("due_at").Time(),
		Channel: reminders.Channel(record.GetString("channel")), Status: reminders.Status(record.GetString("status")),
		Attempts: record.GetInt("attempts"), NextAttemptAt: record.GetDateTime("next_attempt_at").Time(),
		LastError: record.GetString("last_error"), DeliveredAt: record.GetDateTime("delivered_at").Time(),
	}
}

type inAppReminderSender struct {
	app core.App
}

func (s inAppReminderSender) Send(ctx context.Context, reminder reminders.Reminder) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if reminder.Channel != reminders.ChannelInApp {
		return fmt.Errorf("reminders: unsupported delivery channel %q", reminder.Channel)
	}
	collectionName, err := reminderTargetCollection(reminder.TargetKind)
	if err != nil {
		return err
	}
	target, err := s.app.FindRecordById(collectionName, reminder.TargetID)
	if err != nil {
		return fmt.Errorf("find reminder target: %w", err)
	}
	return s.app.RunInTransaction(func(txApp core.App) error {
		existing, err := txApp.FindRecordsByFilter(notificationCollection, "reminder_key = {:key}", "", 1, 0, dbx.Params{"key": reminder.Key})
		if err != nil {
			return err
		}
		if len(existing) != 0 {
			return nil
		}
		collection, err := txApp.FindCollectionByNameOrId(notificationCollection)
		if err != nil {
			return err
		}
		record := core.NewRecord(collection)
		record.Set("owner", target.GetString("owner"))
		record.Set("title", target.GetString("title"))
		record.Set("body", "Reminder is due")
		record.Set("target_kind", string(reminder.TargetKind))
		record.Set("target_id", reminder.TargetID)
		record.Set("reminder_key", reminder.Key)
		return txApp.Save(record)
	})
}

func reminderTargetCollection(kind reminders.TargetKind) (string, error) {
	switch kind {
	case reminders.TargetTask:
		return "todos", nil
	case reminders.TargetEvent:
		return "events", nil
	default:
		return "", reminders.ErrInvalidReminder
	}
}

// registerReminderScheduler runs the in-app delivery worker.
func registerReminderScheduler(app core.App) {
	scheduler := reminders.Scheduler{
		Store:       PocketBaseReminderStore{App: app, Channel: reminders.ChannelInApp},
		Sender:      inAppReminderSender{app: app},
		RetryPolicy: reminders.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Minute, MaxDelay: time.Hour},
		BatchSize:   100,
	}
	app.Cron().MustAdd("in-app-reminders", "* * * * *", func() {
		if _, err := scheduler.RunDue(context.Background(), time.Now()); err != nil {
			app.Logger().Error("run in-app reminders", "error", err)
		}
	})
}

var _ reminders.Store = PocketBaseReminderStore{}
var _ reminders.Sender = inAppReminderSender{}
