package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/andreasmolnardev/taskboard/apps/server/internal/reminders"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func TestReminderListAndDeleteAreOwnerScoped(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "reminder-owner@example.test")
	other := integrationUser(t, server.app, "reminder-other@example.test")
	list := integrationRecord(t, server.app, "lists", map[string]any{"owner": owner.Id, "name": "Tasks"})
	todo := integrationRecord(t, server.app, "todos", map[string]any{"owner": owner.Id, "list": list.Id, "title": "Reminder target"})
	foreignList := integrationRecord(t, server.app, "lists", map[string]any{"owner": other.Id, "name": "Other tasks"})
	foreignTodo := integrationRecord(t, server.app, "todos", map[string]any{"owner": other.Id, "list": foreignList.Id, "title": "Other target"})
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	first, err := reminders.New(reminders.TargetTask, todo.Id, at, -15*time.Minute, reminders.ChannelInApp)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reminders.New(reminders.TargetTask, foreignTodo.Id, at, -30*time.Minute, reminders.ChannelInApp)
	if err != nil {
		t.Fatal(err)
	}
	store := PocketBaseReminderStore{App: server.app}
	if err := store.Add(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	firstRecord, err := server.app.FindFirstRecordByFilter(reminderCollection, "key={:key}", dbx.Params{"key": first.Key})
	if err != nil {
		t.Fatal(err)
	}
	ownerToken := authToken(t, server, owner.GetString("email"))
	otherToken := authToken(t, server, other.GetString("email"))
	path := fmt.Sprintf("/api/reminders?targetKind=task&targetId=%s", todo.Id)
	status, _, body := integrationRequest(t, server, http.MethodGet, path, "", bearerHeaders(ownerToken))
	if status != http.StatusOK || !strings.Contains(body, first.Key) {
		t.Fatalf("reminder list status=%d body=%s", status, body)
	}
	status, _, _ = integrationRequest(t, server, http.MethodDelete, "/api/reminders/"+firstRecord.Id, "", bearerHeaders(otherToken))
	if status != http.StatusNotFound {
		t.Fatalf("cross-user delete status=%d", status)
	}
	status, _, _ = integrationRequest(t, server, http.MethodDelete, "/api/reminders/"+firstRecord.Id, "", bearerHeaders(ownerToken))
	if status != http.StatusNoContent {
		t.Fatalf("owner delete status=%d", status)
	}
}

func integrationRecord(t *testing.T, app core.App, collectionName string, values map[string]any) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(collection)
	for key, value := range values {
		record.Set(key, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
	return record
}
