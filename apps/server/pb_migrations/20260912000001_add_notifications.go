package migrations

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/slopstack/slopstack/apps/server/internal/app"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		return app.EnsureNotificationCollections(txApp)
	}, func(core.App) error {
		// Notifications and reminder schedules can be user-visible state.
		return nil
	}, "20260912000001_add_notifications.go")
}
