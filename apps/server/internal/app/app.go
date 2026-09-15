package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func Register(pb *pocketbase.PocketBase, config Config, api huma.API) {
	registerTodoAPI(api)
	pb.RootCmd.PersistentFlags().StringVar(&config.HTTPAddress, "http", config.HTTPAddress, "HTTP address")
	pb.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := e.App.RunAppMigrations(); err != nil {
			return err
		}
		// Repair databases created by an older bootstrap path or a partially
		// applied migration after the migration registry has run.
		if err := EnsureCollections(e.App); err != nil {
			return err
		}
		if err := EnsureNotificationCollections(e.App); err != nil {
			return err
		}
		if err := EnsureCalDAVChanges(e.App); err != nil {
			return err
		}
		return applyBackupConfig(e.App, config)
	})
	protectUserAdminFlag(pb, config)
	registerOwnershipHooks(pb)
	registerCalDAVChangeHooks(pb)
	registerReminderScheduler(pb)
	pb.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if err := provisionDefaultContainers(e.App, e.Record.Id); err != nil {
			return err
		}
		return e.Next()
	})

	pb.OnServe().BindFunc(func(e *core.ServeEvent) error {
		registerRoutes(e, config, api)
		return e.Next()
	})
}

func applyBackupConfig(app core.App, config Config) error {
	settings := app.Settings()
	if !config.BackupEnabled {
		settings.Backups.Cron = ""
		settings.Backups.S3.Enabled = false
		return nil
	}

	settings.Backups.Cron = config.BackupCron
	settings.Backups.CronMaxKeep = config.BackupMaxKeep
	settings.Backups.S3 = core.S3Config{
		Enabled:        config.BackupS3.Enabled,
		Bucket:         config.BackupS3.Bucket,
		Region:         config.BackupS3.Region,
		Endpoint:       config.BackupS3.Endpoint,
		AccessKey:      config.BackupS3.AccessKey,
		Secret:         config.BackupS3.Secret,
		ForcePathStyle: config.BackupS3.ForcePathStyle,
	}
	if err := settings.Backups.Validate(); err != nil {
		return fmt.Errorf("invalid backup configuration: %w", err)
	}
	return nil
}

func provisionDefaultContainers(app core.App, userID string) error {
	defaults := []struct {
		collection string
		name       string
		color      string
	}{
		{"lists", "Tasks", "#87c4a8"},
		{"calendars", "Events and Holidays", "#3b6ea8"},
		{"address_books", "Contacts", "#7657a8"},
	}

	for _, defaultContainer := range defaults {
		records, err := app.FindRecordsByFilter(
			defaultContainer.collection,
			"owner = {:owner} && name = {:name}",
			"",
			1,
			0,
			dbx.Params{"owner": userID, "name": defaultContainer.name},
		)
		if err != nil {
			return err
		}
		if len(records) > 0 {
			continue
		}

		collection, err := app.FindCollectionByNameOrId(defaultContainer.collection)
		if err != nil {
			return err
		}
		record := core.NewRecord(collection)
		record.Set("owner", userID)
		record.Set("name", defaultContainer.name)
		record.Set("color", defaultContainer.color)
		record.Set("is_default", true)
		if err := app.Save(record); err != nil {
			return err
		}
	}
	return nil
}

func activationCode() string {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "000000"
	}
	code := make([]byte, len(bytes))
	for index, value := range bytes {
		code[index] = '0' + value%10
	}
	return string(code)
}

func protectUserAdminFlag(pb *pocketbase.PocketBase, config Config) {
	pb.OnRecordCreateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if config.RegistrationMode == RegistrationDisabled {
			return e.ForbiddenError("registration is disabled", nil)
		}
		e.Record.Set("isAdmin", false)
		e.Record.Set("disabled", true)
		if config.RegistrationMode == RegistrationOTP {
			code := activationCode()
			e.Record.Set("activation_code", code)
			e.Record.Set("registration_status", "awaiting_activation")
			log.Printf("registration activation code email=%s code=%s", e.Record.GetString("email"), code)
		} else {
			e.Record.Set("registration_status", "awaiting_approval")
		}
		return e.Next()
	})
	pb.OnRecordUpdateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			e.Record.Set("isAdmin", e.Record.Original().GetBool("isAdmin"))
		}
		return e.Next()
	})
}

func registerRoutes(e *core.ServeEvent, config Config, api huma.API) {
	e.Router.GET("/openapi.json", func(event *core.RequestEvent) error { return event.JSON(http.StatusOK, api.OpenAPI()) })
	e.Router.GET("/openapi.yaml", func(event *core.RequestEvent) error {
		data, err := api.OpenAPI().YAML()
		if err != nil {
			return err
		}
		return event.Blob(http.StatusOK, "application/yaml", data)
	})
	e.Router.GET("/docs", func(event *core.RequestEvent) error {
		return event.HTML(http.StatusOK, "<!doctype html><html><body><redoc spec-url=\"/openapi.json\"></redoc><script src=\"https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js\"></script></body></html>")
	})
	registerAuthRoutes(e, config)
	registerAppPasswordRoutes(e)
	registerCalendarIORoutes(e)
	registerOccurrenceRoutes(e)
	registerNotificationRoutes(e)
	registerCalDAVRoutes(e)
	registerCardDAVRoutes(e)
	registerSearchRoutes(e)
	registerTodoRoutes(e, config)
}

func registerAuthRoutes(e *core.ServeEvent, config Config) {
	e.Router.GET("/api/auth/registration-policy", func(event *core.RequestEvent) error {
		return event.JSON(http.StatusOK, map[string]string{"mode": string(config.RegistrationMode)})
	})
}

func registerTodoAPI(api huma.API) {
	type todo struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Completed bool   `json:"completed"`
	}
	huma.Register(api, huma.Operation{OperationID: "listTodos", Method: http.MethodGet, Path: "/api/todos", Tags: []string{"Todos"}, Summary: "List the current user's todos", Security: []map[string][]string{{"pocketbaseAuth": {}}}}, func(_ context.Context, _ *struct{}) (*struct{ Body []todo }, error) { return nil, nil })
}

func registerTodoRoutes(e *core.ServeEvent, config Config) {
	e.Router.GET("/api/todos", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		return event.JSON(http.StatusOK, []any{})
	})
	_ = config
}
