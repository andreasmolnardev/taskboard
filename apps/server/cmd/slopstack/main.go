package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/pocketbase/pocketbase"
	"github.com/slopstack/slopstack/apps/server/internal/app"
	_ "github.com/slopstack/slopstack/apps/server/pb_migrations"
)

func main() {
	config := app.LoadConfig()
	pb := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: config.StoragePath, DefaultDev: config.Development})
	pb.RootCmd.Use = "slopstack"
	pb.RootCmd.Short = "Slopstack application server"
	if flag := pb.RootCmd.PersistentFlags().Lookup("http"); flag != nil {
		flag.DefValue = config.HTTPAddress
		_ = pb.RootCmd.PersistentFlags().Set("http", config.HTTPAddress)
	}

	api := newOpenAPI(config)
	app.Register(pb, config, api)
	registerExportCommand(pb, api)

	if err := pb.Start(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func newOpenAPI(config app.Config) huma.API {
	api := humago.New(http.NewServeMux(), huma.DefaultConfig("Slopstack API", "0.1.0"))
	api.OpenAPI().OpenAPI = "3.1.0"
	api.OpenAPI().Servers = []*huma.Server{{URL: config.PublicURL}}
	api.OpenAPI().Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"pocketbaseAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}
	return api
}

func registerExportCommand(pb *pocketbase.PocketBase, api huma.API) {
	pb.RootCmd.AddCommand((&app.OpenAPIExportCommand{Run: func() error {
		data, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll("openapi", 0o755); err != nil {
			return err
		}
		path := filepath.Join("openapi", "openapi.json")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	}}).Command())
}
