package migrations

import (
	"github.com/andreasmolnardev/taskboard/apps/server/internal/app"
	"github.com/pocketbase/pocketbase/core"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		return app.EnsureCalDAVImports(txApp)
	}, func(core.App) error { return nil }, "20261110000001_add_caldav_imports.go")
}
