package app

import "github.com/pocketbase/pocketbase/core"

// EnsureCollections creates the small core schema on first boot and repairs
// required fields that may be missing from an older persisted database.
func EnsureCollections(app core.App) error {
	collections := []struct {
		Name   string
		Auth   bool
		Fields []map[string]any
	}{
		{"users", true, []map[string]any{{"name": "display_name", "type": "text"}, {"name": "disabled", "type": "bool"}, {"name": "isAdmin", "type": "bool"}, {"name": "activation_code", "type": "text"}, {"name": "registration_status", "type": "text"}}},
		{"groups", false, []map[string]any{{"name": "name", "type": "text", "required": true}, {"name": "description", "type": "text"}}},
		{"permissions", false, []map[string]any{{"name": "name", "type": "text", "required": true}, {"name": "category", "type": "text"}}},
		{"group_permissions", false, []map[string]any{{"name": "group", "type": "text", "required": true}, {"name": "permission", "type": "text", "required": true}}},
		{"user_permissions", false, []map[string]any{{"name": "user", "type": "text", "required": true}, {"name": "permission", "type": "text", "required": true}, {"name": "denied", "type": "bool"}}},
		{"group_members", false, []map[string]any{{"name": "group", "type": "text", "required": true}, {"name": "user", "type": "text", "required": true}}},
		{"todos", false, []map[string]any{{"name": "owner", "type": "text", "required": true}, {"name": "title", "type": "text", "required": true}, {"name": "completed", "type": "bool"}, {"name": "due_date", "type": "date"}}},
		{"audit_logs", false, []map[string]any{{"name": "actor", "type": "text"}, {"name": "action", "type": "text", "required": true}, {"name": "metadata", "type": "json"}}},
	}

	for _, definition := range collections {
		collection, err := app.FindCollectionByNameOrId(definition.Name)
		if err == nil {
			if definition.Name == "users" {
				changed := false
				if collection.Fields.GetByName("isAdmin") == nil {
					collection.Fields.Add(&core.BoolField{Name: "isAdmin"})
					changed = true
				}
				if collection.Fields.GetByName("activation_code") == nil {
					collection.Fields.Add(&core.TextField{Name: "activation_code"})
					changed = true
				}
				if collection.Fields.GetByName("registration_status") == nil {
					collection.Fields.Add(&core.TextField{Name: "registration_status"})
					changed = true
				}
				if changed {
					if err := app.Save(collection); err != nil {
						return err
					}
				}
			}
			continue
		}
		collection = core.NewBaseCollection(definition.Name)
		if definition.Auth {
			collection = core.NewAuthCollection(definition.Name)
		}
		addFields(collection, definition.Fields)
		if err := app.Save(collection); err != nil {
			return err
		}
	}
	return nil
}

func addFields(collection *core.Collection, definitions []map[string]any) {
	for _, definition := range definitions {
		name, _ := definition["name"].(string)
		required, _ := definition["required"].(bool)
		switch definition["type"] {
		case "text":
			collection.Fields.Add(&core.TextField{Name: name, Required: required})
		case "bool":
			collection.Fields.Add(&core.BoolField{Name: name, Required: required})
		case "date":
			collection.Fields.Add(&core.DateField{Name: name, Required: required})
		case "json":
			collection.Fields.Add(&core.JSONField{Name: name, Required: required})
		}
	}
}
