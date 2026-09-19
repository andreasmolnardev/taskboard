package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	calendarengine "github.com/slopstack/slopstack/apps/server/internal/calendar"
)

const (
	calDAVRoot            = "/caldav/"
	maxCalDAVRequestSize  = 10 << 20
	maxCalDAVQueryResults = 500
)

// registerCalDAVRoutes exposes the beta calendar-access protocol surface.
func registerCalDAVRoutes(e *core.ServeEvent) {
	handler := func(event *core.RequestEvent) error {
		if event.Request.URL.Path == "/.well-known/caldav" {
			event.Response.Header().Set("Location", calDAVRoot)
			return event.NoContent(http.StatusPermanentRedirect)
		}
		if event.Request.Method == http.MethodOptions {
			setCalDAVHeaders(event.Response.Header())
			event.Response.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
			return event.NoContent(http.StatusNoContent)
		}
		user, ok := calDAVUser(event)
		if !ok {
			event.Response.Header().Set("WWW-Authenticate", `Basic realm="Taskboard CalDAV", charset="UTF-8"`)
			return event.UnauthorizedError("CalDAV authentication required", nil)
		}
		setCalDAVHeaders(event.Response.Header())
		return serveCalDAV(event, user)
	}
	e.Router.Any("/.well-known/caldav", handler)
	e.Router.Any("/caldav", handler)
	e.Router.Any("/caldav/{path...}", handler)
}

func setCalDAVHeaders(header http.Header) {
	header.Set("DAV", "1, 3, calendar-access")
	header.Set("MS-Author-Via", "DAV")
}

func calDAVUser(event *core.RequestEvent) (*core.Record, bool) {
	auth := strings.TrimSpace(event.Request.Header.Get("Authorization"))
	if len(auth) < 6 || !strings.EqualFold(auth[:6], "Basic ") {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(auth[6:]))
	if err != nil {
		return nil, false
	}
	username, password, found := strings.Cut(string(decoded), ":")
	if !found {
		return nil, false
	}
	user, err := AuthenticateAppPassword(event.App, username, password, requestClientIP(event.Request.RemoteAddr))
	return user, err == nil
}

func serveCalDAV(event *core.RequestEvent, user *core.Record) error {
	switch event.Request.Method {
	case "PROPFIND":
		return calDAVPropfind(event, user)
	case "REPORT":
		return calDAVReport(event, user)
	case http.MethodGet, http.MethodHead:
		return calDAVGet(event, user)
	case http.MethodPut:
		return calDAVPut(event, user)
	case http.MethodDelete:
		return calDAVDelete(event, user)
	default:
		event.Response.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
		return event.NoContent(http.StatusMethodNotAllowed)
	}
}

type calDAVPath struct {
	kind         string
	userID       string
	calendarID   string
	itemKind     string
	recordID     string
	resourceName string
}

func parseCalDAVPath(path string) calDAVPath {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "caldav" {
		return calDAVPath{kind: "root"}
	}
	if len(parts) == 3 && parts[0] == "caldav" && parts[1] == "principals" {
		return calDAVPath{kind: "principal", userID: parts[2]}
	}
	if len(parts) >= 3 && parts[0] == "caldav" && parts[1] == "calendars" {
		result := calDAVPath{kind: "home", userID: parts[2]}
		if len(parts) >= 4 {
			result.kind, result.calendarID = "calendar", parts[3]
		}
		if len(parts) == 5 {
			name, err := url.PathUnescape(parts[4])
			if err != nil || !strings.HasSuffix(name, ".ics") {
				return calDAVPath{}
			}
			stem := strings.TrimSuffix(name, ".ics")
			result.resourceName = name
			result.kind = "resource"
			switch {
			case strings.HasPrefix(stem, "event-"):
				result.itemKind, result.recordID = "events", strings.TrimPrefix(stem, "event-")
			case strings.HasPrefix(stem, "todo-"):
				result.itemKind, result.recordID = "todos", strings.TrimPrefix(stem, "todo-")
			default:
				result.kind = "new-resource"
			}
		}
		if len(parts) > 5 {
			return calDAVPath{}
		}
		return result
	}
	return calDAVPath{}
}

