package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func TestOfflineSyncJournalWriteRollsBackWithRecord(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	user := integrationUser(t, server.app, "sync@example.com")
	collection, err := server.app.FindCollectionByNameOrId("lists")
	if err != nil {
		t.Fatal(err)
	}
	list := core.NewRecord(collection)
	list.Set("owner", user.Id)
	list.Set("name", "Atomic list")
	list.Set("color", "#123456")
	if err := server.app.Save(list); err != nil {
		t.Fatal(err)
	}
	revision := latestRecordRevision(server.app, user.Id, "lists", list.Id)
	if revision == 0 {
		t.Fatal("record save did not append a sync journal row")
	}
	if err := server.app.RunInTransaction(func(tx core.App) error {
		record, err := tx.FindRecordById("lists", list.Id)
		if err != nil {
			return err
		}
		record.Set("name", "Rolled back")
		if err := tx.Save(record); err != nil {
			return err
		}
		return fmt.Errorf("force rollback")
	}); err == nil {
		t.Fatal("expected transaction rollback")
	}
	current, err := server.app.FindRecordById("lists", list.Id)
	if err != nil {
		t.Fatal(err)
	}
	if current.GetString("name") != "Atomic list" {
		t.Fatalf("record change escaped rollback: %q", current.GetString("name"))
	}
	if got := latestRecordRevision(server.app, user.Id, "lists", list.Id); got != revision {
		t.Fatalf("journal revision advanced during rollback: got %d want %d", got, revision)
	}
}

func TestSyncPushRejectsForgedBaseAndReplaysReceipt(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	user := integrationUser(t, server.app, "push@example.com")
	list := ownedDefault(t, server.app, "lists", user.Id, "Tasks")
	token := authToken(t, server, "push@example.com")
	revision := latestRecordRevision(server.app, user.Id, "lists", list.Id)
	body := fmt.Sprintf(`{"operations":[{"operationId":"same-id","collection":"lists","id":%q,"kind":"update","baseRevision":%d,"base":{"name":"forged"},"patch":{"description":"changed"}}]}`, list.Id, revision)
	status, _, response := integrationRequest(t, server, "POST", "/api/sync/push", body, http.Header{"Authorization": []string{"Bearer " + token}, "Content-Type": []string{"application/json"}})
	if status != 200 {
		t.Fatalf("push status %d: %s", status, response)
	}
	var decoded struct {
		Results []syncResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(response), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Results) != 1 || decoded.Results[0].Status != "error" {
		t.Fatalf("forged base accepted: %+v", decoded.Results)
	}
	status, _, replay := integrationRequest(t, server, "POST", "/api/sync/push", body, http.Header{"Authorization": []string{"Bearer " + token}, "Content-Type": []string{"application/json"}})
	if status != 200 || replay != response {
		t.Fatalf("receipt replay changed: first=%s replay=%s", response, replay)
	}
}

