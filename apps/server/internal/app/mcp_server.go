package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const mcpProtocolVersion = "2025-11-25"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpToolResult struct {
	Content []map[string]string `json:"content"`
	IsError bool                `json:"isError,omitempty"`
}

// registerMCPRoute installs a stateless Streamable HTTP MCP endpoint. OAuth
// token verification is handled before dispatch so tool handlers only receive
// an authenticated user identity.
func registerMCPRoute(e *core.ServeEvent, config Config) {
	if len(config.MCPOAuthSigningKey) < 32 {
		return // MCP is disabled until OAuth is configured with a strong signing key.
	}
	path := config.MCPPath
	if path == "" {
		path = "/mcp"
	}
	e.Router.POST(path, func(event *core.RequestEvent) error {
		user, err := mcpUserFromRequest(event.App, event.Request, config)
		if err != nil {
			resourceMetadata := strings.TrimRight(config.PublicURL, "/") + "/.well-known/oauth-protected-resource/" + strings.Trim(strings.TrimSpace(path), "/")
			event.Response.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+resourceMetadata+`"`)
			return event.UnauthorizedError("valid MCP bearer token required", err)
		}
		var request mcpRequest
		decoder := json.NewDecoder(io.LimitReader(event.Request.Body, 1<<20))
		if err := decoder.Decode(&request); err != nil {
			return event.JSON(http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpRPCError{Code: -32700, Message: "invalid JSON request"}})
		}
		response := dispatchMCPRequest(event.App, user.Id, request)
		if request.Method == "notifications/initialized" {
			return event.NoContent(http.StatusAccepted)
		}
		return event.JSON(http.StatusOK, response)
	})
}

