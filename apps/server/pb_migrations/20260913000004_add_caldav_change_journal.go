package migrations

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/slopstack/slopstack/apps/server/internal/app"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		return app.EnsureCalDAVChanges(txApp)
	}, func(txApp core.App) error {
		return nil
	}, "20260913000004_add_caldav_change_journal.go")
}
