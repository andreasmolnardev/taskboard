package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		contacts, err := txApp.FindCollectionByNameOrId("contacts")
		if err != nil {
			return err
		}
		if contacts.Fields.GetByName("dav_href") == nil {
			contacts.Fields.Add(&core.TextField{Name: "dav_href"})
			return txApp.Save(contacts)
		}
		return nil
	}, func(core.App) error {
		// Resource names are synchronization state; never remove them implicitly.
		return nil
	}, "20260914000000_add_carddav_resource_hrefs.go")
}
