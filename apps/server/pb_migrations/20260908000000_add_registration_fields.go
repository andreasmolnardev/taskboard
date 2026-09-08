package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		users, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		changed := false
		if users.Fields.GetByName("activation_code") == nil {
			users.Fields.Add(&core.TextField{Name: "activation_code"})
			changed = true
		}
		if users.Fields.GetByName("registration_status") == nil {
			users.Fields.Add(&core.TextField{Name: "registration_status"})
			changed = true
		}
		if changed {
			return txApp.Save(users)
		}
		return nil
	}, func(txApp core.App) error {
		users, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if users.Fields.GetByName("activation_code") != nil {
			users.Fields.RemoveByName("activation_code")
		}
		if users.Fields.GetByName("registration_status") != nil {
			users.Fields.RemoveByName("registration_status")
		}
		return txApp.Save(users)
	}, "20260908000000_add_registration_fields.go")
}
