package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("todos")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("checklist") != nil {
			return nil
		}
		collection.Fields.Add(&core.JSONField{Name: "checklist"})
		return txApp.Save(collection)
	}, func(core.App) error {
		return nil
	}, "20261115000002_add_task_checklists.go")
}
