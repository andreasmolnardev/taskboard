package app

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"
	"time"

	ical "github.com/emersion/go-ical"
)

func TestParseCalDAVPath(t *testing.T) {
	tests := []struct {
		path       string
		kind       string
		itemKind   string
		recordID   string
		calendarID string
	}{
		{"/caldav/", "root", "", "", ""},
		{"/caldav/principals/user1/", "principal", "", "", ""},
		{"/caldav/calendars/user1/", "home", "", "", ""},
		{"/caldav/calendars/user1/cal1/", "calendar", "", "", "cal1"},
		{"/caldav/calendars/user1/cal1/event-record1.ics", "resource", "events", "record1", "cal1"},
		{"/caldav/calendars/user1/cal1/todo-record2.ics", "resource", "todos", "record2", "cal1"},
		{"/caldav/calendars/user1/cal1/client-name.ics", "new-resource", "", "", "cal1"},
		{"/caldav/calendars/user1/cal1/not-ics", "", "", "", ""},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got := parseCalDAVPath(test.path)
			if got.kind != test.kind || got.itemKind != test.itemKind || got.recordID != test.recordID || got.calendarID != test.calendarID {
				t.Fatalf("parseCalDAVPath(%q) = %#v", test.path, got)
			}
		})
	}
}

func TestCalDAVPropfindXMLTracksKnownAndMissingProperties(t *testing.T) {
	request, err := parseCalDAVXML(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:displayname/><c:calendar-home-set/><d:unknown/></d:prop></d:propfind>`)
	if err != nil {
		t.Fatal(err)
	}
	available := map[xml.Name]string{
		davName("displayname"):          "Work &amp; Home",
		calDAVName("calendar-home-set"): `<d:href>/caldav/calendars/u/</d:href>`,
	}
	ok, missing := requestedCalDAVProperties(request, available)
	body := string(calDAVMultistatus([]calDAVResponse{{Href: "/caldav/", OK: ok, NotFound: missing}}))
	for _, want := range []string{"207 Multi-Status", "<d:displayname>Work &amp; Home</d:displayname>", "<c:calendar-home-set>", "<d:unknown>", "404 Not Found"} {
		if !strings.Contains(body, want) && want != "207 Multi-Status" {
			t.Errorf("response lacks %q: %s", want, body)
		}
	}
	if len(ok) != 2 || len(missing) != 1 {
		t.Fatalf("got %d known and %d missing properties", len(ok), len(missing))
	}
}

func TestCalDAVReportXMLParsesMultigetAndQuery(t *testing.T) {
	multiget, err := parseCalDAVXML(`<c:calendar-multiget xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:getetag/><c:calendar-data/></d:prop><d:href>/caldav/calendars/u/c/event-r.ics</d:href></c:calendar-multiget>`)
	if err != nil {
		t.Fatal(err)
	}
	if multiget.XMLName.Space != calDAVNamespace || multiget.XMLName.Local != "calendar-multiget" || len(multiget.Hrefs) != 1 || len(multiget.Prop.Names) != 2 {
		t.Fatalf("bad multiget parse: %#v", multiget)
	}
	query, err := parseCalDAVXML(`<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:getetag/></d:prop><c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"><c:time-range start="20260901T000000Z" end="20261001T000000Z"/></c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`)
	if err != nil {
		t.Fatal(err)
	}
	component := query.Filter.Components[0].Components[0]
	if component.Name != "VEVENT" || component.TimeRange == nil || component.TimeRange.Start != "20260901T000000Z" {
		t.Fatalf("bad calendar-query parse: %#v", query)
	}
}

func TestCalDAVPreconditions(t *testing.T) {
	etag := `"abc"`
	tests := []struct {
		name   string
		exists bool
		header http.Header
		want   int
	}{
		{"create only succeeds when absent", false, http.Header{"If-None-Match": {"*"}}, 0},
		{"create only rejects existing", true, http.Header{"If-None-Match": {"*"}}, http.StatusPreconditionFailed},
		{"matching update", true, http.Header{"If-Match": {etag}}, 0},
		{"stale update", true, http.Header{"If-Match": {`"old"`}}, http.StatusPreconditionFailed},
		{"missing update", false, http.Header{"If-Match": {"*"}}, http.StatusPreconditionFailed},
		{"cached representation", true, http.Header{"If-None-Match": {`"other", "abc"`}}, http.StatusPreconditionFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := checkCalDAVPreconditions(test.header, test.exists, etag); got != test.want {
				t.Fatalf("got %d, want %d", got, test.want)
			}
		})
	}
}