func dispatchMCPRequest(app core.App, owner string, request mcpRequest) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	if request.JSONRPC != "2.0" || len(request.ID) == 0 {
		response.Error = &mcpRPCError{Code: -32600, Message: "invalid JSON-RPC request"}
		return response
	}
	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "taskboard", "version": "1.0.0"},
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		var call mcpToolCall
		if err := json.Unmarshal(request.Params, &call); err != nil {
			response.Error = &mcpRPCError{Code: -32602, Message: "invalid tool call parameters"}
			return response
		}
		result, err := callMCPTool(app, owner, call)
		if err != nil {
			response.Result = mcpToolResult{Content: []map[string]string{{"type": "text", "text": err.Error()}}, IsError: true}
			return response
		}
		response.Result = result
	default:
		response.Error = &mcpRPCError{Code: -32601, Message: "method not found"}
	}
	return response
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "list_tasks", "description": "Search and filter the current user's tasks, including undated backlog tasks", "inputSchema": objectSchema(map[string]any{"completed": map[string]any{"type": "boolean"}, "status": map[string]any{"type": "string", "enum": []string{"NEEDS-ACTION", "IN-PROCESS", "COMPLETED", "CANCELLED"}}, "list": map[string]any{"type": "string"}, "priority": map[string]any{"type": "string"}, "search": map[string]any{"type": "string"}, "undated": map[string]any{"type": "boolean"}, "due_after": map[string]any{"type": "string", "format": "date-time"}, "due_before": map[string]any{"type": "string", "format": "date-time"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}})},
		{"name": "get_task", "description": "Get one of the current user's tasks", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}}, "id")},
		{"name": "create_task", "description": "Create a task in one of the current user's lists; omit due_date for an unscheduled backlog task", "inputSchema": objectSchema(map[string]any{"title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "list": map[string]any{"type": "string"}, "status": map[string]any{"type": "string", "enum": []string{"NEEDS-ACTION", "IN-PROCESS", "COMPLETED", "CANCELLED"}}, "priority": map[string]any{"type": "string"}, "due_date": map[string]any{"type": "string", "format": "date-time"}}, "title", "list")},
		{"name": "update_task", "description": "Update task details, status, scheduling, or list", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "completed": map[string]any{"type": "boolean"}, "status": map[string]any{"type": "string", "enum": []string{"NEEDS-ACTION", "IN-PROCESS", "COMPLETED", "CANCELLED"}}, "list": map[string]any{"type": "string"}, "priority": map[string]any{"type": "string"}, "due_date": map[string]any{"type": "string", "format": "date-time"}}, "id")},
		{"name": "delete_task", "description": "Delete one of the current user's tasks. Requires confirm=true.", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "confirm": map[string]any{"type": "boolean"}}, "id", "confirm")},
		{"name": "list_projects", "description": "List and search the current user's project lists", "inputSchema": objectSchema(map[string]any{"search": map[string]any{"type": "string"}, "status": map[string]any{"type": "string", "enum": []string{"active", "paused", "completed"}}})},
		{"name": "get_project", "description": "Get project metadata, task counts, backlog, upcoming deadlines, and overdue tasks", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}}, "id")},
		{"name": "create_project", "description": "Create a project using a Taskboard list", "inputSchema": objectSchema(map[string]any{"name": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "home_url": map[string]any{"type": "string", "format": "uri"}, "target_date": map[string]any{"type": "string", "format": "date"}}, "name")},
		{"name": "update_project", "description": "Update a project or archive it", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "home_url": map[string]any{"type": "string", "format": "uri"}, "target_date": map[string]any{"type": "string", "format": "date"}, "status": map[string]any{"type": "string", "enum": []string{"active", "paused", "completed"}}, "archived": map[string]any{"type": "boolean"}}, "id")},
		{"name": "list_calendars", "description": "List the current user's calendars", "inputSchema": objectSchema(map[string]any{})},
		{"name": "list_events", "description": "Search events or list them in a date range; set upcoming to true for future events", "inputSchema": objectSchema(map[string]any{"calendar": map[string]any{"type": "string"}, "search": map[string]any{"type": "string"}, "start_after": map[string]any{"type": "string", "format": "date-time"}, "start_before": map[string]any{"type": "string", "format": "date-time"}, "upcoming": map[string]any{"type": "boolean"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}})},
		{"name": "get_event", "description": "Read one of the current user's events", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}}, "id")},
		{"name": "create_event", "description": "Create an event in one of the user's calendars", "inputSchema": objectSchema(map[string]any{"title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "calendar": map[string]any{"type": "string"}, "start_date": map[string]any{"type": "string"}, "end_date": map[string]any{"type": "string"}, "all_day": map[string]any{"type": "boolean"}, "timezone": map[string]any{"type": "string"}, "rrule": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}}, "title", "calendar", "start_date")},
		{"name": "update_event", "description": "Update an event, including calendar and recurrence fields", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "calendar": map[string]any{"type": "string"}, "start_date": map[string]any{"type": "string"}, "end_date": map[string]any{"type": "string"}, "all_day": map[string]any{"type": "boolean"}, "timezone": map[string]any{"type": "string"}, "rrule": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}}, "id")},
		{"name": "delete_event", "description": "Delete one of the current user's events. Requires confirm=true.", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "confirm": map[string]any{"type": "boolean"}}, "id", "confirm")},
		{"name": "list_contacts", "description": "Search and list the current user's contacts", "inputSchema": objectSchema(map[string]any{"address_book": map[string]any{"type": "string"}, "search": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}})},
		{"name": "get_contact", "description": "Read one of the current user's contacts", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}}, "id")},
		{"name": "create_contact", "description": "Create a contact in one of the user's address books", "inputSchema": objectSchema(map[string]any{"address_book": map[string]any{"type": "string"}, "formatted_name": map[string]any{"type": "string"}, "given_name": map[string]any{"type": "string"}, "family_name": map[string]any{"type": "string"}, "organization": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "email": map[string]any{"type": "string"}, "phone": map[string]any{"type": "string"}, "address": map[string]any{"type": "string"}, "notes": map[string]any{"type": "string"}}, "address_book", "formatted_name")},
		{"name": "update_contact", "description": "Update one of the current user's contacts", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "formatted_name": map[string]any{"type": "string"}, "given_name": map[string]any{"type": "string"}, "family_name": map[string]any{"type": "string"}, "organization": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "email": map[string]any{"type": "string"}, "phone": map[string]any{"type": "string"}, "address": map[string]any{"type": "string"}, "notes": map[string]any{"type": "string"}}, "id")},
		{"name": "delete_contact", "description": "Delete one of the current user's contacts. Requires confirm=true.", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "confirm": map[string]any{"type": "boolean"}}, "id", "confirm")},
	}
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func callMCPTool(app core.App, owner string, call mcpToolCall) (mcpToolResult, error) {
	var value any
	var err error
	switch call.Name {
	case "list_tasks":
		value, err = listMCPTasks(app, owner, call.Arguments)
	case "get_task":
		value, err = getMCPTask(app, owner, call.Arguments)
	case "create_task":
		value, err = createMCPTask(app, owner, call.Arguments)
	case "update_task":
		value, err = updateMCPTask(app, owner, call.Arguments)
	case "delete_task":
		value, err = deleteMCPTask(app, owner, call.Arguments)
	case "list_projects":
		value, err = listMCPProjects(app, owner, call.Arguments)
	case "get_project":
		value, err = getMCPProject(app, owner, call.Arguments)
	case "create_project":
		value, err = createMCPProject(app, owner, call.Arguments)
	case "update_project":
		value, err = updateMCPProject(app, owner, call.Arguments)
	default:
		var handled bool
		value, err, handled = callMCPCalendarContactTool(app, owner, call)
		if !handled {
			return mcpToolResult{}, fmt.Errorf("unknown tool %q", call.Name)
		}
	}
	if err != nil {
		return mcpToolResult{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return mcpToolResult{}, err
	}
	return mcpToolResult{Content: []map[string]string{{"type": "text", "text": string(data)}}}, nil
}

func listMCPTasks(app core.App, owner string, args map[string]any) ([]map[string]any, error) {
	limit := 50
	if raw, ok := args["limit"].(float64); ok {
		if raw < 1 || raw > 100 || raw != float64(int(raw)) {
			return nil, errors.New("limit must be between 1 and 100")
		}
		limit = int(raw)
	}
	filter := "owner = {:owner}"
	params := dbx.Params{"owner": owner}
	for key, field := range map[string]string{"list": "list", "status": "status", "priority": "priority"} {
		if value, ok := args[key].(string); ok && value != "" {
			filter += " && " + field + " = {:" + key + "}"
			params[key] = value
		}
	}
	if search := stringArg(args, "search"); search != "" {
		filter += " && (title ~ {:search} || description ~ {:search})"
		params["search"] = search
	}
	if undated, ok := args["undated"].(bool); ok {
		if undated {
			filter += " && (due_date = '' || due_date = null)"
		} else {
			filter += " && due_date != '' && due_date != null"
		}
	}
	for key, operator := range map[string]string{"due_after": ">=", "due_before": "<="} {
		if value := stringArg(args, key); value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return nil, fmt.Errorf("%s must be an RFC3339 date-time", key)
			}
			filter += " && due_date " + operator + " {:" + key + "}"
			params[key] = value
		}
	}
	if completed, ok := args["completed"].(bool); ok {
		filter += " && completed = {:completed}"
		params["completed"] = completed
	}
	records, err := app.FindRecordsByFilter("todos", filter, "due_date,title", limit, 0, params)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		result = append(result, mcpTaskData(record))
	}
	return result, nil
}

