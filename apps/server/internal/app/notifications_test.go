package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/slopstack/slopstack/apps/server/internal/reminders"
)

func TestEnsureNotificationCollectionsIsSafeAndSetsOwnerRules(t *testing.T) {
	testApp, _, _ := newNotificationTestApp(t)
	defer testApp.Cleanup()

	if err := EnsureNotificationCollections(testApp); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{notificationCollection, reminderCollection} {
		collection, err := testApp.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		if name == notificationCollection {
			for _, field := range []string{"snoozed_until", "dismissed_at"} {
				if collection.Fields.GetByName(field) == nil {
					t.Fatalf("notification must include %s", field)
				}
			}
		}
		want := "@request.auth.id = owner"
		for ruleName, rule := range map[string]*string{
			"list": collection.ListRule, "view": collection.ViewRule, "create": collection.CreateRule,
			"update": collection.UpdateRule, "delete": collection.DeleteRule,
		} {
			if rule == nil || *rule != want {
				t.Errorf("%s %s rule = %v", name, ruleName, rule)
			}
		}
	}
}

func TestPocketBaseReminderStoreDeduplicatesAndClaimsOnce(t *testing.T) {
	testApp, owner, _ := newNotificationTestApp(t)
	defer testApp.Cleanup()
	todo := addNotificationTestRecord(t, testApp, "todos", map[string]any{"owner": owner.Id, "title": "Pay rent"})
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	reminder, err := reminders.New(reminders.TargetTask, todo.Id, now, 0, reminders.ChannelInApp)
	if err != nil {
		t.Fatal(err)
	}
	store := PocketBaseReminderStore{App: testApp}
	if err := store.Add(context.Background(), reminder); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(context.Background(), reminder); !errors.Is(err, reminders.ErrDuplicate) {
		t.Fatalf("duplicate error = %v", err)
	}

	claimed, err := store.ClaimDue(context.Background(), now, 10)
	if err != nil || len(claimed) != 1 || claimed[0].Key != reminder.Key {
		t.Fatalf("first claim = %#v, %v", claimed, err)
	}
	claimed, err = store.ClaimDue(context.Background(), now, 10)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("second claim = %#v, %v", claimed, err)
	}
	records, err := testApp.FindRecordsByFilter(reminderCollection, "key = {:key}", "", 1, 0, dbx.Params{"key": reminder.Key})
	if err != nil || len(records) != 1 || records[0].GetString("owner") != owner.Id {
		t.Fatalf("persisted reminder = %#v, %v", records, err)
	}
}

func TestInAppReminderSenderDeduplicatesNotifications(t *testing.T) {
	testApp, owner, _ := newNotificationTestApp(t)
	defer testApp.Cleanup()
	event := addNotificationTestRecord(t, testApp, "events", map[string]any{
		"owner": owner.Id, "title": "Standup", "start_date": "2026-09-13 10:00:00.000Z",
	})
	reminder, _ := reminders.New(reminders.TargetEvent, event.Id, time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC), 0, reminders.ChannelInApp)
	sender := inAppReminderSender{app: testApp}
	for range 2 {
		if err := sender.Send(context.Background(), reminder); err != nil {
			t.Fatal(err)
		}
	}
	records, err := testApp.FindRecordsByFilter(notificationCollection, "reminder_key = {:key}", "", 0, 0, dbx.Params{"key": reminder.Key})
	if err != nil || len(records) != 1 {
		t.Fatalf("notifications = %#v, %v", records, err)
	}
	if records[0].GetString("owner") != owner.Id || records[0].GetString("title") != "Standup" || records[0].GetBool("read") {
		t.Fatalf("notification = %#v", records[0])
	}
}

func TestInAppReminderSenderRejectsOtherChannels(t *testing.T) {
	testApp, owner, _ := newNotificationTestApp(t)
	defer testApp.Cleanup()
	todo := addNotificationTestRecord(t, testApp, "todos", map[string]any{"owner": owner.Id, "title": "No fake mail"})
	reminder, _ := reminders.New(reminders.TargetTask, todo.Id, time.Now(), 0, reminders.ChannelEmail)
	if err := (inAppReminderSender{app: testApp}).Send(context.Background(), reminder); err == nil {
		t.Fatal("email reminder was accepted by in-app sender")
	}
}

func TestMarkAllNotificationsReadIsolatesOwner(t *testing.T) {
	testApp, owner, other := newNotificationTestApp(t)
	defer testApp.Cleanup()
	first := addNotification(t, testApp, owner.Id, "first")
	second := addNotification(t, testApp, owner.Id, "second")
	foreign := addNotification(t, testApp, other.Id, "foreign")
	now := time.Date(2026, 9, 13, 11, 0, 0, 0, time.UTC)

	count, err := markAllNotificationsRead(testApp, owner.Id, now)
	if err != nil || count != 2 {
		t.Fatalf("mark all = %d, %v", count, err)
	}
	for _, record := range []*core.Record{first, second} {
		fresh, err := testApp.FindRecordById(notificationCollection, record.Id)
		if err != nil || !fresh.GetBool("read") || !fresh.GetDateTime("read_at").Time().Equal(now) {
			t.Fatalf("owned notification not read: %#v, %v", fresh, err)
		}
	}
	freshForeign, err := testApp.FindRecordById(notificationCollection, foreign.Id)
	if err != nil || freshForeign.GetBool("read") {
		t.Fatalf("foreign notification changed: %#v, %v", freshForeign, err)
	}
}

func newNotificationTestApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
	t.Helper()
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureCollections(testApp); err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	if err := EnsureNotificationCollections(testApp); err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	return testApp, newUser(t, testApp, "notifications@example.com"), newUser(t, testApp, "other-notifications@example.com")
}

func addNotification(t *testing.T, app core.App, owner, key string) *core.Record {
	t.Helper()
	return addNotificationTestRecord(t, app, notificationCollection, map[string]any{
		"owner": owner, "title": key, "reminder_key": key,
	})
}

func addNotificationTestRecord(t *testing.T, app core.App, collectionName string, fields map[string]any) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(collection)
	for name, value := range fields {
		record.Set(name, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
	return record
}
