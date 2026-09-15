package migrations

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/slopstack/slopstack/apps/server/internal/app"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		return app.EnsureCollections(txApp)
	}, func(txApp core.App) error {
		return nil
	}, "20260912000000_add_entry_containers.go")
}
