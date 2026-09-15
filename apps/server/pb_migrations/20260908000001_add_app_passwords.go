package migrations

import (
	"github.com/pocketbase/pocketbase/core"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		if _, err := txApp.FindCollectionByNameOrId("app_passwords"); err == nil {
			return nil
		}
		collection := core.NewBaseCollection("app_passwords")
		collection.ListRule = nil
		collection.ViewRule = nil
		collection.CreateRule = nil
		collection.UpdateRule = nil
		collection.DeleteRule = nil
		collection.Fields.Add(&core.TextField{Name: "user", Required: true})
		collection.Fields.Add(&core.TextField{Name: "name"})
		collection.Fields.Add(&core.TextField{Name: "secret_hash", Required: true, Hidden: true})
		return txApp.Save(collection)
	}, func(core.App) error {
		// Do not delete credentials when a developer rolls migrations back.
		return nil
	}, "20260908000001_add_app_passwords.go")
}
