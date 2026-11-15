package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("lists")
		if err != nil {
			return err
		}
		changed := false
		fields := []core.Field{
			&core.BoolField{Name: "is_project"},
			&core.SelectField{Name: "project_status", Values: []string{"active", "paused", "completed"}},
			&core.URLField{Name: "home_url"},
			&core.DateField{Name: "target_date"},
		}
		for _, field := range fields {
			if collection.Fields.GetByName(field.GetName()) == nil {
				collection.Fields.Add(field)
				changed = true
			}
		}
		if changed {
			return txApp.Save(collection)
		}
		return nil
	}, func(core.App) error {
		return nil
	}, "20261115000000_add_project_list_fields.go")
}
