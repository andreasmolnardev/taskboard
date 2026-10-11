package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/andreasmolnardev/slopstack/go/httpapi"
	"github.com/andreasmolnardev/slopstack/go/ownership"
	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type todoRecord struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Completed       bool   `json:"completed"`
	Description     string `json:"description,omitempty"`
	UID             string `json:"uid,omitempty"`
	DTStamp         string `json:"dtstamp,omitempty"`
	StartDate       string `json:"start_date,omitempty"`
	StartLocal      string `json:"start_local,omitempty"`
	List            string `json:"list,omitempty"`
	Calendar        string `json:"calendar,omitempty"`
	Timezone        string `json:"timezone,omitempty"`
	TimeMode        string `json:"time_mode,omitempty"`
	DueDate         string `json:"due_date,omitempty"`
	DueLocal        string `json:"due_local,omitempty"`
	Duration        string `json:"duration,omitempty"`
	Status          string `json:"status,omitempty"`
	CompletedAt     string `json:"completed_at,omitempty"`
	PercentComplete string `json:"percent_complete,omitempty"`
	Priority        string `json:"priority,omitempty"`
	Categories      string `json:"categories,omitempty"`
	RRule           string `json:"rrule,omitempty"`
	Exdate          string `json:"exdate,omitempty"`
	Organizer       string `json:"organizer,omitempty"`
	Attendees       string `json:"attendees,omitempty"`
	URL             string `json:"url,omitempty"`
	Class           string `json:"class,omitempty"`
	Geo             string `json:"geo,omitempty"`
	Alarm           string `json:"alarm,omitempty"`
	DAVHref         string `json:"dav_href,omitempty"`
	Created         string `json:"created"`
	Updated         string `json:"updated"`
}

// todoInput deliberately excludes owner and server-managed DAV payload fields.
// The owner always comes from the authenticated PocketBase record.
type todoInput struct {
	Title           *string `json:"title,omitempty"`
	Completed       *bool   `json:"completed,omitempty"`
	Description     *string `json:"description,omitempty"`
	UID             *string `json:"uid,omitempty"`
	DTStamp         *string `json:"dtstamp,omitempty"`
	StartDate       *string `json:"start_date,omitempty"`
	StartLocal      *string `json:"start_local,omitempty"`
	List            *string `json:"list,omitempty"`
	Calendar        *string `json:"calendar,omitempty"`
	Timezone        *string `json:"timezone,omitempty"`
	TimeMode        *string `json:"time_mode,omitempty"`
	DueDate         *string `json:"due_date,omitempty"`
	DueLocal        *string `json:"due_local,omitempty"`
	Duration        *string `json:"duration,omitempty"`
	Status          *string `json:"status,omitempty"`
	CompletedAt     *string `json:"completed_at,omitempty"`
	PercentComplete *string `json:"percent_complete,omitempty"`
	Priority        *string `json:"priority,omitempty"`
	Categories      *string `json:"categories,omitempty"`
	RRule           *string `json:"rrule,omitempty"`
	Exdate          *string `json:"exdate,omitempty"`
	Organizer       *string `json:"organizer,omitempty"`
	Attendees       *string `json:"attendees,omitempty"`
	URL             *string `json:"url,omitempty"`
	Class           *string `json:"class,omitempty"`
	Geo             *string `json:"geo,omitempty"`
	Alarm           *string `json:"alarm,omitempty"`
}

type todoBody struct {
	Body todoRecord
}

type todoListBody struct {
	Body []todoRecord
}

type todoPathInput struct {
	ID string `path:"id"`
}

type todoCreateInput struct {
	Body todoInput
}

type todoUpdateInput struct {
	ID   string `path:"id"`
	Body todoInput
}

func registerTodoOpenAPI(api huma.API) {
	security := []map[string][]string{{"pocketbaseAuth": {}}}
	huma.Register(api, huma.Operation{OperationID: "listTodos", Method: http.MethodGet, Path: "/api/todos", Tags: []string{"Todos"}, Summary: "List the current user's todos", Security: security}, listTodos)
	huma.Register(api, huma.Operation{OperationID: "createTodo", Method: http.MethodPost, Path: "/api/todos", Tags: []string{"Todos"}, Summary: "Create a todo for the current user", Security: security, DefaultStatus: http.StatusCreated}, createTodo)
	huma.Register(api, huma.Operation{OperationID: "getTodo", Method: http.MethodGet, Path: "/api/todos/{id}", Tags: []string{"Todos"}, Summary: "Read one of the current user's todos", Security: security}, getTodo)
	huma.Register(api, huma.Operation{OperationID: "updateTodo", Method: http.MethodPatch, Path: "/api/todos/{id}", Tags: []string{"Todos"}, Summary: "Update one of the current user's todos", Security: security}, updateTodo)
	huma.Register(api, huma.Operation{OperationID: "deleteTodo", Method: http.MethodDelete, Path: "/api/todos/{id}", Tags: []string{"Todos"}, Summary: "Delete one of the current user's todos", Security: security, DefaultStatus: http.StatusNoContent}, deleteTodo)
}

