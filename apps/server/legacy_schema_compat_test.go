package server_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/archive"

	app "github.com/andreasmolnardev/taskboard/apps/server/internal/app"
	_ "github.com/andreasmolnardev/taskboard/apps/server/pb_migrations"
)

func TestHistoricalTaskboardBackupUpgradePreservesRecords(t *testing.T) {
	const historicalCutoff = "20260915000000_add_todo_calendar.go"
	testApp := newTestAppThroughMigration(t, historicalCutoff)
	defer testApp.Cleanup()

	// Rebuild the disposable app's migration history using the actual registry
	// only through this older release cutoff. EnsureCollections is intentionally
	// a live repair function (several historical callbacks call it), so remove
	// only schema additions whose migration IDs are after the cutoff.
	removeFieldForHistoricalFixture(t, testApp, "todos", "dav_data")
	removeFieldForHistoricalFixture(t, testApp, "events", "dav_data")
	if collection, err := testApp.FindCollectionByNameOrId("carddav_changes"); err == nil {
		if err := testApp.Delete(collection); err != nil {
			t.Fatalf("remove post-cutoff CardDAV journal from historical fixture: %v", err)
		}
	}
	assertFieldAbsent(t, testApp, "todos", "dav_data")
	assertFieldAbsent(t, testApp, "events", "dav_data")
	if _, err := testApp.FindCollectionByNameOrId("carddav_changes"); err == nil {
		t.Fatal("historical fixture unexpectedly has the post-cutoff CardDAV journal")
	}

	user := createLegacyUser(t, testApp, "legacy-taskboard@example.com")
	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	todos, err := testApp.FindCollectionByNameOrId("todos")
	if err != nil {
		t.Fatal(err)
	}
	events, err := testApp.FindCollectionByNameOrId("events")
	if err != nil {
		t.Fatal(err)
	}
	contacts, err := testApp.FindCollectionByNameOrId("contacts")
	if err != nil {
		t.Fatal(err)
	}

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

	// Archive and extract the old populated database without calling
	// RestoreBackup: PocketBase implements that API by restarting the process.
	const archiveName = "historical-taskboard.zip"
	if err := testApp.CreateBackup(context.Background(), archiveName); err != nil {
		t.Fatalf("create disposable historical database archive: %v", err)
	}
	backupsFS, err := testApp.NewBackupsFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := backupsFS.GetReader(archiveName)
	if err != nil {
		backupsFS.Close()
		t.Fatalf("open historical database archive: %v", err)
	}
	archiveBytes, readErr := io.ReadAll(reader)
	closeReaderErr := reader.Close()
	closeFSErr := backupsFS.Close()
	if readErr != nil {
		t.Fatalf("read historical database archive: %v", readErr)
	}
	if closeReaderErr != nil {
		t.Fatalf("close historical database archive reader: %v", closeReaderErr)
	}
	if closeFSErr != nil {
		t.Fatalf("close historical backups filesystem: %v", closeFSErr)
	}
	archivePath := filepath.Join(t.TempDir(), archiveName)
	if err := os.WriteFile(archivePath, archiveBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	upgradedDataDir := t.TempDir()
	if err := archive.Extract(archivePath, upgradedDataDir); err != nil {
		t.Fatalf("extract historical database archive: %v", err)
	}

	upgradedApp := core.NewBaseApp(core.BaseAppConfig{DataDir: upgradedDataDir})
	if err := upgradedApp.Bootstrap(); err != nil {
		t.Fatalf("bootstrap extracted historical database: %v", err)
	}
	defer func() { _ = upgradedApp.ResetBootstrapState() }()

	// Apply the migrations missing after the recorded historical cutoff and
	// the current bootstrap repairs twice to check both migration and repair
	// idempotency against a database restored from an archive.
	for range 2 {
		if err := upgradedApp.RunAppMigrations(); err != nil {
			t.Fatalf("run app migrations: %v", err)
		}
		if err := runCurrentTaskboardBootstrap(upgradedApp); err != nil {
			t.Fatalf("run current bootstrap schema repairs: %v", err)
		}
	}

	assertCollectionAndRecordSurvived(t, upgradedApp, "todos", todos.Id, todo.Id, map[string]any{
		"owner": user.Id, "title": "Legacy todo", "description": "Keep this task", "completed": false,
	})
	assertCollectionAndRecordSurvived(t, upgradedApp, "users", users.Id, user.Id, map[string]any{"email": "legacy-taskboard@example.com"})
	assertCollectionAndRecordSurvived(t, upgradedApp, "events", events.Id, event.Id, map[string]any{
		"owner": user.Id, "title": "Legacy event",
	})
	assertCollectionAndRecordSurvived(t, upgradedApp, "contacts", contacts.Id, contact.Id, map[string]any{
		"owner": user.Id, "address_book": "legacy-address-book", "uid": "legacy-contact-uid",
		"formatted_name": "Legacy Contact", "email": "legacy-contact@example.com",
	})

	assertFieldsAdded(t, upgradedApp, "todos", "calendar", "dav_href", "dav_data", "rrule")
	assertFieldsAdded(t, upgradedApp, "events", "calendar", "recurrence_parent", "recurrence_id", "dav_href", "dav_data")
	assertFieldsAdded(t, upgradedApp, "contacts", "dav_href", "vcard", "organization", "phone")
	if _, err := upgradedApp.FindCollectionByNameOrId("carddav_changes"); err != nil {
		t.Fatalf("current migration/repair did not restore CardDAV journal: %v", err)
	}
}

// TestWriteCLIHistoricalRestoreFixture recreates the checked-in, fully
// synthetic archive from the historical migration cutoff. Set the output path
// only when intentionally regenerating that fixture.
func TestWriteCLIHistoricalRestoreFixture(t *testing.T) {
	outputPath := os.Getenv("TASKBOARD_WRITE_CLI_HISTORICAL_FIXTURE")
	if outputPath == "" {
		t.Skip("set TASKBOARD_WRITE_CLI_HISTORICAL_FIXTURE to write the synthetic archive")
	}
	testApp := newTestAppThroughMigration(t, "20260915000000_add_todo_calendar.go")
	defer testApp.Cleanup()
	removeFieldForHistoricalFixture(t, testApp, "todos", "dav_data")
	removeFieldForHistoricalFixture(t, testApp, "events", "dav_data")
	if collection, err := testApp.FindCollectionByNameOrId("carddav_changes"); err == nil {
		if err := testApp.Delete(collection); err != nil {
			t.Fatalf("remove post-cutoff CardDAV journal: %v", err)
		}
	}
	assertFieldAbsent(t, testApp, "todos", "dav_data")
	assertFieldAbsent(t, testApp, "events", "dav_data")
	if _, err := testApp.FindCollectionByNameOrId("carddav_changes"); err == nil {
		t.Fatal("synthetic CLI restore fixture unexpectedly has the post-cutoff CardDAV journal")
	}

	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.Id = "histuser0000001"
	user.SetEmail("historical-taskboard@example.invalid")
	user.SetPassword("fixture-password-123456")
	if err := testApp.Save(user); err != nil {
		t.Fatalf("save synthetic historical user: %v", err)
	}
	todos, err := testApp.FindCollectionByNameOrId("todos")
	if err != nil {
		t.Fatal(err)
	}
	todo := core.NewRecord(todos)
	todo.Id = "histtodo0000001"
	todo.Set("owner", user.Id)
	todo.Set("title", "Historical task")
	todo.Set("description", "Must survive current CLI restore and migrations")
	todo.Set("completed", false)
	if err := testApp.Save(todo); err != nil {
		t.Fatalf("save synthetic historical task: %v", err)
	}

	const archiveName = "taskboard-20260915-populated.zip"
	if err := testApp.CreateBackup(context.Background(), archiveName); err != nil {
		t.Fatalf("create synthetic historical archive: %v", err)
	}
	backupsFS, err := testApp.NewBackupsFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := backupsFS.GetReader(archiveName)
	if err != nil {
		_ = backupsFS.Close()
		t.Fatalf("open synthetic historical archive: %v", err)
	}
	archiveBytes, readErr := io.ReadAll(reader)
	readerErr := reader.Close()
	fsErr := backupsFS.Close()
	if readErr != nil {
		t.Fatalf("read synthetic historical archive: %v", readErr)
	}
	if readerErr != nil {
		t.Fatalf("close synthetic historical archive reader: %v", readerErr)
	}
	if fsErr != nil {
		t.Fatalf("close synthetic historical backups filesystem: %v", fsErr)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, archiveBytes, 0o644); err != nil {
		t.Fatalf("write synthetic historical archive: %v", err)
	}
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

func newTestAppThroughMigration(t *testing.T, cutoff string) *tests.TestApp {
	t.Helper()
	var historical core.MigrationsList
	for _, migration := range core.AppMigrations.Items() {
		if migration.File <= cutoff {
			historical.Add(migration)
		}
	}
	if len(historical.Items()) == 0 {
		t.Fatalf("no registered app migrations found through historical cutoff %q", cutoff)
	}
	currentRegistry := core.AppMigrations
	core.AppMigrations = historical
	defer func() { core.AppMigrations = currentRegistry }()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("initialize disposable app through historical cutoff %s: %v", cutoff, err)
	}
	return testApp
}

func removeFieldForHistoricalFixture(t *testing.T, testApp core.App, collectionName, fieldName string) {
	t.Helper()
	collection, err := testApp.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatal(err)
	}
	if collection.Fields.GetByName(fieldName) == nil {
		return
	}
	collection.Fields.RemoveByName(fieldName)
	if err := testApp.Save(collection); err != nil {
		t.Fatalf("remove post-cutoff %s.%s from historical fixture: %v", collectionName, fieldName, err)
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

func assertFieldAbsent(t *testing.T, testApp core.App, collectionName, fieldName string) {
	t.Helper()
	collection, err := testApp.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatal(err)
	}
	if collection.Fields.GetByName(fieldName) != nil {
		t.Errorf("historical fixture unexpectedly has %s.%s", collectionName, fieldName)
	}
}
