package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		{"name": "list_tasks", "description": "List the current user's tasks", "inputSchema": objectSchema(map[string]any{"completed": map[string]any{"type": "boolean"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}})},
		{"name": "get_task", "description": "Get one of the current user's tasks", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}}, "id")},
		{"name": "create_task", "description": "Create a task in one of the current user's lists", "inputSchema": objectSchema(map[string]any{"title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "list": map[string]any{"type": "string"}, "due_date": map[string]any{"type": "string", "format": "date-time"}}, "title", "list")},
		{"name": "update_task", "description": "Update one of the current user's tasks", "inputSchema": objectSchema(map[string]any{"id": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "completed": map[string]any{"type": "boolean"}, "due_date": map[string]any{"type": "string", "format": "date-time"}}, "id")},
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
	default:
		return mcpToolResult{}, fmt.Errorf("unknown tool %q", call.Name)
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

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func mcpTaskData(record *core.Record) map[string]any {
	return map[string]any{
		"id": record.Id, "title": record.GetString("title"), "description": record.GetString("description"),
		"list": record.GetString("list"), "completed": record.GetBool("completed"), "due_date": record.GetString("due_date"),
	}
}
