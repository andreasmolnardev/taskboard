package server_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	app "github.com/andreasmolnardev/taskboard/apps/server/internal/app"
	_ "github.com/andreasmolnardev/taskboard/apps/server/pb_migrations"
)

func TestSyntheticLegacyTaskboardSchemaUpgradePreservesRecords(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	user := createLegacyUser(t, testApp, "legacy-taskboard@example.com")
	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	dropCollectionForLegacyFixture(t, testApp, "todos")
	dropCollectionForLegacyFixture(t, testApp, "events")
	dropCollectionForLegacyFixture(t, testApp, "contacts")
	todos := createLegacyCollection(t, testApp, "todos",
		&core.TextField{Name: "owner", Required: true},
		&core.TextField{Name: "title", Required: true},
		&core.TextField{Name: "description"},
		&core.BoolField{Name: "completed"},
	)
	events := createLegacyCollection(t, testApp, "events",
		&core.TextField{Name: "owner", Required: true},
		&core.TextField{Name: "title", Required: true},
		&core.DateField{Name: "start_date", Required: true},
		&core.DateField{Name: "end_date"},
	)
	contacts := createLegacyCollection(t, testApp, "contacts",
		&core.TextField{Name: "owner", Required: true},
		&core.TextField{Name: "address_book", Required: true},
		&core.TextField{Name: "uid", Required: true},
		&core.TextField{Name: "formatted_name", Required: true},
		&core.TextField{Name: "email"},
	)

	todo := core.NewRecord(todos)
	todo.Set("owner", user.Id)
	todo.Set("title", "Legacy todo")
	todo.Set("description", "Keep this task")
	todo.Set("completed", false)
	if err := testApp.Save(todo); err != nil {
		t.Fatal(err)
	}
	event := core.NewRecord(events)
	event.Set("owner", user.Id)
	event.Set("title", "Legacy event")
	event.Set("start_date", "2025-04-03 09:00:00.000Z")
	event.Set("end_date", "2025-04-03 10:00:00.000Z")
	if err := testApp.Save(event); err != nil {
		t.Fatal(err)
	}
	contact := core.NewRecord(contacts)
	contact.Set("owner", user.Id)
	contact.Set("address_book", "legacy-address-book")
	contact.Set("uid", "legacy-contact-uid")
	contact.Set("formatted_name", "Legacy Contact")
	contact.Set("email", "legacy-contact@example.com")
	if err := testApp.Save(contact); err != nil {
		t.Fatal(err)
	}

	// Simulate an installed older release in this disposable app: retain the
	// legacy collections and rows, then reapply the currently registered app
	// migrations and the current bootstrap repair sequence.
	clearAppMigrationHistory(t, testApp)
	for range 2 {
		if err := testApp.RunAppMigrations(); err != nil {
			t.Fatalf("run app migrations: %v", err)
		}
		if err := runCurrentTaskboardBootstrap(testApp); err != nil {
			t.Fatalf("run current bootstrap schema repairs: %v", err)
		}
	}

	assertCollectionAndRecordSurvived(t, testApp, "todos", todos.Id, todo.Id, map[string]any{
		"owner": user.Id, "title": "Legacy todo", "description": "Keep this task", "completed": false,
	})
	assertCollectionAndRecordSurvived(t, testApp, "users", users.Id, user.Id, map[string]any{"email": "legacy-taskboard@example.com"})
	assertCollectionAndRecordSurvived(t, testApp, "events", events.Id, event.Id, map[string]any{
		"owner": user.Id, "title": "Legacy event",
	})
	assertCollectionAndRecordSurvived(t, testApp, "contacts", contacts.Id, contact.Id, map[string]any{
		"owner": user.Id, "address_book": "legacy-address-book", "uid": "legacy-contact-uid",
		"formatted_name": "Legacy Contact", "email": "legacy-contact@example.com",
	})

	assertFieldsAdded(t, testApp, "todos", "calendar", "dav_href", "dav_data", "rrule")
	assertFieldsAdded(t, testApp, "events", "calendar", "recurrence_parent", "recurrence_id", "dav_href", "dav_data")
	assertFieldsAdded(t, testApp, "contacts", "dav_href", "vcard", "organization", "phone")
}

func runCurrentTaskboardBootstrap(testApp core.App) error {
	if err := app.EnsureCollections(testApp); err != nil {
		return err
	}
	if err := app.EnsureNotificationCollections(testApp); err != nil {
		return err
	}
	if err := app.EnsureCalDAVChanges(testApp); err != nil {
		return err
	}
	return app.EnsureCardDAVChanges(testApp)
}

func createLegacyUser(t *testing.T, testApp core.App, email string) *core.Record {
	t.Helper()
	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("legacy-user-password-123")
	if err := testApp.Save(user); err != nil {
		t.Fatal(err)
	}
	return user
}

func createLegacyCollection(t *testing.T, testApp core.App, name string, fields ...core.Field) *core.Collection {
	t.Helper()
	collection := core.NewBaseCollection(name)
	for _, field := range fields {
		collection.Fields.Add(field)
	}
	if err := testApp.Save(collection); err != nil {
		t.Fatalf("create legacy %s collection: %v", name, err)
	}
	return collection
}

func dropCollectionForLegacyFixture(t *testing.T, testApp core.App, name string) {
	t.Helper()
	collection, err := testApp.FindCollectionByNameOrId(name)
	if err != nil {
		return
	}
	if err := testApp.Delete(collection); err != nil {
		t.Fatalf("drop fresh %s collection in disposable fixture: %v", name, err)
	}
}

func clearAppMigrationHistory(t *testing.T, testApp core.App) {
	t.Helper()
	for _, migration := range core.AppMigrations.Items() {
		if _, err := testApp.DB().Delete(core.DefaultMigrationsTable, dbx.HashExp{"file": migration.File}).Execute(); err != nil {
			t.Fatalf("clear disposable migration history for %s: %v", migration.File, err)
		}
	}
}

func assertCollectionAndRecordSurvived(t *testing.T, testApp core.App, name, collectionID, recordID string, values map[string]any) {
	t.Helper()
	collection, err := testApp.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	if collection.Id != collectionID {
		t.Errorf("%s collection ID changed: got %q, want %q", name, collection.Id, collectionID)
	}
	record, err := testApp.FindRecordById(name, recordID)
	if err != nil {
		t.Fatalf("find preserved %s record: %v", name, err)
	}
	if record.Id != recordID {
		t.Errorf("%s record ID changed: got %q, want %q", name, record.Id, recordID)
	}
	for field, want := range values {
		if got := record.Get(field); got != want {
			t.Errorf("%s.%s = %#v, want %#v", name, field, got, want)
		}
	}
}

func assertFieldsAdded(t *testing.T, testApp core.App, collectionName string, fields ...string) {
	t.Helper()
	collection, err := testApp.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range fields {
		if collection.Fields.GetByName(field) == nil {
			t.Errorf("upgrade did not add %s.%s", collectionName, field)
		}
	}
}