func calDAVPropfind(event *core.RequestEvent, user *core.Record) error {
	depth := event.Request.Header.Get("Depth")
	if depth == "" {
		depth = "infinity"
	}
	if depth != "0" && depth != "1" {
		return event.Blob(http.StatusForbidden, "application/xml; charset=utf-8", []byte(`<?xml version="1.0"?><d:error xmlns:d="DAV:"><d:propfind-finite-depth/></d:error>`))
	}
	request, err := readCalDAVXML(event)
	if err != nil {
		return event.BadRequestError("invalid PROPFIND body", err)
	}
	path := parseCalDAVPath(event.Request.URL.Path)
	if path.kind == "" || (path.userID != "" && path.userID != user.Id) {
		return event.NotFoundError("DAV resource not found", nil)
	}
	responses, err := calDAVProperties(event.App, user, path, depth == "1", request)
	if err != nil {
		return err
	}
	return event.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", calDAVMultistatus(responses))
}

func calDAVProperties(app core.App, user *core.Record, path calDAVPath, children bool, request calDAVXMLRequest) ([]calDAVResponse, error) {
	principalHref := "/caldav/principals/" + user.Id + "/"
	homeHref := "/caldav/calendars/" + user.Id + "/"
	base := map[xml.Name]string{
		davName("current-user-principal"): `<d:href>` + principalHref + `</d:href>`,
		davName("principal-URL"):          `<d:href>` + principalHref + `</d:href>`,
		calDAVName("calendar-home-set"):   `<d:href>` + homeHref + `</d:href>`,
	}
	makeResponse := func(href string, values map[xml.Name]string) calDAVResponse {
		ok, missing := requestedCalDAVProperties(request, values)
		return calDAVResponse{Href: href, OK: ok, NotFound: missing}
	}
	switch path.kind {
	case "root":
		values := cloneCalDAVProperties(base)
		values[davName("displayname")] = xmlText(user.GetString("email"))
		values[davName("resourcetype")] = `<d:collection/>`
		return []calDAVResponse{makeResponse(calDAVRoot, values)}, nil
	case "principal":
		values := cloneCalDAVProperties(base)
		values[davName("displayname")] = xmlText(user.GetString("display_name"))
		values[davName("resourcetype")] = `<d:principal/>`
		return []calDAVResponse{makeResponse(principalHref, values)}, nil
	case "home":
		values := cloneCalDAVProperties(base)
		values[davName("displayname")] = "Calendars"
		values[davName("resourcetype")] = `<d:collection/>`
		responses := []calDAVResponse{makeResponse(homeHref, values)}
		if !children {
			return responses, nil
		}
		calendars, err := ownedCalendars(app, user.Id)
		if err != nil {
			return nil, err
		}
		for _, calendar := range calendars {
			responses = append(responses, makeResponse(calendarHref(user.Id, calendar.Id), calendarProperties(calendar)))
		}
		return responses, nil
	case "calendar":
		calendar, err := ownedCalendar(app, user.Id, path.calendarID)
		if err != nil {
			return nil, err
		}
		properties := calendarProperties(calendar)
		token, tokenErr := currentCalDAVToken(app, user.Id, calendar.Id)
		if tokenErr != nil {
			return nil, tokenErr
		}
		properties[davName("sync-token")] = xmlText(token)
		responses := []calDAVResponse{makeResponse(calendarHref(user.Id, calendar.Id), properties)}
		if !children {
			return responses, nil
		}
		resources, err := calendarResources(app, user.Id, calendar.Id, maxCalDAVQueryResults)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			responses = append(responses, makeResponse(resource.href, resourceProperties(resource)))
		}
		return responses, nil
	case "resource", "new-resource":
		resource, err := findCalDAVResource(app, user.Id, path)
		if err != nil {
			return nil, err
		}
		return []calDAVResponse{makeResponse(resource.href, resourceProperties(resource))}, nil
	default:
		return nil, fmt.Errorf("DAV resource not found")
	}
}

