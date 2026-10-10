package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		for _, name := range []string{"events", "todos"} {
			collection, err := txApp.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if collection.Fields.GetByName("dav_data") == nil {
				collection.Fields.Add(&core.TextField{Name: "dav_data"})
				if err := txApp.Save(collection); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(core.App) error {
		// DAV resource data is synchronization state; do not remove it on rollback.
		return nil
	}, "20260920000000_add_dav_resource_data.go")
}