func TestSyncPushCRUDMergeConflictAndPullTombstone(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "sync-crud@example.com")
	other := integrationUser(t, server.app, "sync-other@example.com")
	list := ownedDefault(t, server.app, "lists", owner.Id, "Tasks")
	otherList := ownedDefault(t, server.app, "lists", other.Id, "Tasks")
	token := authToken(t, server, owner.Email())
	push := func(ops ...map[string]any) []syncResult {
		t.Helper()
		body, err := json.Marshal(map[string]any{"operations": ops})
		if err != nil {
			t.Fatal(err)
		}
		status, _, response := integrationRequest(t, server, http.MethodPost, "/api/sync/push", string(body), withContentType(bearerHeaders(token)))
		if status != http.StatusOK {
			t.Fatalf("push status %d: %s", status, response)
		}
		var decoded struct {
			Results []syncResult `json:"results"`
		}
		if err := json.Unmarshal([]byte(response), &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded.Results
	}
	foreign := push(map[string]any{"operationId": "foreign-container", "collection": "todos", "id": "foreignTodoID12", "kind": "create", "baseRevision": 0, "patch": map[string]any{"title": "Invalid", "list": otherList.Id}})
	if len(foreign) != 1 || foreign[0].Status != "error" {
		t.Fatalf("cross-owner container accepted: %+v", foreign)
	}
	create := map[string]any{"operationId": "create-1", "collection": "todos", "id": "offlinetodoid12", "kind": "create", "baseRevision": 0, "base": nil, "patch": map[string]any{"title": "First", "list": list.Id}}
	created := push(create)
	if len(created) != 1 || created[0].Status != "applied" || created[0].Revision == 0 {
		t.Fatalf("create result: %+v", created)
	}
	base, revision := created[0].Record, created[0].Revision
	independent := push(
		map[string]any{"operationId": "edit-title", "collection": "todos", "id": "offlinetodoid12", "kind": "update", "baseRevision": revision, "base": base, "patch": map[string]any{"title": "Renamed"}},
		map[string]any{"operationId": "edit-priority", "collection": "todos", "id": "offlinetodoid12", "kind": "update", "baseRevision": revision, "base": base, "patch": map[string]any{"priority": "3"}},
	)
	if len(independent) != 2 || independent[0].Status != "applied" || independent[1].Status != "applied" {
		t.Fatalf("independent edits did not merge: %+v", independent)
	}
	current, currentRevision := independent[1].Record, independent[1].Revision
	conflicts := push(
		map[string]any{"operationId": "same-a", "collection": "todos", "id": "offlinetodoid12", "kind": "update", "baseRevision": currentRevision, "base": current, "patch": map[string]any{"title": "Choice A"}},
		map[string]any{"operationId": "same-b", "collection": "todos", "id": "offlinetodoid12", "kind": "update", "baseRevision": currentRevision, "base": current, "patch": map[string]any{"title": "Choice B"}},
	)
	if conflicts[0].Status != "applied" || conflicts[1].Status != "conflict" || conflicts[1].Record == nil {
		t.Fatalf("same-field edits did not return current conflict: %+v", conflicts)
	}
	if got := push(map[string]any{"operationId": "delete-1", "collection": "todos", "id": "offlinetodoid12", "kind": "delete", "baseRevision": conflicts[0].Revision, "patch": map[string]any{}}); len(got) != 1 || got[0].Status != "applied" || !got[0].Deleted || got[0].Record != nil {
		t.Fatalf("delete result: %+v", got)
	}
	status, _, pull := integrationRequest(t, server, http.MethodGet, "/api/sync/pull?cursor=0", "", bearerHeaders(token))
	if status != http.StatusOK || !strings.Contains(pull, `"deleted":true`) {
		t.Fatalf("pull omitted tombstone: status=%d body=%s", status, pull)
	}
	otherToken := authToken(t, server, other.Email())
	status, _, otherPull := integrationRequest(t, server, http.MethodGet, "/api/sync/pull?cursor=0", "", bearerHeaders(otherToken))
	if status != http.StatusOK || strings.Contains(otherPull, "offlinetodoid12") {
		t.Fatalf("pull leaked another user's data: %s", otherPull)
	}
}