func calendarProperties(calendar *core.Record) map[xml.Name]string {
	return map[xml.Name]string{
		davName("displayname"):                         xmlText(calendar.GetString("name")),
		davName("resourcetype"):                        `<d:collection/><c:calendar/>`,
		davName("owner"):                               `<d:href>/caldav/principals/` + calendar.GetString("owner") + `/</d:href>`,
		davName("current-user-privilege-set"):          `<d:privilege><d:all/></d:privilege>`,
		davName("getcontenttype"):                      "text/calendar",
		calDAVName("supported-calendar-component-set"): `<c:comp name="VEVENT"/><c:comp name="VTODO"/>`,
		appleName("calendar-color"):                    xmlText(calendar.GetString("color")),
		calDAVName("calendar-description"):             xmlText(calendar.GetString("description")),
	}
}

func cloneCalDAVProperties(source map[xml.Name]string) map[xml.Name]string {
	result := make(map[xml.Name]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func calendarHref(userID, calendarID string) string {
	return "/caldav/calendars/" + userID + "/" + calendarID + "/"
}

type calDAVResource struct {
	record *core.Record
	kind   string
	href   string
	data   []byte
	etag   string
}

func resourceProperties(resource calDAVResource) map[xml.Name]string {
	return map[xml.Name]string{
		davName("getetag"):          resource.etag,
		davName("getcontenttype"):   "text/calendar; charset=utf-8",
		davName("getcontentlength"): fmt.Sprintf("%d", len(resource.data)),
		calDAVName("calendar-data"): xmlText(string(resource.data)),
		davName("resourcetype"):     "",
	}
}

func calDAVGet(event *core.RequestEvent, user *core.Record) error {
	path := parseCalDAVPath(event.Request.URL.Path)
	if path.userID != user.Id {
		return event.NotFoundError("calendar resource not found", nil)
	}
	var data []byte
	var etag string
	switch path.kind {
	case "calendar":
		if _, err := ownedCalendar(event.App, user.Id, path.calendarID); err != nil {
			return event.NotFoundError("calendar not found", nil)
		}
		calendar, err := buildCalendarExport(event.App, user.Id, path.calendarID)
		if err != nil {
			return err
		}
		data, err = encodeICalendar(calendar)
		if err != nil {
			return err
		}
		etag = calendarETag(data)
	case "resource", "new-resource":
		resource, err := findCalDAVResource(event.App, user.Id, path)
		if err != nil {
			return event.NotFoundError("calendar resource not found", nil)
		}
		data, etag = resource.data, resource.etag
	default:
		return event.NotFoundError("calendar resource not found", nil)
	}
	event.Response.Header().Set("ETag", etag)
	event.Response.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	if etagListContains(event.Request.Header.Get("If-None-Match"), etag) {
		return event.NoContent(http.StatusNotModified)
	}
	if event.Request.Method == http.MethodHead {
		return event.NoContent(http.StatusOK)
	}
	return event.Blob(http.StatusOK, "text/calendar; charset=utf-8", data)
}

func calDAVResourceComponent(calendar *ical.Calendar) (*ical.Component, error) {
	var supported *ical.Component
	for _, child := range calendar.Children {
		switch child.Name {
		case ical.CompEvent, ical.CompToDo:
			if supported != nil {
				return nil, errors.New("one VEVENT or VTODO component is required")
			}
			supported = child
		case "VTIMEZONE":
			// Time zone definitions accompany a resource but are not resources.
		default:
			return nil, fmt.Errorf("unsupported calendar component %q", child.Name)
		}
	}
	if supported == nil {
		return nil, errors.New("one VEVENT or VTODO component is required")
	}
	return supported, nil
}

type calDAVPutFailure struct {
	status   int
	conflict bool
	message  string
}

func (failure calDAVPutFailure) Error() string { return failure.message }

func calDAVPut(event *core.RequestEvent, user *core.Record) error {
	path := parseCalDAVPath(event.Request.URL.Path)
	if path.userID != user.Id || (path.kind != "resource" && path.kind != "new-resource") {
		return event.NotFoundError("calendar resource not found", nil)
	}
	if _, err := ownedCalendar(event.App, user.Id, path.calendarID); err != nil {
		return event.NotFoundError("calendar not found", nil)
	}
	event.Request.Body = http.MaxBytesReader(event.Response, event.Request.Body, maxCalDAVRequestSize)
	data, err := io.ReadAll(event.Request.Body)
	if err != nil {
		return event.BadRequestError("calendar resource is too large", err)
	}
	calendar, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return event.BadRequestError("invalid calendar resource", err)
	}
	component, err := calDAVResourceComponent(calendar)
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	var collection string
	switch component.Name {
	case ical.CompEvent:
		collection = "events"
	case ical.CompToDo:
		collection = "todos"
	default:
		return event.BadRequestError("VEVENT or VTODO is required", nil)
	}
	uid := strings.TrimSpace(calendarText(component, ical.PropUID))
	if uid == "" {
		return event.BadRequestError("calendar component UID is required", nil)
	}

	requestedHref := path.resourceName
	requestedCreate := path.kind == "new-resource"
	var saved *core.Record
	var existing *core.Record
	var created bool
	err = event.App.RunInTransaction(func(txApp core.App) error {
		created = requestedCreate
		if created && requestedHref != "" {
			matches, findErr := txApp.FindRecordsByFilter(collection, "owner = {:owner} && dav_href = {:href}", "", 1, 0, dbx.Params{"owner": user.Id, "href": requestedHref})
			if findErr != nil {
				return findErr
			}
			if len(matches) == 1 {
				existing, created = matches[0], false
				path.itemKind, path.recordID = collection, existing.Id
			}
		}
		if !created {
			if path.itemKind != collection {
				return calDAVPutFailure{conflict: true, message: "component type does not match resource"}
			}
			existing, err = txApp.FindRecordById(collection, path.recordID)
			if err != nil || existing.GetString("owner") != user.Id || resourceCalendarID(existing, collection) != path.calendarID {
				return calDAVPutFailure{status: http.StatusNotFound, message: "calendar resource not found"}
			}
			if existing.GetString("uid") != uid {
				return calDAVPutFailure{conflict: true, message: "UID cannot change"}
			}
		}
		currentETag := ""
		if existing != nil {
			resource, resourceErr := makeCalDAVResource(existing, collection, user.Id, path.calendarID)
			if resourceErr != nil {
				return resourceErr
			}
			currentETag = resource.etag
		}
		if status := checkCalDAVPreconditions(event.Request.Header, existing != nil, currentETag); status != 0 {
			return calDAVPutFailure{status: status, message: "ETag precondition failed"}
		}
		matches, findErr := txApp.FindRecordsByFilter(collection, "owner = {:owner} && uid = {:uid}", "", 2, 0, dbx.Params{"owner": user.Id, "uid": uid})
		if findErr != nil {
			return findErr
		}
		if created && len(matches) > 0 {
			return calDAVPutFailure{conflict: true, message: "UID already exists"}
		}
		if collection == "events" {
			err = saveImportedEventForDAV(txApp, user.Id, path.calendarID, component, existing, requestedHref)
		} else {
			err = saveImportedTodoForDAV(txApp, user.Id, path.calendarID, component, existing, requestedHref)
		}
		if err != nil {
			return calDAVPutFailure{status: http.StatusBadRequest, message: "invalid calendar component: " + err.Error()}
		}
		if existing != nil {
			saved = existing
			return nil
		}
		records, findErr := txApp.FindRecordsByFilter(collection, "owner = {:owner} && dav_href = {:href}", "", 1, 0, dbx.Params{"owner": user.Id, "href": requestedHref})
		if findErr != nil || len(records) != 1 {
			return fmt.Errorf("saved calendar resource cannot be resolved")
		}
		saved = records[0]
		return nil
	})
	if err != nil {
		var failure calDAVPutFailure
		if errors.As(err, &failure) {
			if failure.conflict {
				return calDAVConflict(event, failure.message)
			}
			if failure.status == http.StatusPreconditionFailed {
				return event.NoContent(failure.status)
			}
			if failure.status == http.StatusNotFound {
				return event.NotFoundError(failure.message, nil)
			}
			return event.BadRequestError(failure.message, nil)
		}
		return err
	}
	resource, err := makeCalDAVResource(saved, collection, user.Id, path.calendarID)
	if err != nil {
		return err
	}
	event.Response.Header().Set("ETag", resource.etag)
	event.Response.Header().Set("Location", resource.href)
	if created {
		return event.NoContent(http.StatusCreated)
	}
	return event.NoContent(http.StatusNoContent)
}

func calDAVDelete(event *core.RequestEvent, user *core.Record) error {
	path := parseCalDAVPath(event.Request.URL.Path)
	if (path.kind != "resource" && path.kind != "new-resource") || path.userID != user.Id {
		return event.NotFoundError("calendar resource not found", nil)
	}
	resource, err := findCalDAVResource(event.App, user.Id, path)
	if err != nil {
		return event.NotFoundError("calendar resource not found", nil)
	}
	if status := checkCalDAVPreconditions(event.Request.Header, true, resource.etag); status != 0 {
		return event.NoContent(status)
	}
	err = event.App.RunInTransaction(func(txApp core.App) error {
		if resource.kind == "events" && resource.record.GetString("recurrence_parent") == "" {
			overrides, findErr := txApp.FindRecordsByFilter("events", "recurrence_parent = {:parent}", "id", 0, 0, dbx.Params{"parent": resource.record.Id})
			if findErr != nil {
				return findErr
			}
			for _, override := range overrides {
				if deleteErr := txApp.Delete(override); deleteErr != nil {
					return deleteErr
				}
			}
		}
		record, findErr := txApp.FindRecordById(resource.kind, resource.record.Id)
		if findErr != nil {
			return findErr
		}
		return txApp.Delete(record)
	})
	if err != nil {
		return err
	}
	return event.NoContent(http.StatusNoContent)
}

func calDAVConflict(event *core.RequestEvent, message string) error {
	return event.Blob(http.StatusConflict, "application/xml; charset=utf-8", []byte(`<?xml version="1.0"?><d:error xmlns:d="DAV:"><d:no-uid-conflict/><d:responsedescription>`+xmlText(message)+`</d:responsedescription></d:error>`))
}

func checkCalDAVPreconditions(header http.Header, exists bool, etag string) int {
	if match := strings.TrimSpace(header.Get("If-Match")); match != "" {
		if !exists || (match != "*" && !etagListContains(match, etag)) {
			return http.StatusPreconditionFailed
		}
	}
	if none := strings.TrimSpace(header.Get("If-None-Match")); none != "" {
		if exists && (none == "*" || etagListContains(none, etag)) {
			return http.StatusPreconditionFailed
		}
	}
	return 0
}

func etagListContains(list, etag string) bool {
	for _, candidate := range strings.Split(list, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}

func calDAVSyncCollection(event *core.RequestEvent, user *core.Record, path calDAVPath, request calDAVXMLRequest) error {
	if request.SyncLevel != "" && request.SyncLevel != "1" {
		return event.BadRequestError("only sync-level 1 is supported", nil)
	}
	token, err := parseCalDAVToken(request.SyncToken)
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	currentToken, err := currentCalDAVToken(event.App, user.Id, path.calendarID)
	if err != nil {
		return err
	}
	responses := make([]calDAVResponse, 0)
	if request.SyncToken == "" {
		resources, err := calendarResources(event.App, user.Id, path.calendarID, maxCalDAVQueryResults+1)
		if err != nil {
			return err
		}
		if len(resources) > maxCalDAVQueryResults {
			return event.Blob(http.StatusInsufficientStorage, "application/xml; charset=utf-8", []byte(`<d:error xmlns:d="DAV:"><d:number-of-matches-within-limits/></d:error>`))
		}
		for _, resource := range resources {
			ok, missing := requestedCalDAVProperties(request, resourceProperties(resource))
			responses = append(responses, calDAVResponse{Href: resource.href, OK: ok, NotFound: missing})
		}
	} else {
		changes, err := calDAVChangesSince(event.App, user.Id, path.calendarID, token, maxCalDAVQueryResults+1)
		if err != nil {
			return err
		}
		if len(changes) > maxCalDAVQueryResults {
			return event.Blob(http.StatusInsufficientStorage, "application/xml; charset=utf-8", []byte(`<d:error xmlns:d="DAV:"><d:number-of-matches-within-limits/></d:error>`))
		}
		latest := make(map[string]*core.Record)
		for _, change := range changes {
			latest[change.GetString("href")] = change
		}
		if len(latest) > maxCalDAVQueryResults {
			return event.Blob(http.StatusInsufficientStorage, "application/xml; charset=utf-8", []byte(`<d:error xmlns:d="DAV:"><d:number-of-matches-within-limits/></d:error>`))
		}
		for href, change := range latest {
			if change.GetBool("deleted") {
				responses = append(responses, calDAVResponse{Href: href, StatusCode: http.StatusNotFound})
				continue
			}
			parsed, parseErr := url.Parse(href)
			if parseErr != nil {
				continue
			}
			resource, resourceErr := findCalDAVResource(event.App, user.Id, parseCalDAVPath(parsed.Path))
			if resourceErr != nil {
				continue
			}
			ok, missing := requestedCalDAVProperties(request, resourceProperties(resource))
			responses = append(responses, calDAVResponse{Href: resource.href, OK: ok, NotFound: missing})
		}
	}
	return event.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", calDAVMultistatusWithToken(responses, currentToken))
}

func calDAVReport(event *core.RequestEvent, user *core.Record) error {
	path := parseCalDAVPath(event.Request.URL.Path)
	if path.kind != "calendar" || path.userID != user.Id {
		return event.NotFoundError("calendar not found", nil)
	}
	if _, err := ownedCalendar(event.App, user.Id, path.calendarID); err != nil {
		return event.NotFoundError("calendar not found", nil)
	}
	request, err := readCalDAVXML(event)
	if err != nil {
		return event.BadRequestError("invalid REPORT body", err)
	}
	var resources []calDAVResource
	switch request.XMLName {
	case xml.Name{Space: davNamespace, Local: "sync-collection"}:
		return calDAVSyncCollection(event, user, path, request)
	case xml.Name{Space: calDAVNamespace, Local: "calendar-multiget"}:
		if len(request.Hrefs) > maxCalDAVQueryResults {
			return event.Blob(http.StatusInsufficientStorage, "application/xml; charset=utf-8", []byte(`<?xml version="1.0"?><d:error xmlns:d="DAV:"><d:number-of-matches-within-limits/></d:error>`))
		}
		for _, href := range request.Hrefs {
			requestedURL, err := url.Parse(href)
			if err != nil {
				continue
			}
			resourcePath := parseCalDAVPath(requestedURL.Path)
			resource, err := findCalDAVResource(event.App, user.Id, resourcePath)
			if err == nil && resourcePath.calendarID == path.calendarID {
				resources = append(resources, resource)
			}
		}
	case xml.Name{Space: calDAVNamespace, Local: "calendar-query"}:
		resources, err = calendarResources(event.App, user.Id, path.calendarID, maxCalDAVQueryResults+1)
		if err != nil {
			return err
		}
		resources = filterCalDAVResources(resources, request.Filter.Components)
		if len(resources) > maxCalDAVQueryResults {
			return event.Blob(http.StatusInsufficientStorage, "application/xml; charset=utf-8", []byte(`<?xml version="1.0"?><d:error xmlns:d="DAV:"><d:number-of-matches-within-limits/></d:error>`))
		}
	default:
		return event.BadRequestError("unsupported calendar REPORT", nil)
	}
	responses := make([]calDAVResponse, 0, len(resources))
	for _, resource := range resources {
		ok, missing := requestedCalDAVProperties(request, resourceProperties(resource))
		responses = append(responses, calDAVResponse{Href: resource.href, OK: ok, NotFound: missing})
	}
	return event.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", calDAVMultistatus(responses))
}

func filterCalDAVResources(resources []calDAVResource, filters []calDAVComponentFilter) []calDAVResource {
	var component *calDAVComponentFilter
	for i := range filters {
		if strings.EqualFold(filters[i].Name, "VCALENDAR") {
			for j := range filters[i].Components {
				component = &filters[i].Components[j]
				break
			}
		}
	}
	if component == nil {
		return resources
	}
	want := strings.ToUpper(component.Name)
	start, startOK := parseCalDAVReportTime(component.TimeRange, true)
	end, endOK := parseCalDAVReportTime(component.TimeRange, false)
	result := make([]calDAVResource, 0, len(resources))
	for _, resource := range resources {
		if (want == "VEVENT" && resource.kind != "events") || (want == "VTODO" && resource.kind != "todos") {
			continue
		}
		if component.TimeRange != nil {
			if resource.kind == "events" && resource.record.GetString("rrule") != "" && startOK && endOK {
				calendarComponent, err := occurrenceComponent(resource.record)
				if err != nil {
					continue
				}
				expanded, err := calendarengine.Expand(calendarComponent, start, end, maxCalDAVQueryResults+1)
				if err != nil || len(expanded) == 0 {
					continue
				}
			} else {
				recordStart, recordEnd := resourceTimeRange(resource)
				if startOK && !recordEnd.After(start) || endOK && !recordStart.Before(end) {
					continue
				}
			}
		}
		result = append(result, resource)
	}
	return result
}

func parseCalDAVReportTime(value *calDAVTimeRange, start bool) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	raw := value.End
	if start {
		raw = value.Start
	}
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("20060102T150405Z", raw)
	return parsed, err == nil
}

func resourceTimeRange(resource calDAVResource) (time.Time, time.Time) {
	if resource.kind == "events" {
		start := resource.record.GetDateTime("start_date").Time()
		end := resource.record.GetDateTime("end_date").Time()
		if end.IsZero() {
			end = start
		}
		return start, end
	}
	start := resource.record.GetDateTime("start_date").Time()
	end := resource.record.GetDateTime("due_date").Time()
	if start.IsZero() {
		start = end
	}
	if end.IsZero() {
		end = start
	}
	return start, end
}

func readCalDAVXML(event *core.RequestEvent) (calDAVXMLRequest, error) {
	event.Request.Body = http.MaxBytesReader(event.Response, event.Request.Body, maxCalDAVRequestSize)
	data, err := io.ReadAll(event.Request.Body)
	if err != nil {
		return calDAVXMLRequest{}, err
	}
	return parseCalDAVXML(string(data))
}

func ownedCalendars(app core.App, userID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("calendars", "owner = {:owner}", "name", 0, 0, dbx.Params{"owner": userID})
}

func ownedCalendar(app core.App, userID, calendarID string) (*core.Record, error) {
	calendar, err := app.FindRecordById("calendars", calendarID)
	if err != nil || calendar.GetString("owner") != userID {
		return nil, fmt.Errorf("calendar not found")
	}
	return calendar, nil
}

func calendarResources(app core.App, userID, calendarID string, limit int) ([]calDAVResource, error) {
	result := make([]calDAVResource, 0, limit)
	for _, definition := range []struct{ collection, field string }{{"events", "calendar"}, {"todos", "calendar"}} {
		remaining := limit - len(result)
		if remaining <= 0 {
			break
		}
		filter := "owner = {:owner} && calendar = {:calendar}"
		if definition.collection == "todos" {
			filter = "owner = {:owner} && (calendar = {:calendar} || (calendar = '' && list = {:calendar}))"
		}
		records, err := app.FindRecordsByFilter(definition.collection, filter, "id", remaining, 0, dbx.Params{"owner": userID, "calendar": calendarID})
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			resource, err := makeCalDAVResource(record, definition.collection, userID, calendarID)
			if err != nil {
				return nil, err
			}
			result = append(result, resource)
		}
	}
	return result, nil
}

