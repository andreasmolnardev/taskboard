package app

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	ical "github.com/emersion/go-ical"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestSaveImportedEventUpdatesMatchingUID(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()

	decode := func(summary string) *ical.Component {
		return decodeCalendar(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nBEGIN:VEVENT\r\nUID:stable@example.com\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:20260913T090000Z\r\nSUMMARY:"+summary+"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n").Children[0]
	}
	if err := saveImportedEvent(testApp, user.Id, calendar.Id, decode("First")); err != nil {
		t.Fatal(err)
	}
	if err := saveImportedEvent(testApp, user.Id, calendar.Id, decode("Updated")); err != nil {
		t.Fatal(err)
	}

	records := findOwnedUID(t, testApp, "events", user.Id, "stable@example.com")
	if len(records) != 1 {
		t.Fatalf("expected one event after repeated import, got %d", len(records))
	}
	if title := records[0].GetString("title"); title != "Updated" {
		t.Fatalf("expected updated title, got %q", title)
	}
	if calendarID := records[0].GetString("calendar"); calendarID != calendar.Id {
		t.Fatalf("expected calendar %q, got %q", calendar.Id, calendarID)
	}
}

func TestRecurringOverrideImportExportRoundTrip(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	input := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:series@example.com\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:20260913T090000Z\r\nDTEND:20260913T100000Z\r\nRRULE:FREQ=DAILY;COUNT=2\r\nSUMMARY:Master\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:series@example.com\r\nDTSTAMP:20260912T120000Z\r\nRECURRENCE-ID:20260914T090000Z\r\nDTSTART:20260914T120000Z\r\nDTEND:20260914T130000Z\r\nSUMMARY:Moved\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	for _, component := range decodeCalendar(t, input).Children {
		if err := saveImportedEvent(testApp, user.Id, calendar.Id, component); err != nil {
			t.Fatal(err)
		}
	}
	records, err := testApp.FindRecordsByFilter("events", "owner = {:owner}", "", 0, 0, dbx.Params{"owner": user.Id})
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %#v, %v", records, err)
	}
	output := exportedCalendar(t, testApp, user.Id, calendar.Id)
	if len(output.Children) != 2 {
		t.Fatalf("exported components = %d", len(output.Children))
	}
	foundOverride := false
	for _, component := range output.Children {
		if component.Props.Get(ical.PropRecurrenceID) != nil {
			foundOverride = true
			if component.Props.Get(ical.PropRecurrenceID).Value != "20260914T090000Z" || calendarText(component, ical.PropSummary) != "Moved" {
				t.Fatalf("override changed: %#v", component)
			}
		}
	}
	if !foundOverride {
		t.Fatal("export omitted recurrence override")
	}
}

func TestCalendarEventRichFieldsRoundTrip(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	input := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:rich-event@example.com\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:20260913T090000Z\r\nSUMMARY:Rich event\r\n" +
		"ORGANIZER;CN=Owner:mailto:owner@example.com\r\n" +
		"ATTENDEE;CN=One;ROLE=REQ-PARTICIPANT:mailto:one@example.com\r\nATTENDEE;CN=Two:mailto:two@example.com\r\n" +
		"URL:https://example.com/events/1\r\nCLASS:PRIVATE\r\nGEO:37.386013;-122.082932\r\n" +
		"EXDATE:20260920T090000Z,20260921T090000Z\r\nEXDATE;TZID=America/New_York:20260922T090000\r\n" +
		"RDATE;VALUE=DATE:20260925,20260926\r\nRDATE:20260927T090000Z\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nDESCRIPTION:Soon\r\nEND:VALARM\r\n" +
		"BEGIN:VALARM\r\nACTION:AUDIO\r\nTRIGGER:-PT5M\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	component := decodeCalendar(t, input).Children[0]
	if err := saveImportedEvent(testApp, user.Id, calendar.Id, component); err != nil {
		t.Fatal(err)
	}

	exported := exportedCalendar(t, testApp, user.Id, calendar.Id)
	if len(exported.Children) != 1 {
		t.Fatalf("expected one component, got %d", len(exported.Children))
	}
	got := exported.Children[0]
	assertPropertySetEqual(t, component, got, ical.PropOrganizer, ical.PropAttendee, ical.PropExceptionDates, ical.PropRecurrenceDates)
	for _, name := range []string{ical.PropURL, ical.PropClass, ical.PropGeo} {
		if calendarRaw(got, name) != calendarRaw(component, name) {
			t.Errorf("%s changed: got %q want %q", name, calendarRaw(got, name), calendarRaw(component, name))
		}
	}
	if len(got.Children) != 2 {
		t.Fatalf("expected two alarms, got %d", len(got.Children))
	}
	if got.Children[0].Props.Get(ical.PropTrigger).Value != "-PT15M" || got.Children[1].Props.Get(ical.PropAction).Value != "AUDIO" {
		t.Fatal("alarm data changed during round trip")
	}
}