func TestSyncPullPagesLargeInitialSnapshot(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	user := integrationUser(t, server.app, "sync-pages@example.com")
	token := authToken(t, server, user.Email())
	collection, err := server.app.FindCollectionByNameOrId("lists")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 505; i++ {
		record := core.NewRecord(collection)
		record.Set("owner", user.Id)
		record.Set("name", fmt.Sprintf("Offline list %03d", i))
		if err := server.app.Save(record); err != nil {
			t.Fatal(err)
		}
	}
	status, _, first := integrationRequest(t, server, http.MethodGet, "/api/sync/pull?cursor=0", "", bearerHeaders(token))
	if status != http.StatusOK {
		t.Fatalf("first pull status %d: %s", status, first)
	}
	var page syncPullResponse
	if err := json.Unmarshal([]byte(first), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 500 {
		t.Fatalf("first page has %d changes, want 500", len(page.Changes))
	}
	status, _, second := integrationRequest(t, server, http.MethodGet, fmt.Sprintf("/api/sync/pull?cursor=%d", page.Cursor), "", bearerHeaders(token))
	if status != http.StatusOK {
		t.Fatalf("second pull status %d: %s", status, second)
	}
	if err := json.Unmarshal([]byte(second), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) < 6 {
		t.Fatalf("second page has %d changes; snapshot tail was lost", len(page.Changes))
	}
}

func TestSyncRecurrenceEditsAdvanceMasterRevision(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	user := integrationUser(t, server.app, "sync-recurrence@example.com")
	calendar := ownedDefault(t, server.app, "calendars", user.Id, "Events and Holidays")
	collection, err := server.app.FindCollectionByNameOrId("events")
	if err != nil {
		t.Fatal(err)
	}
	master := core.NewRecord(collection)
	master.Set("owner", user.Id)
	master.Set("calendar", calendar.Id)
	master.Set("title", "Daily series")
	master.Set("start_date", "2026-09-10 09:00:00.000Z")
	master.Set("end_date", "2026-09-10 10:00:00.000Z")
	master.Set("rrule", "FREQ=DAILY;COUNT=3")
	if err := server.app.Save(master); err != nil {
		t.Fatal(err)
	}
	baseRevision := latestRecordRevision(server.app, user.Id, "events", master.Id)
	token := authToken(t, server, user.Email())
	push := func(operation map[string]any) syncResult {
		t.Helper()
		body, err := json.Marshal(map[string]any{"operations": []any{operation}})
		if err != nil {
			t.Fatal(err)
		}
		status, _, response := integrationRequest(t, server, http.MethodPost, "/api/sync/push", string(body), withContentType(bearerHeaders(token)))
		if status != http.StatusOK {
			t.Fatalf("push status %d: %s", status, response)
		}
		var decoded struct {
			Results []syncResult `json:"results"`
		}
		if err := json.Unmarshal([]byte(response), &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded.Results[0]
	}
	base := map[string]any{"operationId": "series-override", "collection": "events", "id": master.Id, "kind": "override", "baseRevision": baseRevision, "patch": map[string]any{"recurrenceId": "2026-09-11T09:00:00Z", "values": map[string]any{"title": "Moved occurrence"}}}
	result := push(base)
	if result.Status != "applied" || result.Record["recurrence_parent"] != master.Id {
		t.Fatalf("override result: %+v", result)
	}
	if result.MasterRevision <= baseRevision || result.MasterRecord["id"] != master.Id {
		t.Fatalf("override response lacks the advanced master version: %+v", result)
	}
	if got := latestRecordRevision(server.app, user.Id, "events", master.Id); got <= baseRevision {
		t.Fatalf("override did not advance master revision: was %d now %d", baseRevision, got)
	}
	base["operationId"] = "series-cancel-stale"
	base["kind"] = "cancel"
	cancel := base["patch"].(map[string]any)
	cancel["recurrenceId"] = "2026-09-12T09:00:00Z"
	stale := push(base)
	if stale.Status != "conflict" || stale.Record == nil {
		t.Fatalf("stale occurrence action did not conflict with current master: %+v", stale)
	}
}

func TestSyncPushWriteFailureRollsBackMutationAndStoresReceipt(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "sync-rollback@example.com")
	list := ownedDefault(t, server.app, "lists", owner.Id, "Tasks")
	server.app.OnRecordCreate("todos").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("title") == "Force hook failure" {
			return fmt.Errorf("forced hook failure")
		}
		return e.Next()
	})
	id := "failhooktodo123"
	result, err := applySyncOperation(server.app, owner.Id, syncOperation{
		OperationID: "failed-create-receipt", Collection: "todos", ID: id, Kind: "create",
		Patch: map[string]any{"title": "Force hook failure", "list": list.Id},
	})
	if err != nil || result.Status != "error" {
		t.Fatalf("failed operation result=%+v err=%v", result, err)
	}
	if _, err := server.app.FindRecordById("todos", id); err == nil {
		t.Fatal("failed sync write left a record behind")
	}
	receipts, err := server.app.FindRecordsByFilter(syncReceipts, "owner = {:owner} && operation_id = {:operation}", "", 1, 0, dbx.Params{"owner": owner.Id, "operation": "failed-create-receipt"})
	if err != nil || len(receipts) != 1 {
		t.Fatalf("error result receipt missing: count=%d err=%v", len(receipts), err)
	}
}
