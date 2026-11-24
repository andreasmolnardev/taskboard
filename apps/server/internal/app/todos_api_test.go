package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestTodoCRUDHTTPIntegration(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "todo-owner@example.com")
	other := integrationUser(t, server.app, "todo-other@example.com")
	ownerToken := authToken(t, server, owner.GetString("email"))
	otherToken := authToken(t, server, other.GetString("email"))
	ownerAuth := bearerHeaders(ownerToken)
	otherAuth := bearerHeaders(otherToken)

	status, _, _ := integrationRequest(t, server, http.MethodGet, "/api/todos", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d, want %d", status, http.StatusUnauthorized)
	}
	status, _, _ = integrationRequest(t, server, http.MethodPost, "/api/todos", `{"title":"No auth"}`, withContentType(http.Header{}))
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create = %d, want %d", status, http.StatusUnauthorized)
	}

	status, _, body := integrationRequest(t, server, http.MethodPost, "/api/todos", `{"title":"Plan trip","description":"Book train","completed":false,"priority":"1"}`, withContentType(ownerAuth.Clone()))
	if status != http.StatusCreated {
		t.Fatalf("create = %d, body %s", status, body)
	}
	var created todoRecord
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Title != "Plan trip" || created.Description != "Book train" || created.Priority != "1" {
		t.Fatalf("created todo = %#v", created)
	}

	status, _, body = integrationRequest(t, server, http.MethodGet, "/api/todos", "", ownerAuth)
	var listed []todoRecord
	if status != http.StatusOK || json.Unmarshal([]byte(body), &listed) != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("owner list = %d, body %s", status, body)
	}
	status, _, body = integrationRequest(t, server, http.MethodGet, "/api/todos", "", otherAuth)
	if status != http.StatusOK || json.Unmarshal([]byte(body), &listed) != nil || len(listed) != 0 {
		t.Fatalf("other user's list = %d, body %s", status, body)
	}

	path := "/api/todos/" + created.ID
	status, _, body = integrationRequest(t, server, http.MethodGet, path, "", ownerAuth)
	if status != http.StatusOK || json.Unmarshal([]byte(body), &created) != nil || created.Title != "Plan trip" {
		t.Fatalf("owner read = %d, body %s", status, body)
	}
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		requestBody := ""
		if method == http.MethodPatch {
			requestBody = `{"title":"Intrude"}`
		}
		status, _, _ = integrationRequest(t, server, method, path, requestBody, otherAuth)
		if status != http.StatusNotFound {
			t.Fatalf("cross-user %s = %d, want %d", method, status, http.StatusNotFound)
		}
	}

	status, _, body = integrationRequest(t, server, http.MethodPatch, path, `{"completed":true,"status":"COMPLETED"}`, withContentType(ownerAuth.Clone()))
	var updated todoRecord
	if status != http.StatusOK || json.Unmarshal([]byte(body), &updated) != nil || !updated.Completed || updated.Status != "COMPLETED" {
		t.Fatalf("update = %d, body %s", status, body)
	}
	status, _, body = integrationRequest(t, server, http.MethodPatch, path, `{"title":"  "}`, withContentType(ownerAuth.Clone()))
	if status != http.StatusBadRequest {
		t.Fatalf("empty title update = %d, body %s", status, body)
	}
	status, _, body = integrationRequest(t, server, http.MethodPatch, path, `{}`, withContentType(ownerAuth.Clone()))
	if status != http.StatusBadRequest {
		t.Fatalf("empty update = %d, body %s", status, body)
	}
	status, _, body = integrationRequest(t, server, http.MethodPost, "/api/todos", `{"title":"Missing title?","unknown":true}`, withContentType(ownerAuth.Clone()))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field create = %d, body %s", status, body)
	}

	status, _, body = integrationRequest(t, server, http.MethodDelete, path, "", ownerAuth)
	if status != http.StatusNoContent {
		t.Fatalf("delete = %d, body %s", status, body)
	}
	status, _, body = integrationRequest(t, server, http.MethodGet, path, "", ownerAuth)
	if status != http.StatusNotFound {
		t.Fatalf("read after delete = %d, body %s", status, body)
	}
	status, _, body = integrationRequest(t, server, http.MethodGet, "/api/todos/missing", "", ownerAuth)
	if status != http.StatusNotFound {
		t.Fatalf("unknown todo = %d, body %s", status, body)
	}
}

func TestTodoCreateRejectsForeignList(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "todo-list-owner@example.com")
	other := integrationUser(t, server.app, "todo-list-other@example.com")
	foreignList := ownedDefault(t, server.app, "lists", other.Id, "Tasks")
	token := authToken(t, server, owner.GetString("email"))
	body := fmt.Sprintf(`{"title":"Wrong list","list":%q}`, foreignList.Id)
	status, _, responseBody := integrationRequest(t, server, http.MethodPost, "/api/todos", body, withContentType(bearerHeaders(token)))
	if status != http.StatusBadRequest {
		t.Fatalf("foreign list create = %d, body %s", status, responseBody)
	}
}
