package app

import (
	"context"

	"github.com/andreasmolnardev/slopstack/go/audit"
	"github.com/pocketbase/pocketbase/core"
)

func registerAuditHooks(app core.App) error {
	return audit.RegisterRequestHooks(app, audit.PocketBaseRecorder{App: app}, audit.RequestHookConfig{
		Collections: []string{
			"users",
			"todos",
			"events",
			"lists",
			"calendars",
			"address_books",
			"contacts",
			"app_passwords",
		},
	})
}

func logAuditMutation(ctx context.Context, app core.App, actorID, action, resource, resourceID, source string) {
	if app == nil || resourceID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	metadata := map[string]any{}
	if source != "" {
		metadata["source"] = source
	}
	err := (audit.PocketBaseRecorder{App: app}).Log(ctx, audit.Event{
		ActorID:    actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Metadata:   metadata,
	})
	if err != nil {
		app.Logger().Error("write audit event", "action", action, "collection", resource, "resource_id", resourceID, "error", err)
	}
}

func logRequestAudit(event *core.RequestEvent, action, resource, resourceID, source string) {
	if event == nil || event.App == nil {
		return
	}
	ctx := context.Background()
	if event.Request != nil {
		ctx = event.Request.Context()
	}
	actorID := ""
	if event.Auth != nil {
		actorID = event.Auth.Id
	}
	logAuditMutation(ctx, event.App, actorID, action, resource, resourceID, source)
}