func registerTodoHTTPRoutes(e *core.ServeEvent, api huma.API) {
	httpapi.RegisterHumaRoutes(e, api, []string{"/api/todos", "/api/todos/{id}"}, func(request *http.Request, event *core.RequestEvent) *http.Request {
		return request.WithContext(context.WithValue(request.Context(), todoRequestEventContextKey{}, event))
	})
}

type todoRequestEventContextKey struct{}

func todoRequestEvent(ctx context.Context) *core.RequestEvent {
	event, _ := ctx.Value(todoRequestEventContextKey{}).(*core.RequestEvent)
	return event
}

func authenticatedTodoEvent(ctx context.Context) (*core.RequestEvent, error) {
	event := todoRequestEvent(ctx)
	if event == nil || event.App == nil || event.Auth == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}
	return event, nil
}

func listTodos(ctx context.Context, _ *struct{}) (*todoListBody, error) {
	event, err := authenticatedTodoEvent(ctx)
	if err != nil {
		return nil, err
	}
	records, err := event.App.FindRecordsByFilter("todos", "owner = {:owner}", "", 0, 0, dbx.Params{"owner": event.Auth.Id})
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not list todos", err)
	}
	response := make([]todoRecord, 0, len(records))
	for _, record := range records {
		response = append(response, toTodoRecord(record))
	}
	return &todoListBody{Body: response}, nil
}

