package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type httpIntegrationServer struct {
	app    *pocketbase.PocketBase
	server *httptest.Server
	client *http.Client
}

func newHTTPIntegrationServer(t *testing.T) *httpIntegrationServer {
	t.Helper()

	pb := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:       t.TempDir(),
		DefaultDev:           false,
		DefaultEncryptionEnv: "pb_test_env",
	})
	api := humago.New(http.NewServeMux(), huma.DefaultConfig("Taskboard test API", "test"))
	Register(pb, Config{RegistrationMode: RegistrationApproval}, api)
	if err := pb.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	pbRouter, err := apis.NewRouter(pb)
	if err != nil {
		pb.ResetBootstrapState()
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{
		App:    pb,
		Router: pbRouter,
		Server: &http.Server{},
	}
	if err := pb.OnServe().Trigger(serveEvent, func(*core.ServeEvent) error { return nil }); err != nil {
		pb.ResetBootstrapState()
		t.Fatal(err)
	}
	handler, err := pbRouter.BuildMux()
	if err != nil {
		pb.ResetBootstrapState()
		t.Fatal(err)
	}

	server := httptest.NewServer(handler)
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	t.Cleanup(func() {
		server.Close()
		pb.ResetBootstrapState()
	})
	return &httpIntegrationServer{app: pb, server: server, client: client}
}

func integrationUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(collection)
	user.SetEmail(email)
	user.SetPassword("password123456")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	return user
}

