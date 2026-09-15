package app

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const appPasswordCollection = "app_passwords"

// AuthenticateAppPassword checks a CalDAV username and app password. The
// username is the user's email address, like PocketBase password auth.
func AuthenticateAppPassword(app core.App, username, password string) (*core.Record, error) {
	users, err := app.FindRecordsByFilter("users", "email = {:email}", "", 1, 0, dbx.Params{"email": username})
	if err != nil || len(users) == 0 {
		return nil, errors.New("invalid credentials")
	}
	user := users[0]
	if user.GetBool("disabled") {
		return nil, errors.New("invalid credentials")
	}

	passwords, err := app.FindRecordsByFilter(appPasswordCollection, "user = {:user}", "", 0, 0, dbx.Params{"user": user.Id})
	if err != nil {
		return nil, errors.New("invalid credentials")
	}
	for _, token := range passwords {
		if bcrypt.CompareHashAndPassword([]byte(token.GetString("secret_hash")), []byte(password)) == nil {
			return user, nil
		}
	}
	return nil, errors.New("invalid credentials")
}

func registerAppPasswordRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/auth/app-passwords", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		return event.JSON(http.StatusOK, appPasswordList(event.App, event.Auth.Id))
	})

	e.Router.POST("/api/auth/app-passwords", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		var input struct {
			Name string `json:"name"`
		}
		if err := event.BindBody(&input); err != nil {
			return event.BadRequestError("invalid request body", err)
		}
		secret, err := newAppPassword()
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		collection, err := event.App.FindCollectionByNameOrId(appPasswordCollection)
		if err != nil {
			return err
		}
		record := core.NewRecord(collection)
		record.Set("user", event.Auth.Id)
		record.Set("name", strings.TrimSpace(input.Name))
		record.Set("secret_hash", string(hash))
		if err := event.App.Save(record); err != nil {
			return err
		}
		return event.JSON(http.StatusCreated, map[string]any{"id": record.Id, "name": record.GetString("name"), "secret": secret})
	})

	e.Router.DELETE("/api/auth/app-passwords/{id}", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		record, err := event.App.FindRecordById(appPasswordCollection, event.Request.PathValue("id"))
		if err != nil || record.GetString("user") != event.Auth.Id {
			return event.NotFoundError("app password not found", nil)
		}
		if err := event.App.Delete(record); err != nil {
			return err
		}
		return event.NoContent(http.StatusNoContent)
	})
}

func appPasswordList(app core.App, userID string) []map[string]any {
	records, err := app.FindRecordsByFilter(appPasswordCollection, "user = {:user}", "-created", 0, 0, dbx.Params{"user": userID})
	if err != nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		result = append(result, map[string]any{"id": record.Id, "name": record.GetString("name"), "created": record.GetDateTime("created")})
	}
	return result
}

func newAppPassword() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate app password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