func findCalDAVResource(app core.App, userID string, path calDAVPath) (calDAVResource, error) {
	if (path.kind != "resource" && path.kind != "new-resource") || path.userID != userID {
		return calDAVResource{}, fmt.Errorf("resource not found")
	}
	if path.itemKind == "" {
		for _, kind := range []string{"events", "todos"} {
			matches, err := app.FindRecordsByFilter(kind, "owner = {:owner} && dav_href = {:href}", "", 1, 0, dbx.Params{"owner": userID, "href": path.resourceName})
			if err != nil {
				return calDAVResource{}, err
			}
			if len(matches) == 1 && resourceCalendarID(matches[0], kind) == path.calendarID {
				return makeCalDAVResource(matches[0], kind, userID, path.calendarID)
			}
		}
		return calDAVResource{}, fmt.Errorf("resource not found")
	}
	record, err := app.FindRecordById(path.itemKind, path.recordID)
	if err != nil || record.GetString("owner") != userID || resourceCalendarID(record, path.itemKind) != path.calendarID {
		return calDAVResource{}, fmt.Errorf("resource not found")
	}
	return makeCalDAVResource(record, path.itemKind, userID, path.calendarID)
}

func resourceCalendarID(record *core.Record, kind string) string {
	if kind == "events" {
		return record.GetString("calendar")
	}
	if calendarID := record.GetString("calendar"); calendarID != "" {
		return calendarID
	}
	return record.GetString("list")
}

