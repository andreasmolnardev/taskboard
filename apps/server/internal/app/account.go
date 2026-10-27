package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type accountExport struct {
	FormatVersion int                         `json:"formatVersion"`
	ExportedAt    string                      `json:"exportedAt"`
	Account       map[string]any              `json:"account"`
	Data          map[string][]map[string]any `json:"data"`
}

var accountDataCollections = []struct {
	name  string
	field string
}{
	{"todos", "owner"},
	{"events", "owner"},
	{"lists", "owner"},
	{"calendars", "owner"},
	{"address_books", "owner"},
	{"contacts", "owner"},
	{"notifications", "owner"},
	{"reminders", "owner"},
	{"caldav_changes", "owner"},
	{"app_passwords", "user"},
	{"user_permissions", "user"},
	{"group_members", "user"},
	{"audit_logs", "actor"},
}

func registerAccountRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/account/export", exportAccountRoute)
	e.Router.POST("/api/account/delete", deleteAccountRoute)
}

func exportAccountRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}

	export := accountExport{
		FormatVersion: 1,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Account: map[string]any{
			"email":        user.Email(),
			"display_name": user.GetString("display_name"),
			"created":      user.GetString("created"),
		},
		Data: make(map[string][]map[string]any, len(accountDataCollections)),
	}

	for _, definition := range accountDataCollections {
		records, err := event.App.FindRecordsByFilter(
			definition.name,
			definition.field+" = {:user}",
			"created",
			0,
			0,
			dbx.Params{"user": user.Id},
		)
		if err != nil {
			return event.InternalServerError("could not export account data", err)
		}
		items := make([]map[string]any, 0, len(records))
		for _, record := range records {
			items = append(items, record.PublicExport())
		}
		export.Data[definition.name] = items
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return event.InternalServerError("could not encode account export", err)
	}
	event.Response.Header().Set("Content-Disposition", "attachment; filename=taskboard-account-export.json")
	event.Response.Header().Set("Cache-Control", "private, no-store")
	return event.Blob(http.StatusOK, "application/json; charset=utf-8", data)
}

func deleteAccountRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	var input struct {
		ConfirmEmail string `json:"confirmEmail"`
	}
	if err := event.BindBody(&input); err != nil {
		return event.BadRequestError("invalid request body", err)
	}
	if !strings.EqualFold(strings.TrimSpace(input.ConfirmEmail), strings.TrimSpace(user.Email())) {
		return event.BadRequestError("confirmation email does not match", nil)
	}

	if err := event.App.RunInTransaction(func(txApp core.App) error {
		for _, definition := range accountDataCollections {
			records, err := txApp.FindRecordsByFilter(
				definition.name,
				definition.field+" = {:user}",
				"",
				0,
				0,
				dbx.Params{"user": user.Id},
			)
			if err != nil {
				return err
			}
			for _, record := range records {
				if err := txApp.Delete(record); err != nil {
					return err
				}
			}
		}
		account, err := txApp.FindRecordById("users", user.Id)
		if err != nil {
			return err
		}
		return txApp.Delete(account)
	}); err != nil {
		return event.InternalServerError("could not delete account", err)
	}

	return event.NoContent(http.StatusNoContent)
}
