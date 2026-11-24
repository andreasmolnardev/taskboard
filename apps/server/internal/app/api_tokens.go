package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const apiTokenTable = "taskboard_api_tokens"

func registerAPITokenRoutes(e *core.ServeEvent) error {
	e.Router.BindFunc(func(event *core.RequestEvent) error {
		if event.Auth == nil {
			parts := strings.Fields(event.Request.Header.Get("Authorization"))
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && strings.HasPrefix(parts[1], "tb_") {
				var token struct {
					UserID string `db:"user_id"`
				}
				if err := event.App.DB().NewQuery(`SELECT user_id FROM ` + apiTokenTable + ` WHERE token_hash={:hash}`).Bind(dbx.Params{"hash": hashAPIToken(parts[1])}).One(&token); err == nil {
					user, err := event.App.FindRecordById("users", token.UserID)
					if err != nil || user.GetBool("disabled") {
						return event.UnauthorizedError("invalid API token", err)
					}
					event.Auth = user
				}
			}
		}
		return event.Next()
	})
	e.Router.GET("/api/auth/api-tokens", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		var rows []struct {
			ID      string `db:"id" json:"id"`
			Name    string `db:"name" json:"name"`
			Created string `db:"created" json:"created"`
		}
		if err := event.App.DB().NewQuery(`SELECT id,name,created FROM ` + apiTokenTable + ` WHERE user_id={:user} ORDER BY created DESC`).Bind(dbx.Params{"user": event.Auth.Id}).All(&rows); err != nil {
			return event.InternalServerError("could not list API tokens", err)
		}
		if rows == nil {
			rows = []struct {
				ID      string `db:"id" json:"id"`
				Name    string `db:"name" json:"name"`
				Created string `db:"created" json:"created"`
			}{}
		}
		return event.JSON(http.StatusOK, rows)
	})
	e.Router.POST("/api/auth/api-tokens", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		var input struct {
			Name string `json:"name"`
		}
		if err := event.BindBody(&input); err != nil || strings.TrimSpace(input.Name) == "" {
			return event.BadRequestError("token name is required", err)
		}
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return event.InternalServerError("could not create API token", err)
		}
		secret := "tb_" + base64.RawURLEncoding.EncodeToString(bytes)
		idBytes := make([]byte, 12)
		if _, err := rand.Read(idBytes); err != nil {
			return event.InternalServerError("could not create API token", err)
		}
		id := hex.EncodeToString(idBytes)
		created := time.Now().UTC().Format(time.RFC3339)
		_, err := event.App.DB().NewQuery(`INSERT INTO ` + apiTokenTable + ` (id,user_id,name,token_hash,created) VALUES ({:id},{:user},{:name},{:hash},{:created})`).Bind(dbx.Params{"id": id, "user": event.Auth.Id, "name": strings.TrimSpace(input.Name), "hash": hashAPIToken(secret), "created": created}).Execute()
		if err != nil {
			return event.InternalServerError("could not create API token", err)
		}
		return event.JSON(http.StatusCreated, map[string]any{"id": id, "name": strings.TrimSpace(input.Name), "token": secret, "created": created})
	})
	e.Router.DELETE("/api/auth/api-tokens/{id}", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		result, err := event.App.DB().NewQuery(`DELETE FROM ` + apiTokenTable + ` WHERE id={:id} AND user_id={:user}`).Bind(dbx.Params{"id": event.Request.PathValue("id"), "user": event.Auth.Id}).Execute()
		if err != nil {
			return event.InternalServerError("could not revoke API token", err)
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return event.NotFoundError("API token not found", nil)
		}
		return event.NoContent(http.StatusNoContent)
	})
	return nil
}

func hashAPIToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