func ownedDefault(t *testing.T, app core.App, collection, owner, name string) *core.Record {
	t.Helper()
	records, err := app.FindRecordsByFilter(collection, "owner = {:owner} && name = {:name}", "", 1, 0, dbx.Params{
		"owner": owner,
		"name":  name,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one %s named %q for %s, got %d", collection, name, owner, len(records))
	}
	return records[0]
}

func integrationRequest(t *testing.T, server *httpIntegrationServer, method, path, body string, headers http.Header) (int, http.Header, string) {
	t.Helper()
	request, err := http.NewRequest(method, server.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := server.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, string(data)
}

func bearerHeaders(token string) http.Header {
	return http.Header{"Authorization": []string{"Bearer " + token}}
}

func basicHeaders(email, password string) http.Header {
	value := base64.StdEncoding.EncodeToString([]byte(email + ":" + password))
	return http.Header{"Authorization": []string{"Basic " + value}}
}

func authToken(t *testing.T, server *httpIntegrationServer, email string) string {
	t.Helper()
	body := fmt.Sprintf(`{"identity":%q,"password":"password123456"}`, email)
	status, _, responseBody := integrationRequest(t, server, http.MethodPost, "/api/collections/users/auth-with-password", body, http.Header{"Content-Type": []string{"application/json"}})
	if status != http.StatusOK {
		t.Fatalf("auth status = %d, body = %s", status, responseBody)
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	if response.Token == "" {
		t.Fatal("auth response did not contain a token")
	}
	return response.Token
}

func appPassword(t *testing.T, server *httpIntegrationServer, token, name string) string {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q}`, name)
	status, _, responseBody := integrationRequest(t, server, http.MethodPost, "/api/auth/app-passwords", body, withContentType(bearerHeaders(token)))
	if status != http.StatusCreated {
		t.Fatalf("app password status = %d, body = %s", status, responseBody)
	}
	var response struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	if response.Secret == "" {
		t.Fatal("app password response did not contain a secret")
	}
	return response.Secret
}

func withContentType(headers http.Header) http.Header {
	headers.Set("Content-Type", "application/json")
	return headers
}

func syncToken(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "<d:sync-token>")
	if start < 0 {
		t.Fatalf("sync response has no token: %s", body)
	}
	start += len("<d:sync-token>")
	end := strings.Index(body[start:], "</d:sync-token>")
	if end < 0 {
		t.Fatalf("sync response has malformed token: %s", body)
	}
	return body[start : start+end]
}

func ctag(body string) string {
	start := strings.Index(body, "<d:getctag>")
	if start < 0 {
		return ""
	}
	start += len("<d:getctag>")
	end := strings.Index(body[start:], "</d:getctag>")
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}

func TestCalDAVHTTPIntegration(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "caldav-http@example.com")
	other := integrationUser(t, server.app, "other-caldav-http@example.com")
	calendar := ownedDefault(t, server.app, "calendars", owner.Id, "Events and Holidays")
	otherCalendar := ownedDefault(t, server.app, "calendars", other.Id, "Events and Holidays")

	ownerToken := authToken(t, server, owner.GetString("email"))
	otherToken := authToken(t, server, other.GetString("email"))
	ownerPassword := appPassword(t, server, ownerToken, "caldav")
	otherPassword := appPassword(t, server, otherToken, "caldav")
	ownerAuth := basicHeaders(owner.GetString("email"), ownerPassword)
	otherAuth := basicHeaders(other.GetString("email"), otherPassword)
	calendarPath := "/caldav/calendars/" + owner.Id + "/" + calendar.Id
	resourcePath := calendarPath + "/client-event.ics"

	status, headers, _ := integrationRequest(t, server, http.MethodGet, "/.well-known/caldav", "", nil)
	if status != http.StatusPermanentRedirect || headers.Get("Location") != "/caldav/" {
		t.Fatalf("CalDAV discovery = %d, Location %q", status, headers.Get("Location"))
	}
	status, headers, _ = integrationRequest(t, server, http.MethodOptions, "/caldav/", "", nil)
	if status != http.StatusNoContent || headers.Get("DAV") != "1, 3, calendar-access" || !strings.Contains(headers.Get("Allow"), "REPORT") {
		t.Fatalf("CalDAV OPTIONS = %d, DAV %q, Allow %q", status, headers.Get("DAV"), headers.Get("Allow"))
	}
	status, headers, _ = integrationRequest(t, server, "PROPFIND", "/caldav/", `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:displayname/></d:prop></d:propfind>`, http.Header{"Depth": []string{"0"}})
	if status != http.StatusUnauthorized || headers.Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthenticated PROPFIND = %d, WWW-Authenticate %q", status, headers.Get("WWW-Authenticate"))
	}

	propfind := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:current-user-principal/><d:displayname/><c:calendar-home-set/></d:prop></d:propfind>`
	status, _, body := integrationRequest(t, server, "PROPFIND", "/caldav/", propfind, mergeHeaders(ownerAuth, http.Header{"Depth": []string{"0"}, "Content-Type": []string{"application/xml"}}))
	if status != http.StatusMultiStatus || !strings.Contains(body, "/caldav/principals/"+owner.Id+"/") || !strings.Contains(body, "/caldav/calendars/"+owner.Id+"/") || strings.Contains(body, other.Id) {
		t.Fatalf("CalDAV root PROPFIND = %d, body = %s", status, body)
	}
	status, _, body = integrationRequest(t, server, "PROPFIND", "/caldav/calendars/"+owner.Id+"/", propfind, mergeHeaders(ownerAuth, http.Header{"Depth": []string{"1"}, "Content-Type": []string{"application/xml"}}))
	if status != http.StatusMultiStatus || !strings.Contains(body, calendar.Id) || strings.Contains(body, otherCalendar.Id) {
		t.Fatalf("CalDAV home PROPFIND = %d, body = %s", status, body)
	}

	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Taskboard//HTTP test\r\nBEGIN:VEVENT\r\nUID:http-event@example.com\r\nDTSTAMP:20260915T120000Z\r\nDTSTART:20260920T090000Z\r\nDTEND:20260920T100000Z\r\nSUMMARY:Initial event\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	status, headers, _ = integrationRequest(t, server, http.MethodPut, resourcePath, ics, mergeHeaders(ownerAuth, http.Header{"Content-Type": []string{"text/calendar"}, "If-None-Match": []string{"*"}}))
	createdETag := headers.Get("ETag")
	if status != http.StatusCreated || createdETag == "" || headers.Get("Location") == "" {
		t.Fatalf("CalDAV PUT create = %d, ETag %q, Location %q", status, createdETag, headers.Get("Location"))
	}
	status, headers, body = integrationRequest(t, server, http.MethodGet, resourcePath, "", ownerAuth)
	if status != http.StatusOK || headers.Get("ETag") != createdETag || !strings.Contains(body, "SUMMARY:Initial event") {
		t.Fatalf("CalDAV GET = %d, ETag %q, body = %s", status, headers.Get("ETag"), body)
	}

	query := `<?xml version="1.0"?><c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:getetag/><c:calendar-data/></d:prop><c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"/></c:comp-filter></c:filter></c:calendar-query>`
	status, _, body = integrationRequest(t, server, "REPORT", calendarPath+"/", query, withContentType(ownerAuth))
	if status != http.StatusMultiStatus || !strings.Contains(body, "client-event.ics") || !strings.Contains(body, createdETag) || !strings.Contains(body, "Initial event") {
		t.Fatalf("CalDAV calendar-query = %d, body = %s", status, body)
	}

	syncRequest := `<?xml version="1.0"?><d:sync-collection xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:sync-token/><d:sync-level>1</d:sync-level><d:prop><d:getetag/><c:calendar-data/></d:prop></d:sync-collection>`
	status, _, body = integrationRequest(t, server, "REPORT", calendarPath+"/", syncRequest, withContentType(ownerAuth))
	if status != http.StatusMultiStatus || !strings.Contains(body, resourcePath) {
		t.Fatalf("CalDAV initial sync = %d, body = %s", status, body)
	}
	createdToken := syncToken(t, body)

	updatedICS := strings.Replace(ics, "Initial event", "Updated event", 1)
	status, headers, _ = integrationRequest(t, server, http.MethodPut, resourcePath, updatedICS, mergeHeaders(ownerAuth, http.Header{"Content-Type": []string{"text/calendar"}, "If-Match": []string{createdETag}}))
	updatedETag := headers.Get("ETag")
	if status != http.StatusNoContent || updatedETag == "" || updatedETag == createdETag {
		t.Fatalf("CalDAV PUT update = %d, old ETag %q, new ETag %q", status, createdETag, updatedETag)
	}
	status, _, _ = integrationRequest(t, server, http.MethodPut, resourcePath, updatedICS, mergeHeaders(ownerAuth, http.Header{"Content-Type": []string{"text/calendar"}, "If-Match": []string{createdETag}}))
	if status != http.StatusPreconditionFailed {
		t.Fatalf("stale CalDAV PUT = %d, want %d", status, http.StatusPreconditionFailed)
	}
	status, _, body = integrationRequest(t, server, "REPORT", calendarPath+"/", syncRequestWithToken(createdToken), withContentType(ownerAuth))
	if status != http.StatusMultiStatus || !strings.Contains(body, updatedETag) || !strings.Contains(body, "Updated event") {
		t.Fatalf("CalDAV changed sync = %d, body = %s", status, body)
	}
	updatedToken := syncToken(t, body)

	status, _, _ = integrationRequest(t, server, http.MethodGet, resourcePath, "", otherAuth)
	if status != http.StatusNotFound {
		t.Fatalf("cross-user CalDAV GET = %d, want %d", status, http.StatusNotFound)
	}
	status, _, _ = integrationRequest(t, server, "PROPFIND", "/caldav/calendars/"+owner.Id+"/", propfind, mergeHeaders(otherAuth, http.Header{"Depth": []string{"1"}, "Content-Type": []string{"application/xml"}}))
	if status != http.StatusNotFound {
		t.Fatalf("cross-user CalDAV PROPFIND = %d, want %d", status, http.StatusNotFound)
	}
	status, _, _ = integrationRequest(t, server, http.MethodDelete, resourcePath, "", mergeHeaders(otherAuth, http.Header{"If-Match": []string{updatedETag}}))
	if status != http.StatusNotFound {
		t.Fatalf("cross-user CalDAV DELETE = %d, want %d", status, http.StatusNotFound)
	}

	status, _, _ = integrationRequest(t, server, http.MethodDelete, resourcePath, "", mergeHeaders(ownerAuth, http.Header{"If-Match": []string{updatedETag}}))
	if status != http.StatusNoContent {
		t.Fatalf("CalDAV DELETE = %d, want %d", status, http.StatusNoContent)
	}
	status, _, body = integrationRequest(t, server, "REPORT", calendarPath+"/", syncRequestWithToken(updatedToken), withContentType(ownerAuth))
	if status != http.StatusMultiStatus || !strings.Contains(body, resourcePath) || !strings.Contains(body, "404 Not Found") {
		t.Fatalf("CalDAV delete sync = %d, body = %s", status, body)
	}
}

func syncRequestWithToken(token string) string {
	return `<?xml version="1.0"?><d:sync-collection xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:sync-token>` + token + `</d:sync-token><d:sync-level>1</d:sync-level><d:prop><d:getetag/><c:calendar-data/></d:prop></d:sync-collection>`
}

func mergeHeaders(base, extra http.Header) http.Header {
	result := base.Clone()
	for key, values := range extra {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func TestCardDAVHTTPIntegration(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "carddav-http@example.com")
	other := integrationUser(t, server.app, "other-carddav-http@example.com")
	book := ownedDefault(t, server.app, "address_books", owner.Id, "Contacts")
	otherBook := ownedDefault(t, server.app, "address_books", other.Id, "Contacts")

	ownerToken := authToken(t, server, owner.GetString("email"))
	otherToken := authToken(t, server, other.GetString("email"))
	ownerPassword := appPassword(t, server, ownerToken, "carddav")
	otherPassword := appPassword(t, server, otherToken, "carddav")
	ownerAuth := basicHeaders(owner.GetString("email"), ownerPassword)
	otherAuth := basicHeaders(other.GetString("email"), otherPassword)
	bookPath := "/carddav/" + book.Id + "/"
	contactPath := bookPath + "jane-doe.vcf"

	status, headers, _ := integrationRequest(t, server, http.MethodGet, "/.well-known/carddav", "", nil)
	if status != http.StatusUnauthorized || headers.Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthenticated CardDAV discovery = %d, WWW-Authenticate %q", status, headers.Get("WWW-Authenticate"))
	}
	status, _, body := integrationRequest(t, server, http.MethodGet, "/.well-known/carddav", "", ownerAuth)
	if status != http.StatusMovedPermanently || !strings.Contains(body, `d:href="/carddav/"`) {
		t.Fatalf("CardDAV discovery = %d, body = %s", status, body)
	}

	status, _, body = integrationRequest(t, server, "PROPFIND", "/carddav/", "", ownerAuth)
	beforeCTag := ctag(body)
	if status != http.StatusMultiStatus || beforeCTag == "" || !strings.Contains(body, bookPath) || strings.Contains(body, otherBook.Id) {
		t.Fatalf("CardDAV PROPFIND = %d, ctag %q, body = %s", status, beforeCTag, body)
	}

	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:jane-doe\r\nFN:Jane Doe\r\nN:Doe;Jane;;;\r\nEMAIL:jane@example.com\r\nEND:VCARD\r\n"
	status, headers, _ = integrationRequest(t, server, http.MethodPut, contactPath, vcard, mergeHeaders(ownerAuth, http.Header{"Content-Type": []string{"text/vcard"}}))
	createdETag := headers.Get("ETag")
	if status != http.StatusCreated || createdETag == "" || headers.Get("Location") == "" {
		t.Fatalf("CardDAV PUT create = %d, ETag %q, Location %q", status, createdETag, headers.Get("Location"))
	}
	status, headers, body = integrationRequest(t, server, http.MethodGet, contactPath, "", ownerAuth)
	if status != http.StatusOK || headers.Get("ETag") != createdETag || !strings.Contains(body, "FN:Jane Doe") {
		t.Fatalf("CardDAV GET = %d, ETag %q, body = %s", status, headers.Get("ETag"), body)
	}
	status, _, body = integrationRequest(t, server, "REPORT", bookPath, "", ownerAuth)
	if status != http.StatusMultiStatus || !strings.Contains(body, "jane-doe.vcf") || !strings.Contains(body, createdETag) {
		t.Fatalf("CardDAV REPORT = %d, body = %s", status, body)
	}

	status, _, _ = integrationRequest(t, server, http.MethodGet, contactPath, "", otherAuth)
	if status != http.StatusNotFound {
		t.Fatalf("cross-user CardDAV GET = %d, want %d", status, http.StatusNotFound)
	}
	status, _, _ = integrationRequest(t, server, http.MethodPut, contactPath, vcard, mergeHeaders(otherAuth, http.Header{"Content-Type": []string{"text/vcard"}}))
	if status != http.StatusNotFound {
		t.Fatalf("cross-user CardDAV PUT = %d, want %d", status, http.StatusNotFound)
	}
	status, _, _ = integrationRequest(t, server, http.MethodDelete, contactPath, "", otherAuth)
	if status != http.StatusNotFound {
		t.Fatalf("cross-user CardDAV DELETE = %d, want %d", status, http.StatusNotFound)
	}

	updatedVCard := strings.Replace(vcard, "Jane Doe", "Jane Q. Doe", 1)
	status, headers, _ = integrationRequest(t, server, http.MethodPut, contactPath, updatedVCard, mergeHeaders(ownerAuth, http.Header{"Content-Type": []string{"text/vcard"}}))
	updatedETag := headers.Get("ETag")
	if status != http.StatusNoContent || updatedETag == "" || updatedETag == createdETag {
		t.Fatalf("CardDAV PUT update = %d, old ETag %q, new ETag %q", status, createdETag, updatedETag)
	}
	status, _, body = integrationRequest(t, server, "PROPFIND", "/carddav/", "", ownerAuth)
	afterCTag := ctag(body)
	if status != http.StatusMultiStatus || afterCTag == "" || afterCTag == beforeCTag || !strings.Contains(body, updatedETag) {
		t.Fatalf("CardDAV changed PROPFIND = %d, ctag %q, body = %s", status, afterCTag, body)
	}

	status, _, _ = integrationRequest(t, server, http.MethodDelete, contactPath, "", ownerAuth)
	if status != http.StatusNoContent {
		t.Fatalf("CardDAV DELETE = %d, want %d", status, http.StatusNoContent)
	}
	status, _, body = integrationRequest(t, server, http.MethodGet, contactPath, "", ownerAuth)
	if status != http.StatusNotFound {
		t.Fatalf("deleted CardDAV GET = %d, want %d", status, http.StatusNotFound)
	}
}

func TestPocketBaseHTTPAuthorizationRules(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	owner := integrationUser(t, server.app, "rules-owner@example.com")
	other := integrationUser(t, server.app, "rules-other@example.com")
	ownerCalendar := ownedDefault(t, server.app, "calendars", owner.Id, "Events and Holidays")

	ownerToken := authToken(t, server, owner.GetString("email"))
	otherToken := authToken(t, server, other.GetString("email"))

	status, _, _ := integrationRequest(t, server, http.MethodGet, "/api/todos", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated custom API = %d, want %d", status, http.StatusUnauthorized)
	}
	status, _, _ = integrationRequest(t, server, http.MethodGet, "/api/todos", "", bearerHeaders(ownerToken))
	if status != http.StatusOK {
		t.Fatalf("authenticated custom API = %d, want %d", status, http.StatusOK)
	}

	calendarURL := "/api/collections/calendars/records/" + ownerCalendar.Id
	status, _, _ = integrationRequest(t, server, http.MethodGet, calendarURL, "", bearerHeaders(ownerToken))
	if status != http.StatusOK {
		t.Fatalf("owner calendar view = %d, want %d", status, http.StatusOK)
	}
	status, _, _ = integrationRequest(t, server, http.MethodGet, calendarURL, "", bearerHeaders(otherToken))
	if status != http.StatusNotFound {
		t.Fatalf("cross-user calendar view = %d, want %d", status, http.StatusNotFound)
	}
	status, _, body := integrationRequest(t, server, http.MethodGet, "/api/collections/calendars/records?filter="+url.QueryEscape("id='"+ownerCalendar.Id+"'"), "", bearerHeaders(otherToken))
	if status != http.StatusOK || strings.Contains(body, ownerCalendar.Id) {
		t.Fatalf("cross-user calendar list = %d, body = %s", status, body)
	}

	validCalendar := `{"owner":"` + other.Id + `","name":"Other calendar"}`
	status, _, body = integrationRequest(t, server, http.MethodPost, "/api/collections/calendars/records", validCalendar, withContentType(bearerHeaders(otherToken)))
	if status != http.StatusOK {
		t.Fatalf("same-owner calendar create = %d, body = %s", status, body)
	}
	var createdCalendar struct {
		ID    string `json:"id"`
		Owner string `json:"owner"`
	}
	if err := json.Unmarshal([]byte(body), &createdCalendar); err != nil {
		t.Fatal(err)
	}
	if createdCalendar.ID == "" || createdCalendar.Owner != other.Id {
		t.Fatalf("created calendar = %#v", createdCalendar)
	}

	spoofedCalendar := `{"owner":"` + owner.Id + `","name":"Spoofed owner"}`
	status, _, body = integrationRequest(t, server, http.MethodPost, "/api/collections/calendars/records", spoofedCalendar, withContentType(bearerHeaders(otherToken)))
	if status != http.StatusBadRequest {
		t.Fatalf("spoofed calendar create = %d, body = %s", status, body)
	}

	foreignEvent := `{"owner":"` + other.Id + `","calendar":"` + ownerCalendar.Id + `","title":"Foreign","start_date":"2026-09-20 09:00:00.000Z"}`
	status, _, body = integrationRequest(t, server, http.MethodPost, "/api/collections/events/records", foreignEvent, withContentType(bearerHeaders(otherToken)))
	if status != http.StatusBadRequest {
		t.Fatalf("foreign-container event create = %d, body = %s", status, body)
	}
	status, _, _ = integrationRequest(t, server, http.MethodPatch, calendarURL, `{"name":"No access"}`, withContentType(bearerHeaders(otherToken)))
	if status != http.StatusNotFound {
		t.Fatalf("cross-user calendar update = %d, want %d", status, http.StatusNotFound)
	}
	status, _, _ = integrationRequest(t, server, http.MethodDelete, calendarURL, "", bearerHeaders(otherToken))
	if status != http.StatusNotFound {
		t.Fatalf("cross-user calendar delete = %d, want %d", status, http.StatusNotFound)
	}

}
