package app

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	defaultSearchPage    = 1
	defaultSearchPerPage = 20
	maxSearchPerPage     = 100
)

type searchParams struct {
	Query     string
	Type      string
	From      *time.Time
	To        *time.Time
	Container string
	Completed *bool
	Status    string
	Page      int
	PerPage   int
}

type searchResult struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Container   string `json:"container,omitempty"`
	Date        string `json:"date,omitempty"`
	Completed   *bool  `json:"completed,omitempty"`
	Status      string `json:"status,omitempty"`
}

type searchResponse struct {
	Page       int            `json:"page"`
	PerPage    int            `json:"perPage"`
	TotalItems int            `json:"totalItems"`
	TotalPages int            `json:"totalPages"`
	Items      []searchResult `json:"items"`
}

type searchCollection struct {
	name             string
	resultType       string
	titleField       string
	description      string
	queryFields      []string
	containerField   string
	dateField        string
	supportsStatus   bool
	supportsComplete bool
}

var searchCollections = []searchCollection{
	{name: "todos", resultType: "todo", titleField: "title", description: "description", queryFields: []string{"title", "description"}, containerField: "list", dateField: "due_date", supportsStatus: true, supportsComplete: true},
	{name: "events", resultType: "event", titleField: "title", description: "description", queryFields: []string{"title", "description", "location"}, containerField: "calendar", dateField: "start_date", supportsStatus: true},
	{name: "lists", resultType: "list", titleField: "name", description: "description", queryFields: []string{"name", "description"}},
	{name: "calendars", resultType: "calendar", titleField: "name", description: "description", queryFields: []string{"name", "description"}},
	{name: "contacts", resultType: "contact", titleField: "formatted_name", description: "notes", queryFields: []string{"formatted_name", "given_name", "family_name", "organization", "title", "email", "phone", "address", "notes"}, containerField: "address_book"},
}

// registerSearchRoutes exposes deep search. It is intentionally not wired into
// registerRoutes yet so callers can opt in when the API contract is published.
func registerSearchRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/search", searchRoute)
}

func searchRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	params, err := parseSearchParams(event.Request.URL.Query())
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	response, err := deepSearch(event.App, user.Id, params)
	if err != nil {
		return err
	}
	return event.JSON(http.StatusOK, response)
}

func parseSearchParams(values url.Values) (searchParams, error) {
	params := searchParams{
		Query:     strings.TrimSpace(values.Get("query")),
		Type:      strings.ToLower(strings.TrimSpace(values.Get("type"))),
		Container: strings.TrimSpace(values.Get("container")),
		Status:    strings.TrimSpace(values.Get("status")),
		Page:      defaultSearchPage,
		PerPage:   defaultSearchPerPage,
	}

	if params.Type != "" && !validSearchType(params.Type) {
		return searchParams{}, fmt.Errorf("type must be one of todo, event, list, calendar, contact")
	}
	var err error
	if raw := strings.TrimSpace(values.Get("from")); raw != "" {
		from, parseErr := parseSearchDate(raw, false)
		if parseErr != nil {
			return searchParams{}, fmt.Errorf("from must be an RFC3339 timestamp or YYYY-MM-DD date")
		}
		params.From = &from
	}
	if raw := strings.TrimSpace(values.Get("to")); raw != "" {
		to, parseErr := parseSearchDate(raw, true)
		if parseErr != nil {
			return searchParams{}, fmt.Errorf("to must be an RFC3339 timestamp or YYYY-MM-DD date")
		}
		params.To = &to
	}
	if params.From != nil && params.To != nil && params.From.After(*params.To) {
		return searchParams{}, fmt.Errorf("from must not be after to")
	}
	if raw := strings.TrimSpace(values.Get("completed")); raw != "" {
		completed, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			return searchParams{}, fmt.Errorf("completed must be true or false")
		}
		params.Completed = &completed
	}
	if params.Page, err = parseBoundedInt(values.Get("page"), defaultSearchPage, 1, int(^uint(0)>>1), "page"); err != nil {
		return searchParams{}, err
	}
	if params.PerPage, err = parseBoundedInt(values.Get("perPage"), defaultSearchPerPage, 1, maxSearchPerPage, "perPage"); err != nil {
		return searchParams{}, err
	}
	return params, nil
}

