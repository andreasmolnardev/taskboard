package app

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestEnsureCollectionsAddsAdminFlagToUsers(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	if err := EnsureCollections(testApp); err != nil {
		t.Fatal(err)
	}

	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.Fields.GetByName("isAdmin") == nil {
		t.Fatal("users collection must include isAdmin")
	}
}

func TestProvisionDefaultContainers(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	if err := EnsureCollections(testApp); err != nil {
		t.Fatal(err)
	}

	user, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(user)
	record.SetEmail("default@example.com")
	record.SetPassword("password123456")
	if err := testApp.Save(record); err != nil {
		t.Fatal(err)
	}

	if err := provisionDefaultContainers(testApp, record.Id); err != nil {
		t.Fatal(err)
	}
	if err := provisionDefaultContainers(testApp, record.Id); err != nil {
		t.Fatal(err)
	}

	for _, defaultContainer := range []struct {
		collection string
		name       string
	}{
		{"lists", "Tasks"},
		{"calendars", "Events and Holidays"},
	} {
		records, err := testApp.FindRecordsByFilter(defaultContainer.collection, "owner = {:owner} && name = {:name}", "", 0, 0, dbx.Params{
			"owner": record.Id,
			"name":  defaultContainer.name,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 {
			t.Fatalf("expected one default %s, got %d", defaultContainer.name, len(records))
		}
	}
}

func TestApplyBackupConfig(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	config := Config{
		BackupEnabled: true,
		BackupCron:    "0 3 * * *",
		BackupMaxKeep: 14,
	}
	if err := applyBackupConfig(testApp, config); err != nil {
		t.Fatal(err)
	}
	if testApp.Settings().Backups.Cron != config.BackupCron {
		t.Fatalf("expected backup cron %q", config.BackupCron)
	}
	if testApp.Settings().Backups.CronMaxKeep != config.BackupMaxKeep {
		t.Fatalf("expected backup retention %d", config.BackupMaxKeep)
	}
}

func TestApplyBackupConfigRejectsInvalidSettings(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	err = applyBackupConfig(testApp, Config{
		BackupEnabled: true,
		BackupCron:    "not a cron expression",
		BackupMaxKeep: 0,
	})
	if err == nil {
		t.Fatal("expected invalid backup configuration to fail")
	}
}

func TestEnsureCollectionsAddsTaskboardCollections(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	if err := EnsureCollections(testApp); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"todos", "events", "lists", "calendars", "address_books", "contacts"} {
		collection, err := testApp.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("%s collection must exist: %v", name, err)
		}
		if collection.Fields.GetByName("owner") == nil {
			t.Fatalf("%s collection must include owner", name)
		}
		if collection.ListRule == nil || collection.CreateRule == nil {
			t.Fatalf("%s collection must have user access rules", name)
		}
	}

	for collectionName, fieldName := range map[string]string{"todos": "list", "events": "calendar", "calendars": "timezone"} {
		collection, err := testApp.FindCollectionByNameOrId(collectionName)
		if err != nil {
			t.Fatal(err)
		}
		if collection.Fields.GetByName(fieldName) == nil {
			t.Fatalf("%s collection must include %s", collectionName, fieldName)
		}
	}
}