func TestCalendarTemporalValuesRoundTrip(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	input := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:zone@example.com\r\nDTSTAMP:20261001T120000Z\r\nDTSTART;TZID=America/New_York:20261101T090000\r\nDTEND;TZID=America/New_York:20261101T100000\r\nSUMMARY:Zone\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:day@example.com\r\nDTSTAMP:20261001T120000Z\r\nDTSTART;VALUE=DATE:20261101\r\nDTEND;VALUE=DATE:20261103\r\nSUMMARY:Days\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	for _, component := range decodeCalendar(t, input).Children {
		if err := saveImportedEvent(testApp, user.Id, calendar.Id, component); err != nil {
			t.Fatal(err)
		}
	}
	zone := findOwnedUID(t, testApp, "events", user.Id, "zone@example.com")[0]
	if zone.GetString("timezone") != "America/New_York" || zone.GetString("time_mode") != "zoned" || zone.GetString("start_local") != "20261101T090000" {
		t.Fatalf("zoned time was not preserved: timezone=%q mode=%q local=%q", zone.GetString("timezone"), zone.GetString("time_mode"), zone.GetString("start_local"))
	}
	day := findOwnedUID(t, testApp, "events", user.Id, "day@example.com")[0]
	if !day.GetBool("all_day") || day.GetString("time_mode") != "date" || day.GetString("start_local") != "20261101" || day.GetString("end_local") != "20261103" {
		t.Fatalf("date-only event was not preserved: %#v", day)
	}
	output := exportedCalendar(t, testApp, user.Id, calendar.Id)
	for _, component := range output.Children {
		if component.Props.Get(ical.PropUID).Value == "zone@example.com" {
			start := component.Props.Get(ical.PropDateTimeStart)
			if start.Params.Get(ical.PropTimezoneID) != "America/New_York" || start.Value != "20261101T090000" {
				t.Fatalf("zone export changed: %#v", start)
			}
		}
		if component.Props.Get(ical.PropUID).Value == "day@example.com" {
			if component.Props.Get(ical.PropDateTimeStart).Value != "20261101" || component.Props.Get(ical.PropDateTimeEnd).Value != "20261103" {
				t.Fatal("exclusive all-day dates changed")
			}
		}
	}
}

