package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestAccountExportContainsOnlyCurrentUsersData(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "export-owner@example.com")
	other := integrationUser(t, server.app, "export-other@example.com")
	list := ownedDefault(t, server.app, "lists", owner.Id, "Tasks")
	createTodo := func(user *core.Record, listID, title string) {
		collection, err := server.app.FindCollectionByNameOrId("todos")
		if err != nil {
			t.Fatal(err)
		}
		record := core.NewRecord(collection)
		record.Set("owner", user.Id)
		record.Set("list", listID)
		record.Set("title", title)
		if err := server.app.Save(record); err != nil {
			t.Fatal(err)
		}
	}
	createTodo(owner, list.Id, "Private task")
	otherList := ownedDefault(t, server.app, "lists", other.Id, "Tasks")
	createTodo(other, otherList.Id, "Other user's task")
	token := authToken(t, server, owner.Email())

	status, headers, body := integrationRequest(t, server, http.MethodGet, "/api/account/export", "", bearerHeaders(token))
	if status != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", status, body)
	}
	if headers.Get("Content-Disposition") == "" || headers.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("export headers = %v", headers)
	}
	if !strings.Contains(body, "Private task") || !strings.Contains(body, owner.Email()) || strings.Contains(body, "Other user's task") || strings.Contains(body, other.Email()) {
		t.Fatalf("export did not isolate the current user's data: %s", body)
	}
}

func TestDeleteAccountRequiresEmailConfirmationAndRemovesOwnedData(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "delete-owner@example.com")
	other := integrationUser(t, server.app, "delete-other@example.com")
	list := ownedDefault(t, server.app, "lists", owner.Id, "Tasks")
	todoCollection, err := server.app.FindCollectionByNameOrId("todos")
	if err != nil {
		t.Fatal(err)
	}
	todo := core.NewRecord(todoCollection)
	todo.Set("owner", owner.Id)
	todo.Set("list", list.Id)
	todo.Set("title", "Delete me")
	if err := server.app.Save(todo); err != nil {
		t.Fatal(err)
	}
	token := authToken(t, server, owner.Email())
	wrongBody := fmt.Sprintf(`{"confirmEmail":%q}`, other.Email())
	status, _, body := integrationRequest(t, server, http.MethodPost, "/api/account/delete", wrongBody, withContentType(bearerHeaders(token)))
	if status != http.StatusBadRequest {
		t.Fatalf("mismatched confirmation status = %d, body = %s", status, body)
	}
	if _, err := server.app.FindRecordById("users", owner.Id); err != nil {
		t.Fatalf("account was deleted after a mismatched confirmation: %v", err)
	}

	confirmBody := fmt.Sprintf(`{"confirmEmail":%q}`, owner.Email())
	status, _, body = integrationRequest(t, server, http.MethodPost, "/api/account/delete", confirmBody, withContentType(bearerHeaders(token)))
	if status != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", status, body)
	}
	if _, err := server.app.FindRecordById("users", owner.Id); err == nil {
		t.Fatal("account still exists after deletion")
	}
	if _, err := server.app.FindRecordById("todos", todo.Id); err == nil {
		t.Fatal("owned todo still exists after deletion")
	}
	if _, err := server.app.FindRecordById("users", other.Id); err != nil {
		t.Fatalf("another user's account was affected: %v", err)
	}
}
