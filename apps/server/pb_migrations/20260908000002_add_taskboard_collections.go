package migrations

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/slopstack/slopstack/apps/server/internal/app"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		return app.EnsureCollections(txApp)
	}, func(core.App) error {
		// Collections may contain user data; rollback must not destroy it.
		return nil
	}, "20260908000002_add_taskboard_collections.go")
}
