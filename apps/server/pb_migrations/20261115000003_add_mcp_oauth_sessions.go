package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		_, err := txApp.DB().NewQuery(`CREATE TABLE IF NOT EXISTS mcp_oauth_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, client_id TEXT NOT NULL, resource TEXT NOT NULL, refresh_hash TEXT NOT NULL UNIQUE, expires INTEGER NOT NULL, created INTEGER NOT NULL)`).Execute()
		return err
	}, func(txApp core.App) error {
		_, err := txApp.DB().NewQuery(`DROP TABLE IF EXISTS mcp_oauth_sessions`).Execute()
		return err
	}, "20261115000003_add_mcp_oauth_sessions.go")
}