func createTodo(ctx context.Context, input *todoCreateInput) (*todoBody, error) {
	event, err := authenticatedTodoEvent(ctx)
	if err != nil {
		return nil, err
	}
	if input.Body.Title == nil || strings.TrimSpace(*input.Body.Title) == "" {
		return nil, huma.Error400BadRequest("title is required")
	}
	title := strings.TrimSpace(*input.Body.Title)
	input.Body.Title = &title
	if input.Body.List != nil && strings.TrimSpace(*input.Body.List) != "" {
		if err := validateEntryContainer(event.App, "todos", *input.Body.List, event.Auth.Id); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	collection, err := event.App.FindCollectionByNameOrId("todos")
	if err != nil {
		return nil, huma.Error500InternalServerError("Todo collection is unavailable", err)
	}
	record := core.NewRecord(collection)
	record.Set("owner", event.Auth.Id)
	applyTodoInput(record, input.Body)
	if err := event.App.Save(record); err != nil {
		return nil, huma.Error400BadRequest("Todo could not be saved", err)
	}
	logRequestAudit(event, "create", "todos", record.Id, "todo-api")
	return &todoBody{Body: toTodoRecord(record)}, nil
}

func getTodo(ctx context.Context, input *todoPathInput) (*todoBody, error) {
	event, err := authenticatedTodoEvent(ctx)
	if err != nil {
		return nil, err
	}
	record, err := findOwnedTodo(event, input.ID)
	if err != nil {
		return nil, err
	}
	return &todoBody{Body: toTodoRecord(record)}, nil
}

func updateTodo(ctx context.Context, input *todoUpdateInput) (*todoBody, error) {
	event, err := authenticatedTodoEvent(ctx)
	if err != nil {
		return nil, err
	}
	record, err := findOwnedTodo(event, input.ID)
	if err != nil {
		return nil, err
	}
	if input.Body.Title != nil && strings.TrimSpace(*input.Body.Title) == "" {
		return nil, huma.Error400BadRequest("title must not be empty")
	}
	if !todoInputHasValues(input.Body) {
		return nil, huma.Error400BadRequest("provide at least one field to update")
	}
	if input.Body.Title != nil {
		title := strings.TrimSpace(*input.Body.Title)
		input.Body.Title = &title
	}
	if input.Body.List != nil && strings.TrimSpace(*input.Body.List) != "" {
		if err := validateEntryContainer(event.App, "todos", *input.Body.List, event.Auth.Id); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	applyTodoInput(record, input.Body)
	if err := event.App.Save(record); err != nil {
		return nil, huma.Error400BadRequest("Todo could not be saved", err)
	}
	logRequestAudit(event, "update", "todos", record.Id, "todo-api")
	return &todoBody{Body: toTodoRecord(record)}, nil
}

func deleteTodo(ctx context.Context, input *todoPathInput) (*struct{}, error) {
	event, err := authenticatedTodoEvent(ctx)
	if err != nil {
		return nil, err
	}
	record, err := findOwnedTodo(event, input.ID)
	if err != nil {
		return nil, err
	}
	if err := event.App.Delete(record); err != nil {
		return nil, huma.Error500InternalServerError("Todo could not be deleted", err)
	}
	logRequestAudit(event, "delete", "todos", record.Id, "todo-api")
	return &struct{}{}, nil
}

func findOwnedTodo(event *core.RequestEvent, id string) (*core.Record, error) {
	record, err := event.App.FindRecordById("todos", id)
	if err != nil || !ownership.IsOwner(record, "owner", event.Auth.Id) {
		return nil, huma.Error404NotFound("Todo not found")
	}
	return record, nil
}

func applyTodoInput(record *core.Record, input todoInput) {
	fields := []struct {
		name  string
		value any
	}{
		{"title", input.Title}, {"completed", input.Completed}, {"description", input.Description},
		{"uid", input.UID}, {"dtstamp", input.DTStamp}, {"start_date", input.StartDate},
		{"start_local", input.StartLocal}, {"list", input.List}, {"calendar", input.Calendar},
		{"timezone", input.Timezone}, {"time_mode", input.TimeMode}, {"due_date", input.DueDate},
		{"due_local", input.DueLocal}, {"duration", input.Duration}, {"status", input.Status},
		{"completed_at", input.CompletedAt}, {"percent_complete", input.PercentComplete},
		{"priority", input.Priority}, {"categories", input.Categories}, {"rrule", input.RRule},
		{"exdate", input.Exdate}, {"organizer", input.Organizer}, {"attendees", input.Attendees},
		{"url", input.URL}, {"class", input.Class}, {"geo", input.Geo}, {"alarm", input.Alarm},
	}
	for _, field := range fields {
		switch value := field.value.(type) {
		case *string:
			if value != nil {
				record.Set(field.name, *value)
			}
		case *bool:
			if value != nil {
				record.Set(field.name, *value)
			}
		}
	}
}

func todoInputHasValues(input todoInput) bool {
	return input.Title != nil || input.Completed != nil || input.Description != nil || input.UID != nil ||
		input.DTStamp != nil || input.StartDate != nil || input.StartLocal != nil || input.List != nil ||
		input.Calendar != nil || input.Timezone != nil || input.TimeMode != nil || input.DueDate != nil ||
		input.DueLocal != nil || input.Duration != nil || input.Status != nil || input.CompletedAt != nil ||
		input.PercentComplete != nil || input.Priority != nil || input.Categories != nil || input.RRule != nil ||
		input.Exdate != nil || input.Organizer != nil || input.Attendees != nil || input.URL != nil ||
		input.Class != nil || input.Geo != nil || input.Alarm != nil
}

func toTodoRecord(record *core.Record) todoRecord {
	return todoRecord{
		ID: record.Id, Title: record.GetString("title"), Completed: record.GetBool("completed"),
		Description: record.GetString("description"), UID: record.GetString("uid"), DTStamp: record.GetString("dtstamp"),
		StartDate: record.GetString("start_date"), StartLocal: record.GetString("start_local"),
		List: record.GetString("list"), Calendar: record.GetString("calendar"), Timezone: record.GetString("timezone"),
		TimeMode: record.GetString("time_mode"), DueDate: record.GetString("due_date"), DueLocal: record.GetString("due_local"),
		Duration: record.GetString("duration"), Status: record.GetString("status"), CompletedAt: record.GetString("completed_at"),
		PercentComplete: record.GetString("percent_complete"), Priority: record.GetString("priority"),
		Categories: record.GetString("categories"), RRule: record.GetString("rrule"), Exdate: record.GetString("exdate"),
		Organizer: record.GetString("organizer"), Attendees: record.GetString("attendees"), URL: record.GetString("url"),
		Class: record.GetString("class"), Geo: record.GetString("geo"), Alarm: record.GetString("alarm"),
		DAVHref: record.GetString("dav_href"), Created: record.GetString("created"), Updated: record.GetString("updated"),
	}
}
