package migrations

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/slopstack/slopstack/apps/server/internal/app"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		return app.EnsureCollections(txApp)
	}, func(core.App) error {
		// Address books and contacts are user data, not disposable schema.
		return nil
	}, "20260908000004_add_carddav_collections.go")
}
