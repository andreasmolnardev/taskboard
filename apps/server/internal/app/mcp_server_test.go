package app

import (
	"testing"
)

func TestMCPTaskToolsEnforceRecordOwnership(t *testing.T) {
	testApp, owner, other := newSearchTestApp(t)
	defer testApp.Cleanup()

	list := addSearchRecord(t, testApp, "lists", map[string]any{"owner": owner.Id, "name": "Work"})
	otherList := addSearchRecord(t, testApp, "lists", map[string]any{"owner": other.Id, "name": "Private"})
	ownTask := addSearchRecord(t, testApp, "todos", map[string]any{"owner": owner.Id, "list": list.Id, "title": "Mine"})
	foreignTask := addSearchRecord(t, testApp, "todos", map[string]any{"owner": other.Id, "list": otherList.Id, "title": "Theirs"})

	listed, err := listMCPTasks(testApp, owner.Id, map[string]any{})
	if err != nil || len(listed) != 1 || listed[0]["id"] != ownTask.Id {
		t.Fatalf("list_tasks returned wrong tasks: %#v, %v", listed, err)
	}
	if _, err := getMCPTask(testApp, owner.Id, map[string]any{"id": foreignTask.Id}); err == nil {
		t.Fatal("get_task exposed another user's task")
	}
	if _, err := updateMCPTask(testApp, owner.Id, map[string]any{"id": foreignTask.Id, "title": "changed"}); err == nil {
		t.Fatal("update_task changed another user's task")
	}
	if _, err := deleteMCPTask(testApp, owner.Id, map[string]any{"id": foreignTask.Id, "confirm": true}); err == nil {
		t.Fatal("delete_task deleted another user's task")
	}

	created, err := createMCPTask(testApp, owner.Id, map[string]any{"title": "New", "list": list.Id})
	if err != nil || created["title"] != "New" {
		t.Fatalf("create_task failed: %#v, %v", created, err)
	}
	if _, err := createMCPTask(testApp, owner.Id, map[string]any{"title": "Cross-list", "list": otherList.Id}); err == nil {
		t.Fatal("create_task accepted another user's list")
	}
}

func TestMCPDispatchListsToolsAndReturnsToolErrors(t *testing.T) {
	response := dispatchMCPRequest(nil, "user", mcpRequest{JSONRPC: "2.0", ID: []byte("1"), Method: "tools/list"})
	if response.Error != nil {
		t.Fatalf("tools/list returned error: %#v", response.Error)
	}
	result, ok := response.Result.(map[string]any)
	if !ok || len(result["tools"].([]map[string]any)) != 20 {
		t.Fatalf("unexpected tools/list result: %#v", response.Result)
	}
	response = dispatchMCPRequest(nil, "user", mcpRequest{JSONRPC: "2.0", ID: []byte("2"), Method: "unknown"})
	if response.Error == nil || response.Error.Code != -32601 {
		t.Fatalf("expected method-not-found error, got %#v", response)
	}
}

func TestMCPProjectsAndTaskFiltersRespectOwnership(t *testing.T) {
	testApp, owner, other := newSearchTestApp(t)
	defer testApp.Cleanup()
	project, err := createMCPProject(testApp, owner.Id, map[string]any{"name": "Roadmap", "description": "Ship it", "home_url": "https://example.test/repo", "target_date": "2027-01-15"})
	if err != nil {
		t.Fatal(err)
	}
	projectID := project["id"].(string)
	list, err := testApp.FindRecordById("lists", projectID)
	if err != nil {
		t.Fatal(err)
	}
	backlog, err := createMCPTask(testApp, owner.Id, map[string]any{"title": "Backlog", "list": list.Id, "status": "IN-PROCESS"})
	if err != nil {
		t.Fatal(err)
	}
	if backlog["due_date"] != "" || backlog["status"] != "IN-PROCESS" {
		t.Fatalf("task = %#v", backlog)
	}
	filtered, err := listMCPTasks(testApp, owner.Id, map[string]any{"undated": true, "status": "IN-PROCESS", "list": list.Id})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered backlog = %#v, err=%v", filtered, err)
	}
	if _, err := getMCPProject(testApp, other.Id, map[string]any{"id": projectID}); err == nil {
		t.Fatal("project was exposed to another owner")
	}
	updated, err := updateMCPProject(testApp, owner.Id, map[string]any{"id": projectID, "status": "paused"})
	if err != nil || updated["status"] != "paused" || updated["progress_percent"] != 0 {
		t.Fatalf("project update = %#v, err=%v", updated, err)
	}
}

func TestMCPEventAndContactToolsEnforceOwnershipAndConfirmDeletes(t *testing.T) {
	testApp, owner, other := newSearchTestApp(t)
	defer testApp.Cleanup()
	calendar := addSearchRecord(t, testApp, "calendars", map[string]any{"owner": owner.Id, "name": "Personal"})
	foreignCalendar := addSearchRecord(t, testApp, "calendars", map[string]any{"owner": other.Id, "name": "Private"})
	event, err := createMCPEvent(testApp, owner.Id, map[string]any{"title": "Planning", "calendar": calendar.Id, "start_date": "2027-02-01T10:00:00Z", "end_date": "2027-02-01T11:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createMCPEvent(testApp, owner.Id, map[string]any{"title": "Cross calendar", "calendar": foreignCalendar.Id, "start_date": "2027-02-01T10:00:00Z"}); err == nil {
		t.Fatal("event accepted another user's calendar")
	}
	listed, err := listMCPEvents(testApp, owner.Id, map[string]any{"calendar": calendar.Id, "search": "Planning"})
	if err != nil || len(listed) != 1 {
		t.Fatalf("events = %#v, err=%v", listed, err)
	}
	if _, err := deleteMCPEvent(testApp, owner.Id, map[string]any{"id": event["id"], "confirm": true}); err != nil {
		t.Fatal(err)
	}

	book := addSearchRecord(t, testApp, "address_books", map[string]any{"owner": owner.Id, "name": "People"})
	foreignBook := addSearchRecord(t, testApp, "address_books", map[string]any{"owner": other.Id, "name": "Private people"})
	contact, err := createMCPContact(testApp, owner.Id, map[string]any{"address_book": book.Id, "formatted_name": "Ada Lovelace", "email": "ada@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createMCPContact(testApp, owner.Id, map[string]any{"address_book": foreignBook.Id, "formatted_name": "Private"}); err == nil {
		t.Fatal("contact accepted another user's address book")
	}
	contacts, err := listMCPContacts(testApp, owner.Id, map[string]any{"search": "Ada"})
	if err != nil || len(contacts) != 1 {
		t.Fatalf("contacts = %#v, err=%v", contacts, err)
	}
	if _, err := deleteMCPContact(testApp, owner.Id, map[string]any{"id": contact["id"]}); err == nil {
		t.Fatal("contact delete bypassed confirmation")
	}
	if _, err := deleteMCPContact(testApp, owner.Id, map[string]any{"id": contact["id"], "confirm": true}); err != nil {
		t.Fatal(err)
	}
}
