package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("todos")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("calendar") == nil {
			collection.Fields.Add(&core.TextField{Name: "calendar"})
			return txApp.Save(collection)
		}
		return nil
	}, func(core.App) error {
		return nil
	}, "20260915000000_add_todo_calendar.go")
}