func parseSearchDate(value string, endOfDay bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return parsed.UTC(), nil
}

func parseBoundedInt(raw string, fallback, minimum, maximum int, name string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func validSearchType(value string) bool {
	for _, collection := range searchCollections {
		if collection.resultType == value {
			return true
		}
	}
	return false
}

func deepSearch(app core.App, owner string, params searchParams) (searchResponse, error) {
	items := make([]searchResult, 0)
	for _, collection := range searchCollections {
		if params.Type != "" && params.Type != collection.resultType {
			continue
		}
		if (params.From != nil || params.To != nil) && collection.dateField == "" {
			continue
		}
		if params.Container != "" && collection.containerField == "" {
			continue
		}
		if params.Completed != nil && !collection.supportsComplete {
			continue
		}
		if params.Status != "" && !collection.supportsStatus {
			continue
		}

		filter, filterParams := searchFilter(collection, owner, params)
		records, err := app.FindRecordsByFilter(collection.name, filter, "", 0, 0, filterParams)
		if err != nil {
			return searchResponse{}, fmt.Errorf("search %s: %w", collection.name, err)
		}
		for _, record := range records {
			items = append(items, resultFromRecord(collection, record))
		}
	}

	sort.Slice(items, func(i, j int) bool {
		left, right := strings.ToLower(items[i].Title), strings.ToLower(items[j].Title)
		if left != right {
			return left < right
		}
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].ID < items[j].ID
	})

	total := len(items)
	totalPages := 0
	if total > 0 {
		totalPages = 1 + (total-1)/params.PerPage
	}
	// Check the page offset before multiplying. Page is intentionally allowed
	// to be a large positive int, and (page-1)*perPage can otherwise wrap.
	pageOffset := params.Page - 1
	start := total
	if pageOffset <= total/params.PerPage {
		start = pageOffset * params.PerPage
		if start > total {
			start = total
		}
	}
	end := total
	if params.PerPage <= total-start {
		end = start + params.PerPage
	}
	return searchResponse{Page: params.Page, PerPage: params.PerPage, TotalItems: total, TotalPages: totalPages, Items: items[start:end]}, nil
}

func searchFilter(collection searchCollection, owner string, params searchParams) (string, dbx.Params) {
	parts := []string{"owner = {:owner}"}
	values := dbx.Params{"owner": owner}
	if params.Query != "" {
		queryParts := make([]string, 0, len(collection.queryFields))
		for index, field := range collection.queryFields {
			name := fmt.Sprintf("query%d", index)
			queryParts = append(queryParts, field+" ~ {:"+name+"}")
			values[name] = params.Query
		}
		parts = append(parts, "("+strings.Join(queryParts, " || ")+")")
	}
	if params.Container != "" {
		parts = append(parts, collection.containerField+" = {:container}")
		values["container"] = params.Container
	}
	if params.From != nil {
		parts = append(parts, collection.dateField+" >= {:from}")
		values["from"] = *params.From
	}
	if params.To != nil {
		parts = append(parts, collection.dateField+" <= {:to}")
		values["to"] = *params.To
	}
	if params.Completed != nil {
		parts = append(parts, "completed = {:completed}")
		values["completed"] = *params.Completed
	}
	if params.Status != "" {
		parts = append(parts, "status = {:status}")
		values["status"] = params.Status
	}
	return strings.Join(parts, " && "), values
}

func resultFromRecord(collection searchCollection, record *core.Record) searchResult {
	result := searchResult{
		ID:          record.Id,
		Type:        collection.resultType,
		Title:       record.GetString(collection.titleField),
		Description: record.GetString(collection.description),
		Status:      record.GetString("status"),
	}
	if collection.containerField != "" {
		result.Container = record.GetString(collection.containerField)
	}
	if collection.dateField != "" {
		result.Date = record.GetDateTime(collection.dateField).String()
	}
	if collection.supportsComplete {
		completed := record.GetBool("completed")
		result.Completed = &completed
	}
	return result
}