func getMCPTask(app core.App, owner string, args map[string]any) (map[string]any, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return nil, errors.New("id is required")
	}
	records, err := app.FindRecordsByFilter("todos", "id = {:id} && owner = {:owner}", "", 1, 0, dbx.Params{"id": id, "owner": owner})
	if err != nil || len(records) == 0 {
		return nil, errors.New("task not found")
	}
	return mcpTaskData(records[0]), nil
}

func createMCPTask(app core.App, owner string, args map[string]any) (map[string]any, error) {
	title, _ := args["title"].(string)
	listID, _ := args["list"].(string)
	if title == "" {
		return nil, errors.New("title is required")
	}
	if err := validateEntryContainer(app, "todos", listID, owner); err != nil {
		return nil, err
	}
	collection, err := app.FindCollectionByNameOrId("todos")
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(collection)
	record.Set("owner", owner)
	record.Set("title", title)
	record.Set("description", stringArg(args, "description"))
	record.Set("list", listID)
	status := stringArg(args, "status")
	if !validMCPTaskStatus(status) {
		status = "NEEDS-ACTION"
	}
	record.Set("status", status)
	record.Set("completed", status == "COMPLETED")
	record.Set("priority", stringArg(args, "priority"))
	if due, ok := args["due_date"].(string); ok && due != "" {
		record.Set("due_date", due)
	}
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpTaskData(record), nil
}

