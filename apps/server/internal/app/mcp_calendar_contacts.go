package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func callMCPCalendarContactTool(app core.App, owner string, call mcpToolCall) (any, error, bool) {
	var value any
	var err error
	switch call.Name {
	case "list_calendars":
		value, err = listMCPCalendars(app, owner)
	case "list_events":
		value, err = listMCPEvents(app, owner, call.Arguments)
	case "get_event":
		value, err = getMCPEvent(app, owner, call.Arguments)
	case "create_event":
		value, err = createMCPEvent(app, owner, call.Arguments)
	case "update_event":
		value, err = updateMCPEvent(app, owner, call.Arguments)
	case "delete_event":
		value, err = deleteMCPEvent(app, owner, call.Arguments)
	case "list_contacts":
		value, err = listMCPContacts(app, owner, call.Arguments)
	case "get_contact":
		value, err = getMCPContact(app, owner, call.Arguments)
	case "create_contact":
		value, err = createMCPContact(app, owner, call.Arguments)
	case "update_contact":
		value, err = updateMCPContact(app, owner, call.Arguments)
	case "delete_contact":
		value, err = deleteMCPContact(app, owner, call.Arguments)
	default:
		return nil, nil, false
	}
	return value, err, true
}

func listMCPCalendars(app core.App, owner string) ([]map[string]any, error) {
	records, err := app.FindRecordsByFilter("calendars", "owner={:owner} && archived!=true", "name", 100, 0, dbx.Params{"owner": owner})
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		result = append(result, map[string]any{"id": record.Id, "name": record.GetString("name"), "description": record.GetString("description"), "color": record.GetString("color"), "timezone": record.GetString("timezone")})
	}
	return result, nil
}

func listMCPEvents(app core.App, owner string, args map[string]any) ([]map[string]any, error) {
	limit := 50
	if raw, ok := args["limit"].(float64); ok {
		if raw < 1 || raw > 100 || raw != float64(int(raw)) {
			return nil, errors.New("limit must be between 1 and 100")
		}
		limit = int(raw)
	}
	filter := "owner={:owner}"
	params := dbx.Params{"owner": owner}
	if calendar := stringArg(args, "calendar"); calendar != "" {
		if err := validateEntryContainer(app, "events", calendar, owner); err != nil {
			return nil, err
		}
		filter += " && calendar={:calendar}"
		params["calendar"] = calendar
	}
	if search := stringArg(args, "search"); search != "" {
		filter += " && (title ~ {:search} || description ~ {:search} || location ~ {:search})"
		params["search"] = search
	}
	for key, op := range map[string]string{"start_after": ">=", "start_before": "<="} {
		if value := stringArg(args, key); value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return nil, fmt.Errorf("%s must be RFC3339", key)
			}
			filter += " && start_date " + op + " {:" + key + "}"
			params[key] = value
		}
	}
	if upcoming, ok := args["upcoming"].(bool); ok && upcoming {
		filter += " && start_date >= {:now}"
		params["now"] = time.Now().UTC().Format(time.RFC3339)
	}
	records, err := app.FindRecordsByFilter("events", filter, "start_date", limit, 0, params)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		result = append(result, mcpEventData(record))
	}
	return result, nil
}

func getMCPEvent(app core.App, owner string, args map[string]any) (map[string]any, error) {
	record, err := app.FindRecordById("events", stringArg(args, "id"))
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("event not found")
	}
	return mcpEventData(record), nil
}

var mcpEventFields = []string{"title", "description", "start_date", "end_date", "all_day", "timezone", "time_mode", "rrule", "exdate", "location", "status", "alarm"}

func createMCPEvent(app core.App, owner string, args map[string]any) (map[string]any, error) {
	title, calendar, start := strings.TrimSpace(stringArg(args, "title")), stringArg(args, "calendar"), stringArg(args, "start_date")
	if title == "" || start == "" {
		return nil, errors.New("title and start_date are required")
	}
	if err := validateEntryContainer(app, "events", calendar, owner); err != nil {
		return nil, err
	}
	collection, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(collection)
	record.Set("owner", owner)
	record.Set("calendar", calendar)
	record.Set("title", title)
	for _, field := range mcpEventFields {
		if value, ok := args[field]; ok {
			record.Set(field, value)
		}
	}
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpEventData(record), nil
}