func makeCalDAVResource(record *core.Record, kind, userID, calendarID string) (calDAVResource, error) {
	calendar := ical.NewCalendar()
	calendar.Props.SetText(ical.PropVersion, "2.0")
	calendar.Props.SetText(ical.PropProductID, "-//Taskboard//CalDAV//EN")
	var component *ical.Component
	var err error
	var prefix string
	if kind == "events" {
		component, err = exportEvent(record)
		prefix = "event-"
	} else {
		component, err = exportTodo(record)
		prefix = "todo-"
	}
	if err != nil {
		return calDAVResource{}, err
	}
	calendar.Children = append(calendar.Children, component)
	data, err := encodeICalendar(calendar)
	if err != nil {
		return calDAVResource{}, err
	}
	hrefName := record.GetString("dav_href")
	if hrefName == "" {
		hrefName = prefix + record.Id + ".ics"
	}
	href := calendarHref(userID, calendarID) + url.PathEscape(hrefName)
	return calDAVResource{record: record, kind: kind, href: href, data: data, etag: calendarETag(data)}, nil
}

func encodeICalendar(calendar *ical.Calendar) ([]byte, error) {
	var output bytes.Buffer
	if err := ical.NewEncoder(&output).Encode(calendar); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func calendarETag(data []byte) string {
	digest := sha256.Sum256(data)
	return `"` + hex.EncodeToString(digest[:]) + `"`
}

// CalDAV sync uses the append-only change journal and monotonic owner sequence.