func updateMCPTask(app core.App, owner string, args map[string]any) (map[string]any, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return nil, errors.New("id is required")
	}
	record, err := app.FindRecordById("todos", id)
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("task not found")
	}
	if title, ok := args["title"].(string); ok {
		if title == "" {
			return nil, errors.New("title cannot be empty")
		}
		record.Set("title", title)
	}
	if description, ok := args["description"].(string); ok {
		record.Set("description", description)
	}
	if listID, ok := args["list"].(string); ok {
		if err := validateEntryContainer(app, "todos", listID, owner); err != nil {
			return nil, err
		}
		record.Set("list", listID)
	}
	if priority, ok := args["priority"].(string); ok {
		record.Set("priority", priority)
	}
	if status, ok := args["status"].(string); ok {
		if !validMCPTaskStatus(status) {
			return nil, errors.New("invalid task status")
		}
		record.Set("status", status)
		record.Set("completed", status == "COMPLETED")
	}
	if completed, ok := args["completed"].(bool); ok {
		record.Set("completed", completed)
		if completed {
			record.Set("status", "COMPLETED")
		} else {
			record.Set("status", "NEEDS-ACTION")
		}
	}
	if due, ok := args["due_date"].(string); ok {
		record.Set("due_date", due)
	}
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpTaskData(record), nil
}

func validMCPTaskStatus(value string) bool {
	switch value {
	case "NEEDS-ACTION", "IN-PROCESS", "COMPLETED", "CANCELLED":
		return true
	default:
		return false
	}
}

func deleteMCPTask(app core.App, owner string, args map[string]any) (map[string]any, error) {
	if confirmed, _ := args["confirm"].(bool); !confirmed {
		return nil, errors.New("deleting a task requires confirm=true")
	}
	id := stringArg(args, "id")
	record, err := app.FindRecordById("todos", id)
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("task not found")
	}
	result := mcpTaskData(record)
	if err := app.Delete(record); err != nil {
		return nil, err
	}
	return result, nil
}

func listMCPProjects(app core.App, owner string, args map[string]any) ([]map[string]any, error) {
	filter := "owner = {:owner} && is_project = true && archived != true"
	params := dbx.Params{"owner": owner}
	if status := stringArg(args, "status"); status != "" {
		if !validMCPProjectStatus(status) {
			return nil, errors.New("invalid project status")
		}
		filter += " && project_status = {:status}"
		params["status"] = status
	}
	if search := stringArg(args, "search"); search != "" {
		filter += " && (name ~ {:search} || description ~ {:search})"
		params["search"] = search
	}
	records, err := app.FindRecordsByFilter("lists", filter, "name", 100, 0, params)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		data, err := mcpProjectData(app, owner, record)
		if err != nil {
			return nil, err
		}
		result = append(result, data)
	}
	return result, nil
}

func getMCPProject(app core.App, owner string, args map[string]any) (map[string]any, error) {
	id := stringArg(args, "id")
	record, err := app.FindRecordById("lists", id)
	if err != nil || record.GetString("owner") != owner || !record.GetBool("is_project") {
		return nil, errors.New("project not found")
	}
	return mcpProjectData(app, owner, record)
}

