package migrations

import "github.com/pocketbase/pocketbase/core"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		_, err := txApp.DB().NewQuery(`CREATE TABLE IF NOT EXISTS taskboard_api_tokens (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL, token_hash TEXT NOT NULL UNIQUE, created TEXT NOT NULL)`).Execute()
		return err
	}, func(txApp core.App) error {
		_, err := txApp.DB().NewQuery(`DROP TABLE IF EXISTS taskboard_api_tokens`).Execute()
		return err
	}, "20261115000004_add_api_tokens.go")
}
