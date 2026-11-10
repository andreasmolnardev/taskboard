package app

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestWebShellAndServiceWorkerServing(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "<html>Taskboard</html>", "sw.js": "self.addEventListener('fetch', () => {});", "manifest.webmanifest": `{"name":"Taskboard"}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	server := newHTTPIntegrationServer(t, Config{WebPath: dir})
	for _, route := range []string{"/", "/calendar", "/auth/login", "/sw.js", "/manifest.webmanifest"} {
		status, headers, body := integrationRequest(t, server, http.MethodGet, route, "", nil)
		if status != http.StatusOK || body == "" {
			t.Fatalf("GET %s = %d: %s", route, status, body)
		}
		if headers.Get("Cache-Control") != "no-cache" {
			t.Fatalf("GET %s must allow update checks: %v", route, headers)
		}
	}
	for _, route := range []string{"/api/missing", "/assets/missing.js", "/caldav/missing"} {
		status, _, body := integrationRequest(t, server, http.MethodGet, route, "", nil)
		if status == http.StatusOK || body == "<html>Taskboard</html>" {
			t.Fatalf("GET %s incorrectly served the app shell", route)
		}
	}
}