func createMCPProject(app core.App, owner string, args map[string]any) (map[string]any, error) {
	name := strings.TrimSpace(stringArg(args, "name"))
	if name == "" {
		return nil, errors.New("name is required")
	}
	if err := validateMCPHomeURL(stringArg(args, "home_url")); err != nil {
		return nil, err
	}
	if date := stringArg(args, "target_date"); date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, errors.New("target_date must be YYYY-MM-DD")
		}
	}
	collection, err := app.FindCollectionByNameOrId("lists")
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(collection)
	record.Set("owner", owner)
	record.Set("name", name)
	record.Set("description", stringArg(args, "description"))
	record.Set("color", "#2f6f63")
	record.Set("is_project", true)
	record.Set("project_status", "active")
	record.Set("home_url", stringArg(args, "home_url"))
	record.Set("target_date", stringArg(args, "target_date"))
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpProjectData(app, owner, record)
}

func updateMCPProject(app core.App, owner string, args map[string]any) (map[string]any, error) {
	record, err := app.FindRecordById("lists", stringArg(args, "id"))
	if err != nil || record.GetString("owner") != owner || !record.GetBool("is_project") {
		return nil, errors.New("project not found")
	}
	if name, ok := args["name"].(string); ok {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("name cannot be empty")
		}
		record.Set("name", strings.TrimSpace(name))
	}
	if value, ok := args["description"].(string); ok {
		record.Set("description", value)
	}
	if value, ok := args["home_url"].(string); ok {
		if err := validateMCPHomeURL(value); err != nil {
			return nil, err
		}
		record.Set("home_url", value)
	}
	if value, ok := args["target_date"].(string); ok {
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return nil, errors.New("target_date must be YYYY-MM-DD")
			}
		}
		record.Set("target_date", value)
	}
	if value, ok := args["status"].(string); ok {
		if !validMCPProjectStatus(value) {
			return nil, errors.New("invalid project status")
		}
		record.Set("project_status", value)
	}
	if value, ok := args["archived"].(bool); ok {
		record.Set("archived", value)
	}
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpProjectData(app, owner, record)
}

func validMCPProjectStatus(value string) bool {
	return value == "active" || value == "paused" || value == "completed"
}

func validateMCPHomeURL(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("home_url must be an HTTP or HTTPS URL")
	}
	return nil
}

func mcpProjectData(app core.App, owner string, project *core.Record) (map[string]any, error) {
	all, err := app.FindRecordsByFilter("todos", "owner={:owner} && list={:list}", "", 500, 0, dbx.Params{"owner": owner, "list": project.Id})
	if err != nil {
		return nil, err
	}
	tasks := make([]map[string]any, 0, len(all))
	completed, remaining := 0, 0
	backlog, overdue, upcoming := []map[string]any{}, []map[string]any{}, []map[string]any{}
	today := time.Now().Format("2006-01-02")
	for _, task := range all {
		data := mcpTaskData(task)
		date := task.GetString("due_date")
		isDone := task.GetBool("completed") || task.GetString("status") == "COMPLETED"
		if isDone {
			completed++
		} else {
			remaining++
		}
		if date == "" && !isDone {
			backlog = append(backlog, data)
		}
		if date != "" && date[:min(len(date), 10)] < today && !isDone {
			overdue = append(overdue, data)
		}
		if date != "" && date[:min(len(date), 10)] >= today && !isDone {
			upcoming = append(upcoming, data)
		}
		tasks = append(tasks, data)
	}
	progress := 0
	if len(all) > 0 {
		progress = completed * 100 / len(all)
	}
	return map[string]any{"id": project.Id, "name": project.GetString("name"), "description": project.GetString("description"), "home_url": project.GetString("home_url"), "target_date": project.GetString("target_date"), "status": project.GetString("project_status"), "archived": project.GetBool("archived"), "completed_tasks": completed, "remaining_tasks": remaining, "progress_percent": progress, "backlog": backlog, "upcoming": upcoming, "overdue": overdue, "tasks": tasks}, nil
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func mcpTaskData(record *core.Record) map[string]any {
	return map[string]any{
		"id": record.Id, "title": record.GetString("title"), "description": record.GetString("description"),
		"list": record.GetString("list"), "completed": record.GetBool("completed"), "status": record.GetString("status"), "priority": record.GetString("priority"), "due_date": record.GetString("due_date"), "checklist": record.Get("checklist"),
	}
}
