package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		users, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if users.Fields.GetByName("isAdmin") == nil {
			users.Fields.Add(&core.BoolField{Name: "isAdmin"})
			return txApp.Save(users)
		}
		return nil
	}, func(txApp core.App) error {
		users, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if users.Fields.GetByName("isAdmin") != nil {
			users.Fields.RemoveByName("isAdmin")
			return txApp.Save(users)
		}
		return nil
	}, "20260824000000_add_user_is_admin.go")
}
