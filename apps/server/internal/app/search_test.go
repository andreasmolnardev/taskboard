package app

import (
	"net/url"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestDeepSearchIsolatesOwnersAndTreatsQueryAsData(t *testing.T) {
	testApp, owner, other := newSearchTestApp(t)
	defer testApp.Cleanup()

	addSearchRecord(t, testApp, "todos", map[string]any{"owner": owner.Id, "title": "Owner needle", "description": "safe"})
	addSearchRecord(t, testApp, "todos", map[string]any{"owner": other.Id, "title": "Other needle", "description": "private"})

	params := mustSearchParams(t, url.Values{"query": {"needle"}})
	response, err := deepSearch(testApp, owner.Id, params)
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalItems != 1 || response.Items[0].Title != "Owner needle" {
		t.Fatalf("expected only the owner's match, got %#v", response.Items)
	}

	params = mustSearchParams(t, url.Values{"query": {`needle" || owner != "`}})
	response, err = deepSearch(testApp, owner.Id, params)
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalItems != 0 {
		t.Fatalf("query text changed the filter, got %#v", response.Items)
	}
}

func TestDeepSearchFiltersTypeDateContainerCompletionAndStatus(t *testing.T) {
	testApp, owner, _ := newSearchTestApp(t)
	defer testApp.Cleanup()

	list := addSearchRecord(t, testApp, "lists", map[string]any{"owner": owner.Id, "name": "Work"})
	otherList := addSearchRecord(t, testApp, "lists", map[string]any{"owner": owner.Id, "name": "Home"})
	addSearchRecord(t, testApp, "todos", map[string]any{
		"owner": owner.Id, "title": "Ship release", "list": list.Id, "due_date": "2026-09-15 12:00:00.000Z", "completed": true, "status": "COMPLETED",
	})
	addSearchRecord(t, testApp, "todos", map[string]any{
		"owner": owner.Id, "title": "Ship old work", "list": list.Id, "due_date": "2026-08-01 12:00:00.000Z", "completed": true, "status": "COMPLETED",
	})
	addSearchRecord(t, testApp, "todos", map[string]any{
		"owner": owner.Id, "title": "Ship home", "list": otherList.Id, "due_date": "2026-09-15 12:00:00.000Z", "completed": true, "status": "COMPLETED",
	})
	addSearchRecord(t, testApp, "todos", map[string]any{
		"owner": owner.Id, "title": "Ship pending", "list": list.Id, "due_date": "2026-09-15 12:00:00.000Z", "completed": false, "status": "NEEDS-ACTION",
	})
	addSearchRecord(t, testApp, "events", map[string]any{
		"owner": owner.Id, "title": "Ship event", "calendar": list.Id, "start_date": "2026-09-15 12:00:00.000Z", "status": "COMPLETED",
	})

	params := mustSearchParams(t, url.Values{
		"query": {"Ship"}, "type": {"todo"}, "from": {"2026-09-01"}, "to": {"2026-09-30"},
		"container": {list.Id}, "completed": {"true"}, "status": {"COMPLETED"},
	})
	response, err := deepSearch(testApp, owner.Id, params)
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalItems != 1 || response.Items[0].Title != "Ship release" {
		t.Fatalf("expected one fully filtered todo, got %#v", response.Items)
	}
	if response.Items[0].Type != "todo" || response.Items[0].Completed == nil || !*response.Items[0].Completed {
		t.Fatalf("todo result lost type or completion state: %#v", response.Items[0])
	}
}

func TestDeepSearchPaginatesMergedResults(t *testing.T) {
	testApp, owner, _ := newSearchTestApp(t)
	defer testApp.Cleanup()

	addSearchRecord(t, testApp, "events", map[string]any{"owner": owner.Id, "title": "Bravo", "start_date": "2026-09-15 12:00:00.000Z"})
	addSearchRecord(t, testApp, "contacts", map[string]any{"owner": owner.Id, "address_book": "book", "uid": "charlie", "formatted_name": "Charlie"})
	addSearchRecord(t, testApp, "lists", map[string]any{"owner": owner.Id, "name": "Alpha"})

	params := mustSearchParams(t, url.Values{"page": {"2"}, "perPage": {"2"}})
	response, err := deepSearch(testApp, owner.Id, params)
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalItems != 3 || response.TotalPages != 2 || len(response.Items) != 1 || response.Items[0].Title != "Charlie" {
		t.Fatalf("unexpected merged page: %#v", response)
	}
}

func TestDeepSearchLargePageDoesNotOverflow(t *testing.T) {
	testApp, owner, _ := newSearchTestApp(t)
	defer testApp.Cleanup()
	addSearchRecord(t, testApp, "lists", map[string]any{"owner": owner.Id, "name": "Only result"})
	params := mustSearchParams(t, url.Values{"page": {"9223372036854775807"}, "perPage": {"100"}})
	response, err := deepSearch(testApp, owner.Id, params)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 0 || response.TotalItems != 1 {
		t.Fatalf("unexpected large page response: %#v", response)
	}
}

func TestParseSearchParamsRejectsMalformedValues(t *testing.T) {
	tests := []url.Values{
		{"type": {"note"}},
		{"from": {"yesterday"}},
		{"to": {"2026-13-01"}},
		{"from": {"2026-09-02"}, "to": {"2026-09-01"}},
		{"completed": {"sometimes"}},
		{"page": {"0"}},
		{"page": {"many"}},
		{"perPage": {"0"}},
		{"perPage": {"101"}},
	}
	for _, values := range tests {
		if _, err := parseSearchParams(values); err == nil {
			t.Errorf("expected malformed params to fail: %v", values)
		}
	}
}

func TestParseSearchParamsDefaultsAndBounds(t *testing.T) {
	params := mustSearchParams(t, nil)
	if params.Page != defaultSearchPage || params.PerPage != defaultSearchPerPage {
		t.Fatalf("unexpected defaults: %#v", params)
	}
	params = mustSearchParams(t, url.Values{"page": {"3"}, "perPage": {"100"}})
	if params.Page != 3 || params.PerPage != 100 {
		t.Fatalf("unexpected bounds: %#v", params)
	}
}

func newSearchTestApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
	t.Helper()
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureCollections(testApp); err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	return testApp, newUser(t, testApp, "search@example.com"), newUser(t, testApp, "other-search@example.com")
}

func addSearchRecord(t *testing.T, app core.App, collectionName string, fields map[string]any) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(collection)
	for name, value := range fields {
		record.Set(name, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func mustSearchParams(t *testing.T, values url.Values) searchParams {
	t.Helper()
	params, err := parseSearchParams(values)
	if err != nil {
		t.Fatal(err)
	}
	return params
}
