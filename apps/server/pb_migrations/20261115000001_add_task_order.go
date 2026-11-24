package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("todos")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("sort_order") != nil {
			return nil
		}
		collection.Fields.Add(&core.NumberField{Name: "sort_order"})
		return txApp.Save(collection)
	}, func(core.App) error {
		return nil
	}, "20261115000001_add_task_order.go")
}