func updateMCPEvent(app core.App, owner string, args map[string]any) (map[string]any, error) {
	record, err := app.FindRecordById("events", stringArg(args, "id"))
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("event not found")
	}
	if calendar, ok := args["calendar"].(string); ok {
		if err := validateEntryContainer(app, "events", calendar, owner); err != nil {
			return nil, err
		}
		record.Set("calendar", calendar)
	}
	for _, field := range mcpEventFields {
		if field == "calendar" {
			continue
		}
		if value, ok := args[field]; ok {
			record.Set(field, value)
		}
	}
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpEventData(record), nil
}

func deleteMCPEvent(app core.App, owner string, args map[string]any) (map[string]any, error) {
	if confirmed, _ := args["confirm"].(bool); !confirmed {
		return nil, errors.New("deleting an event requires confirm=true")
	}
	record, err := app.FindRecordById("events", stringArg(args, "id"))
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("event not found")
	}
	data := mcpEventData(record)
	if err := app.Delete(record); err != nil {
		return nil, err
	}
	return data, nil
}

func mcpEventData(record *core.Record) map[string]any {
	data := map[string]any{"id": record.Id, "title": record.GetString("title"), "description": record.GetString("description"), "calendar": record.GetString("calendar")}
	for _, field := range mcpEventFields {
		data[field] = record.Get(field)
	}
	return data
}

func listMCPContacts(app core.App, owner string, args map[string]any) ([]map[string]any, error) {
	limit := 50
	if raw, ok := args["limit"].(float64); ok {
		if raw < 1 || raw > 100 || raw != float64(int(raw)) {
			return nil, errors.New("limit must be between 1 and 100")
		}
		limit = int(raw)
	}
	filter := "owner={:owner}"
	params := dbx.Params{"owner": owner}
	if book := stringArg(args, "address_book"); book != "" {
		if err := validateAddressBook(app, book, owner); err != nil {
			return nil, err
		}
		filter += " && address_book={:book}"
		params["book"] = book
	}
	if search := stringArg(args, "search"); search != "" {
		filter += " && (formatted_name ~ {:search} || email ~ {:search} || organization ~ {:search})"
		params["search"] = search
	}
	records, err := app.FindRecordsByFilter("contacts", filter, "formatted_name", limit, 0, params)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		result = append(result, mcpContactData(record))
	}
	return result, nil
}

func getMCPContact(app core.App, owner string, args map[string]any) (map[string]any, error) {
	record, err := app.FindRecordById("contacts", stringArg(args, "id"))
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("contact not found")
	}
	return mcpContactData(record), nil
}

var mcpContactFields = []string{"formatted_name", "given_name", "family_name", "organization", "title", "email", "phone", "address", "notes"}

func createMCPContact(app core.App, owner string, args map[string]any) (map[string]any, error) {
	book, name := stringArg(args, "address_book"), strings.TrimSpace(stringArg(args, "formatted_name"))
	if name == "" {
		return nil, errors.New("formatted_name is required")
	}
	if err := validateAddressBook(app, book, owner); err != nil {
		return nil, err
	}
	collection, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(collection)
	record.Set("owner", owner)
	record.Set("address_book", book)
	record.Set("uid", fmt.Sprintf("%d-%s", time.Now().UnixNano(), owner))
	for _, field := range mcpContactFields {
		if value, ok := args[field]; ok {
			record.Set(field, value)
		}
	}
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpContactData(record), nil
}

func updateMCPContact(app core.App, owner string, args map[string]any) (map[string]any, error) {
	record, err := app.FindRecordById("contacts", stringArg(args, "id"))
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("contact not found")
	}
	for _, field := range mcpContactFields {
		if value, ok := args[field]; ok {
			record.Set(field, value)
		}
	}
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return mcpContactData(record), nil
}

func deleteMCPContact(app core.App, owner string, args map[string]any) (map[string]any, error) {
	if confirmed, _ := args["confirm"].(bool); !confirmed {
		return nil, errors.New("deleting a contact requires confirm=true")
	}
	record, err := app.FindRecordById("contacts", stringArg(args, "id"))
	if err != nil || record.GetString("owner") != owner {
		return nil, errors.New("contact not found")
	}
	data := mcpContactData(record)
	if err := app.Delete(record); err != nil {
		return nil, err
	}
	return data, nil
}

func mcpContactData(record *core.Record) map[string]any {
	data := map[string]any{"id": record.Id, "address_book": record.GetString("address_book"), "uid": record.GetString("uid")}
	for _, field := range mcpContactFields {
		data[field] = record.Get(field)
	}
	return data
}
