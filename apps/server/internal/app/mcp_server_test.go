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
	if !ok || len(result["tools"].([]map[string]any)) != 4 {
		t.Fatalf("unexpected tools/list result: %#v", response.Result)
	}
	response = dispatchMCPRequest(nil, "user", mcpRequest{JSONRPC: "2.0", ID: []byte("2"), Method: "unknown"})
	if response.Error == nil || response.Error.Code != -32601 {
		t.Fatalf("expected method-not-found error, got %#v", response)
	}
}
