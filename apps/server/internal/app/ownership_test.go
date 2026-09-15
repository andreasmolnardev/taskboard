package app

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestValidateEntryContainerEnforcesOwnerKindAndArchive(t *testing.T) {
	testApp, owner, other := newNotificationTestApp(t)
	defer testApp.Cleanup()
	list := addOwnedContainer(t, testApp, "lists", owner.Id, "Tasks")
	foreign := addOwnedContainer(t, testApp, "lists", other.Id, "Other")
	calendar := addOwnedContainer(t, testApp, "calendars", owner.Id, "Events")

	if err := validateEntryContainer(testApp, "todos", list.Id, owner.Id); err != nil {
		t.Fatal(err)
	}
	if err := validateEntryContainer(testApp, "todos", foreign.Id, owner.Id); err == nil {
		t.Fatal("foreign list was accepted")
	}
	if err := validateEntryContainer(testApp, "todos", calendar.Id, owner.Id); err == nil {
		t.Fatal("calendar was accepted for a task")
	}
	list.Set("archived", true)
	if err := testApp.Save(list); err != nil {
		t.Fatal(err)
	}
	if err := validateEntryContainer(testApp, "todos", list.Id, owner.Id); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("archived list error = %v", err)
	}
}

func addOwnedContainer(t *testing.T, app core.App, collectionName, owner, name string) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(collection)
	record.Set("owner", owner)
	record.Set("name", name)
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
	return record
}
