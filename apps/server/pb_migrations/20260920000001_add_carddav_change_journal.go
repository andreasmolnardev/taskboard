package migrations

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/andreasmolnardev/taskboard/apps/server/internal/app"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		return app.EnsureCardDAVChanges(txApp)
	}, func(core.App) error {
		// Synchronization history is user data and must survive rollback.
		return nil
	}, "20260920000001_add_carddav_change_journal.go")
}
