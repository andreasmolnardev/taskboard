package migrations

import (
	"github.com/pocketbase/pocketbase/core"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		if _, err := txApp.DB().NewQuery(`CREATE TABLE IF NOT EXISTS mcp_oauth_clients (id TEXT PRIMARY KEY, redirects TEXT NOT NULL, created TEXT NOT NULL)`).Execute(); err != nil {
			return err
		}
		_, err := txApp.DB().NewQuery(`CREATE TABLE IF NOT EXISTS mcp_oauth_codes (code_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL, client_id TEXT NOT NULL, redirect_uri TEXT NOT NULL, challenge TEXT NOT NULL, resource TEXT NOT NULL, expires INTEGER NOT NULL)`).Execute()
		return err
	}, func(txApp core.App) error {
		if _, err := txApp.DB().NewQuery(`DROP TABLE IF EXISTS mcp_oauth_codes`).Execute(); err != nil {
			return err
		}
		_, err := txApp.DB().NewQuery(`DROP TABLE IF EXISTS mcp_oauth_clients`).Execute()
		return err
	}, "20261110000000_add_mcp_oauth.go")
}
