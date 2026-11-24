package app

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// The production image puts the public Vite build in WebPath.
func registerWebRoutes(e *core.ServeEvent, config Config) {
	if config.WebPath == "" {
		return
	}
	files := os.DirFS(config.WebPath)
	if _, err := fs.Stat(files, "index.html"); err != nil {
		return
	}
	e.Router.Any("/{path...}", func(event *core.RequestEvent) error {
		if event.Request.Method != http.MethodGet && event.Request.Method != http.MethodHead {
			return event.NotFoundError("not found", nil)
		}
		name := strings.TrimPrefix(event.Request.URL.Path, "/")
		for _, prefix := range []string{"api", "_", "caldav", "carddav"} {
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				return event.NotFoundError("not found", nil)
			}
		}
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) {
			return event.NotFoundError("not found", nil)
		}
		info, err := fs.Stat(files, name)
		if err != nil || info.IsDir() {
			// Missing assets must not receive an HTML response.
			if path.Ext(name) != "" {
				return event.NotFoundError("not found", nil)
			}
			name = "index.html"
		}
		event.Response.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "assets/") {
			event.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		return event.FileFS(files, name)
	})
}
