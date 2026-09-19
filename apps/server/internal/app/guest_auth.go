package app

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const guestUserEmail = "guest@taskboard.local"

func ensureGuestUser(app core.App) (*core.Record, error) {
	users, err := app.FindRecordsByFilter("users", "email = {:email}", "", 1, 0, dbx.Params{"email": guestUserEmail})
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		user := users[0]
		if user.GetBool("disabled") || user.GetBool("isAdmin") || !user.Verified() {
			user.Set("disabled", false)
			user.Set("isAdmin", false)
			user.Set("registration_status", "active")
			user.SetVerified(true)
			if err := app.Save(user); err != nil {
				return nil, err
			}
		}
		return user, nil
	}

	collection, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	password, err := newAppPassword()
	if err != nil {
		return nil, err
	}
	user := core.NewRecord(collection)
	user.SetEmail(guestUserEmail)
	user.SetPassword(password)
	user.Set("display_name", "Guest")
	user.Set("disabled", false)
	user.Set("isAdmin", false)
	user.Set("registration_status", "active")
	user.SetVerified(true)
	if err := app.Save(user); err != nil {
		return nil, err
	}
	return user, nil
}