func TestCalendarTodoImportExport(t *testing.T) {
	testApp, user, _ := newCalendarTestApp(t)
	defer testApp.Cleanup()
	list := newList(t, testApp, user.Id, "Imported tasks")
	input := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:task@example.com\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:20260913T090000Z\r\nDUE:20260914T170000Z\r\nCOMPLETED:20260914T160000Z\r\n" +
		"SUMMARY:Ship it\r\nDESCRIPTION:Task body\r\nSTATUS:COMPLETED\r\nPERCENT-COMPLETE:100\r\nPRIORITY:1\r\n" +
		"ORGANIZER:mailto:owner@example.com\r\nATTENDEE:mailto:worker@example.com\r\nURL:https://example.com/tasks/1\r\nCLASS:CONFIDENTIAL\r\nGEO:1.0;2.0\r\n" +
		"EXDATE:20260920T090000Z\r\nRDATE:20260921T090000Z\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT10M\r\nDESCRIPTION:Task due\r\nEND:VALARM\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"
	component := decodeCalendar(t, input).Children[0]
	if err := saveImportedTodo(testApp, user.Id, list.Id, component); err != nil {
		t.Fatal(err)
	}

	records := findOwnedUID(t, testApp, "todos", user.Id, "task@example.com")
	if len(records) != 1 {
		t.Fatalf("expected one task, got %d", len(records))
	}
	if !records[0].GetBool("completed") || records[0].GetString("list") != list.Id {
		t.Fatal("task state or target was not saved")
	}
	got, err := exportTodo(records[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ical.PropUID, ical.PropSummary, ical.PropDescription, ical.PropStatus, ical.PropPercentComplete, ical.PropPriority, ical.PropURL, ical.PropClass, ical.PropGeo} {
		if calendarRaw(got, name) != calendarRaw(component, name) {
			t.Errorf("%s changed: got %q want %q", name, calendarRaw(got, name), calendarRaw(component, name))
		}
	}
	assertPropertySetEqual(t, component, got, ical.PropOrganizer, ical.PropAttendee, ical.PropExceptionDates, ical.PropRecurrenceDates)
	if len(got.Children) != 1 {
		t.Fatalf("expected one task alarm, got %d", len(got.Children))
	}
}

func TestImportedUIDsAreIsolatedByOwnerAndType(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	otherUser := newUser(t, testApp, "other-calendar@example.com")
	otherCalendar := newCalendar(t, testApp, otherUser.Id, "Other")
	uid := "shared@example.com"
	event := decodeCalendar(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nBEGIN:VEVENT\r\nUID:"+uid+"\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:20260913T090000Z\r\nSUMMARY:Event\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n").Children[0]
	todo := decodeCalendar(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nBEGIN:VTODO\r\nUID:"+uid+"\r\nDTSTAMP:20260912T120000Z\r\nSUMMARY:Task\r\nEND:VTODO\r\nEND:VCALENDAR\r\n").Children[0]

	if err := saveImportedEvent(testApp, otherUser.Id, calendar.Id, event); err == nil {
		t.Fatal("expected helper to reject a calendar owned by another user")
	}
	if err := saveImportedEvent(testApp, otherUser.Id, otherCalendar.Id, event); err != nil {
		t.Fatal(err)
	}
	if err := saveImportedEvent(testApp, user.Id, calendar.Id, event); err != nil {
		t.Fatal(err)
	}
	list := newList(t, testApp, user.Id, "Imported tasks")
	if err := saveImportedTodo(testApp, user.Id, list.Id, todo); err != nil {
		t.Fatal(err)
	}
	if len(findOwnedUID(t, testApp, "events", otherUser.Id, uid)) != 1 || len(findOwnedUID(t, testApp, "events", user.Id, uid)) != 1 || len(findOwnedUID(t, testApp, "todos", user.Id, uid)) != 1 {
		t.Fatal("same UID should remain distinct across owners and component types")
	}
}

func TestImportedComponentRejectsMalformedInput(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	tests := []struct {
		name string
		ics  string
		save func(core.App, string, string, *ical.Component) error
	}{
		{"event missing UID", "BEGIN:VEVENT\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:20260913T090000Z\r\nSUMMARY:X\r\nEND:VEVENT\r\n", saveImportedEvent},
		{"event missing start", "BEGIN:VEVENT\r\nUID:x\r\nDTSTAMP:20260912T120000Z\r\nSUMMARY:X\r\nEND:VEVENT\r\n", saveImportedEvent},
		{"event invalid start", "BEGIN:VEVENT\r\nUID:x\r\nDTSTAMP:20260912T120000Z\r\nDTSTART:not-a-date\r\nSUMMARY:X\r\nEND:VEVENT\r\n", saveImportedEvent},
		{"event invalid stamp", "BEGIN:VEVENT\r\nUID:x\r\nDTSTAMP:bad\r\nDTSTART:20260913T090000Z\r\nSUMMARY:X\r\nEND:VEVENT\r\n", saveImportedEvent},
		{"todo invalid due", "BEGIN:VTODO\r\nUID:x\r\nDTSTAMP:20260912T120000Z\r\nDUE:bad\r\nSUMMARY:X\r\nEND:VTODO\r\n", saveImportedTodo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calendarData := decodeCalendar(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n"+tc.ics+"END:VCALENDAR\r\n")
			if err := tc.save(testApp, user.Id, calendar.Id, calendarData.Children[0]); err == nil {
				t.Fatal("expected malformed component error")
			}
		})
	}
	if err := saveImportedEvent(testApp, user.Id, calendar.Id, nil); err == nil {
		t.Fatal("expected nil component error")
	}
}

func TestImportedRecordRejectsAmbiguousOwnedDuplicate(t *testing.T) {
	testApp, user, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	collection, err := testApp.FindCollectionByNameOrId("events")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		record := core.NewRecord(collection)
		record.Set("owner", user.Id)
		record.Set("calendar", calendar.Id)
		record.Set("uid", "duplicate@example.com")
		record.Set("title", "Duplicate")
		record.Set("start_date", "2026-09-13 09:00:00.000Z")
		if err := testApp.Save(record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := importedRecord(testApp, "events", user.Id, "duplicate@example.com"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous duplicate error, got %v", err)
	}
}

func newCalendarTestApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
	t.Helper()
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureCollections(testApp); err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	user := newUser(t, testApp, "calendar@example.com")
	calendar := newCalendar(t, testApp, user.Id, "Imported")
	return testApp, user, calendar
}

func newList(t *testing.T, app core.App, owner, name string) *core.Record {
	t.Helper()
	lists, err := app.FindCollectionByNameOrId("lists")
	if err != nil {
		t.Fatal(err)
	}
	list := core.NewRecord(lists)
	list.Set("owner", owner)
	list.Set("name", name)
	if err := app.Save(list); err != nil {
		t.Fatal(err)
	}
	return list
}

func newCalendar(t *testing.T, app core.App, owner, name string) *core.Record {
	t.Helper()
	calendars, err := app.FindCollectionByNameOrId("calendars")
	if err != nil {
		t.Fatal(err)
	}
	calendar := core.NewRecord(calendars)
	calendar.Set("owner", owner)
	calendar.Set("name", name)
	if err := app.Save(calendar); err != nil {
		t.Fatal(err)
	}
	return calendar
}

func newUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("password123456")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	return user
}

func decodeCalendar(t *testing.T, input string) *ical.Calendar {
	t.Helper()
	decoded, err := ical.NewDecoder(strings.NewReader(input)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func exportedCalendar(t *testing.T, app core.App, userID, calendarID string) *ical.Calendar {
	t.Helper()
	output, err := buildCalendarExport(app, userID, calendarID)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := ical.NewEncoder(&encoded).Encode(output); err != nil {
		t.Fatal(err)
	}
	return decodeCalendar(t, encoded.String())
}

func findOwnedUID(t *testing.T, app core.App, collection, owner, uid string) []*core.Record {
	t.Helper()
	records, err := app.FindRecordsByFilter(collection, "owner = {:owner} && uid = {:uid}", "", 0, 0, dbx.Params{"owner": owner, "uid": uid})
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func assertPropertySetEqual(t *testing.T, want, got *ical.Component, names ...string) {
	t.Helper()
	for _, name := range names {
		if !reflect.DeepEqual(got.Props.Values(name), want.Props.Values(name)) {
			t.Errorf("%s changed:\n got %#v\nwant %#v", name, got.Props.Values(name), want.Props.Values(name))
		}
	}
}