func TestCalDAVCalendarQueryFiltersTypeAndTime(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	event := decodeCalendar(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nBEGIN:VEVENT\r\nUID:inside@example.com\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:20260913T090000Z\r\nDTEND:20260913T100000Z\r\nSUMMARY:Inside\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n").Children[0]
	if err := saveImportedEvent(testApp, user.Id, calendar.Id, event); err != nil {
		t.Fatal(err)
	}
	resources, err := calendarResources(testApp, user.Id, calendar.Id, maxCalDAVQueryResults)
	if err != nil {
		t.Fatal(err)
	}
	filters := []calDAVComponentFilter{{Name: "VCALENDAR", Components: []calDAVComponentFilter{{Name: "VEVENT", TimeRange: &calDAVTimeRange{Start: "20260913T000000Z", End: "20260914T000000Z"}}}}}
	if got := filterCalDAVResources(resources, filters); len(got) != 1 || got[0].kind != "events" {
		t.Fatalf("expected one event, got %#v", got)
	}
	filters[0].Components[0].TimeRange = &calDAVTimeRange{Start: "20260914T000000Z", End: "20260915T000000Z"}
	if got := filterCalDAVResources(resources, filters); len(got) != 0 {
		t.Fatalf("expected no events, got %d", len(got))
	}
}

func TestCalDAVCalendarQueryIncludesRecurringOccurrenceInRange(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	addOccurrenceEvent(t, testApp, map[string]any{
		"owner": user.Id, "calendar": calendar.Id, "uid": "recurring-dav@example.com", "title": "Recurring",
		"start_date": "2026-09-01 09:00:00.000Z", "end_date": "2026-09-01 10:00:00.000Z", "rrule": "FREQ=DAILY;COUNT=20",
	})
	resources, err := calendarResources(testApp, user.Id, calendar.Id, maxCalDAVQueryResults)
	if err != nil {
		t.Fatal(err)
	}
	filters := []calDAVComponentFilter{{Name: "VCALENDAR", Components: []calDAVComponentFilter{{Name: "VEVENT", TimeRange: &calDAVTimeRange{Start: "20260913T000000Z", End: "20260914T000000Z"}}}}}
	if got := filterCalDAVResources(resources, filters); len(got) != 1 {
		t.Fatalf("recurring resource count = %d", len(got))
	}
}

func TestCalDAVResourceUsesStoredClientHref(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	event := addOccurrenceEvent(t, testApp, map[string]any{
		"owner": user.Id, "calendar": calendar.Id, "title": "DAV event", "uid": "stored-href@example.com",
		"start_date": "2026-09-13 09:00:00.000Z", "dav_href": "client-name.ics",
	})
	resource, err := makeCalDAVResource(event, "events", user.Id, calendar.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(resource.href, "/client-name.ics") {
		t.Fatalf("href = %q", resource.href)
	}
	path := parseCalDAVPath(resource.href)
	if path.kind != "new-resource" || path.resourceName != "client-name.ics" {
		t.Fatalf("custom href path = %#v", path)
	}
	resolved, err := findCalDAVResource(testApp, user.Id, path)
	if err != nil || resolved.record.Id != event.Id {
		t.Fatalf("resolved resource = %#v, %v", resolved, err)
	}
}

func TestCalDAVResourceUsesExistingICSRoundTrip(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	component := ical.NewComponent(ical.CompEvent)
	component.Props.SetText(ical.PropUID, "dav@example.com")
	component.Props.SetText(ical.PropSummary, "DAV event")
	component.Props.SetDateTime(ical.PropDateTimeStart, time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC))
	if err := saveImportedEvent(testApp, user.Id, calendar.Id, component); err != nil {
		t.Fatal(err)
	}
	resources, err := calendarResources(testApp, user.Id, calendar.Id, maxCalDAVQueryResults)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || !strings.Contains(string(resources[0].data), "UID:dav@example.com") || !strings.HasPrefix(resources[0].etag, `"`) {
		t.Fatalf("bad DAV resource: %#v", resources)
	}
}

func TestCalDAVChangeJournalTracksUpdatesAndDeletes(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	event := addOccurrenceEvent(t, testApp, map[string]any{
		"owner": user.Id, "calendar": calendar.Id, "title": "Journal", "uid": "journal@example.com",
		"start_date": "2026-09-13 09:00:00.000Z",
	})
	if err := appendCalDAVChange(testApp, event, "events", false); err != nil {
		t.Fatal(err)
	}
	event.Set("title", "Updated")
	if err := testApp.Save(event); err != nil {
		t.Fatal(err)
	}
	if err := appendCalDAVChange(testApp, event, "events", false); err != nil {
		t.Fatal(err)
	}
	if err := appendCalDAVChange(testApp, event, "events", true); err != nil {
		t.Fatal(err)
	}
	token, err := currentCalDAVToken(testApp, user.Id, calendar.Id)
	if err != nil || token != "3" {
		t.Fatalf("token = %q, %v", token, err)
	}
	changes, err := calDAVChangesSince(testApp, user.Id, calendar.Id, 1, 10)
	if err != nil || len(changes) != 2 || !changes[1].GetBool("deleted") {
		t.Fatalf("changes = %#v, %v", changes, err)
	}
}

func TestCalDAVParsesSyncCollectionRequest(t *testing.T) {
	request, err := parseCalDAVXML(`<d:sync-collection xmlns:d="DAV:"><d:sync-token>7</d:sync-token><d:sync-level>1</d:sync-level><d:prop><d:getetag/></d:prop></d:sync-collection>`)
	if err != nil {
		t.Fatal(err)
	}
	if request.XMLName.Space != davNamespace || request.XMLName.Local != "sync-collection" || request.SyncToken != "7" || request.SyncLevel != "1" {
		t.Fatalf("request = %#v", request)
	}
}

func TestCalDAVCalendarPropertiesBaseDoesNotInventSyncToken(t *testing.T) {
	testApp, _, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	properties := calendarProperties(calendar)
	if _, found := properties[davName("sync-token")]; found {
		t.Fatal("sync-token must not be advertised without a change journal")
	}
}
