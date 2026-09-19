package app

import "github.com/pocketbase/pocketbase/core"

// EnsureCollections creates the small core schema on first boot and repairs
// required fields that may be missing from an older persisted database.
func EnsureCollections(app core.App) error {
	collections := []struct {
		Name   string
		Auth   bool
		Fields []map[string]any
		Rule   string
	}{
		{"users", true, []map[string]any{{"name": "display_name", "type": "text"}, {"name": "disabled", "type": "bool"}, {"name": "isAdmin", "type": "bool"}, {"name": "activation_code", "type": "text"}, {"name": "registration_status", "type": "text"}}, ""},
		{"app_passwords", false, []map[string]any{{"name": "user", "type": "text", "required": true}, {"name": "name", "type": "text"}, {"name": "secret_hash", "type": "text", "required": true, "hidden": true}}, ""},
		{"groups", false, []map[string]any{{"name": "name", "type": "text", "required": true}, {"name": "description", "type": "text"}}, ""},
		{"permissions", false, []map[string]any{{"name": "name", "type": "text", "required": true}, {"name": "category", "type": "text"}}, ""},
		{"group_permissions", false, []map[string]any{{"name": "group", "type": "text", "required": true}, {"name": "permission", "type": "text", "required": true}}, ""},
		{"user_permissions", false, []map[string]any{{"name": "user", "type": "text", "required": true}, {"name": "permission", "type": "text", "required": true}, {"name": "denied", "type": "bool"}}, ""},
		{"group_members", false, []map[string]any{{"name": "group", "type": "text", "required": true}, {"name": "user", "type": "text", "required": true}}, ""},
		{"todos", false, []map[string]any{{"name": "owner", "type": "text", "required": true}, {"name": "title", "type": "text", "required": true}, {"name": "description", "type": "text"}, {"name": "uid", "type": "text"}, {"name": "dtstamp", "type": "date"}, {"name": "start_date", "type": "date"}, {"name": "start_local", "type": "text"}, {"name": "list", "type": "text"}, {"name": "calendar", "type": "text"}, {"name": "timezone", "type": "text"}, {"name": "time_mode", "type": "text"}, {"name": "completed", "type": "bool"}, {"name": "due_date", "type": "date"}, {"name": "due_local", "type": "text"}, {"name": "duration", "type": "text"}, {"name": "status", "type": "text"}, {"name": "completed_at", "type": "date"}, {"name": "percent_complete", "type": "text"}, {"name": "priority", "type": "text"}, {"name": "categories", "type": "text"}, {"name": "rrule", "type": "text"}, {"name": "exdate", "type": "text"}, {"name": "organizer", "type": "text"}, {"name": "attendees", "type": "text"}, {"name": "url", "type": "text"}, {"name": "class", "type": "text"}, {"name": "geo", "type": "text"}, {"name": "alarm", "type": "text"}, {"name": "dav_href", "type": "text"}}, "@request.auth.id = owner"},
		{"events", false, []map[string]any{{"name": "owner", "type": "text", "required": true}, {"name": "title", "type": "text", "required": true}, {"name": "description", "type": "text"}, {"name": "uid", "type": "text"}, {"name": "dtstamp", "type": "date"}, {"name": "start_date", "type": "date", "required": true}, {"name": "start_local", "type": "text"}, {"name": "end_date", "type": "date"}, {"name": "end_local", "type": "text"}, {"name": "duration", "type": "text"}, {"name": "all_day", "type": "bool"}, {"name": "calendar", "type": "text"}, {"name": "timezone", "type": "text"}, {"name": "time_mode", "type": "text"}, {"name": "status", "type": "text"}, {"name": "location", "type": "text"}, {"name": "categories", "type": "text"}, {"name": "rrule", "type": "text"}, {"name": "exdate", "type": "text"}, {"name": "organizer", "type": "text"}, {"name": "attendees", "type": "text"}, {"name": "url", "type": "text"}, {"name": "class", "type": "text"}, {"name": "geo", "type": "text"}, {"name": "alarm", "type": "text"}, {"name": "recurrence_parent", "type": "text"}, {"name": "recurrence_id", "type": "text"}, {"name": "recurrence_cancelled", "type": "bool"}, {"name": "dav_href", "type": "text"}}, "@request.auth.id = owner"},
		{"lists", false, []map[string]any{{"name": "owner", "type": "text", "required": true}, {"name": "name", "type": "text", "required": true}, {"name": "description", "type": "text"}, {"name": "color", "type": "text"}, {"name": "archived", "type": "bool"}, {"name": "is_default", "type": "bool"}}, "@request.auth.id = owner"},
		{"calendars", false, []map[string]any{{"name": "owner", "type": "text", "required": true}, {"name": "name", "type": "text", "required": true}, {"name": "description", "type": "text"}, {"name": "color", "type": "text"}, {"name": "timezone", "type": "text"}, {"name": "archived", "type": "bool"}, {"name": "is_default", "type": "bool"}}, "@request.auth.id = owner"},
		{"address_books", false, []map[string]any{{"name": "owner", "type": "text", "required": true}, {"name": "name", "type": "text", "required": true}, {"name": "description", "type": "text"}, {"name": "color", "type": "text"}}, "@request.auth.id = owner"},
		{"contacts", false, []map[string]any{{"name": "owner", "type": "text", "required": true}, {"name": "address_book", "type": "text", "required": true}, {"name": "uid", "type": "text", "required": true}, {"name": "dav_href", "type": "text"}, {"name": "formatted_name", "type": "text", "required": true}, {"name": "given_name", "type": "text"}, {"name": "family_name", "type": "text"}, {"name": "organization", "type": "text"}, {"name": "title", "type": "text"}, {"name": "email", "type": "text"}, {"name": "phone", "type": "text"}, {"name": "address", "type": "text"}, {"name": "notes", "type": "text"}, {"name": "vcard", "type": "text"}}, "@request.auth.id = owner"},
		{"audit_logs", false, []map[string]any{{"name": "actor", "type": "text"}, {"name": "action", "type": "text", "required": true}, {"name": "metadata", "type": "json"}}, ""},
	}

	for _, definition := range collections {
		collection, err := app.FindCollectionByNameOrId(definition.Name)
		if err == nil {
			changed := addMissingFields(collection, definition.Fields)
			if definition.Rule != "" && collection.ListRule == nil {
				collection.ListRule = &definition.Rule
				collection.ViewRule = &definition.Rule
				collection.CreateRule = &definition.Rule
				collection.UpdateRule = &definition.Rule
				collection.DeleteRule = &definition.Rule
				changed = true
			}
			if changed {
				if err := app.Save(collection); err != nil {
					return err
				}
			}
			continue
		}
		collection = core.NewBaseCollection(definition.Name)
		if definition.Auth {
			collection = core.NewAuthCollection(definition.Name)
		}
		addFields(collection, definition.Fields)
		if definition.Rule != "" {
			collection.ListRule = &definition.Rule
			collection.ViewRule = &definition.Rule
			collection.CreateRule = &definition.Rule
			collection.UpdateRule = &definition.Rule
			collection.DeleteRule = &definition.Rule
		}
		if err := app.Save(collection); err != nil {
			return err
		}
	}
	if err := EnsureCalDAVChanges(app); err != nil {
		return err
	}
	return nil
}

func addMissingFields(collection *core.Collection, definitions []map[string]any) bool {
	changed := false
	for _, definition := range definitions {
		name, _ := definition["name"].(string)
		if collection.Fields.GetByName(name) == nil {
			addFields(collection, []map[string]any{definition})
			changed = true
		}
	}
	return changed
}

func addFields(collection *core.Collection, definitions []map[string]any) {
	for _, definition := range definitions {
		name, _ := definition["name"].(string)
		required, _ := definition["required"].(bool)
		hidden, _ := definition["hidden"].(bool)
		switch definition["type"] {
		case "text":
			collection.Fields.Add(&core.TextField{Name: name, Required: required, Hidden: hidden})
		case "bool":
			collection.Fields.Add(&core.BoolField{Name: name, Required: required})
		case "date":
			collection.Fields.Add(&core.DateField{Name: name, Required: required})
		case "json":
			collection.Fields.Add(&core.JSONField{Name: name, Required: required})
		}
	}
}
