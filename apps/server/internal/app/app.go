package app

import (
	"context"
	"crypto/rand"
	"log"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func Register(pb *pocketbase.PocketBase, config Config, api huma.API) {
	pb.RootCmd.PersistentFlags().StringVar(&config.HTTPAddress, "http", config.HTTPAddress, "HTTP address")
	pb.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := EnsureCollections(e.App); err != nil {
			return err
		}
		return e.App.RunAppMigrations()
	})
	protectUserAdminFlag(pb, config)

	pb.OnServe().BindFunc(func(e *core.ServeEvent) error {
		registerRoutes(e, config, api)
		return e.Next()
	})
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
	registerTodoAPI(api)
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
