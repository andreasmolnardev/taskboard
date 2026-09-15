package app

import (
	"errors"
	"net/url"
	"testing"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/pocketbase/pocketbase/core"
	calendarengine "github.com/slopstack/slopstack/apps/server/internal/calendar"
)

func TestCalendarOccurrencesExpandsOwnedMastersAndOverlaps(t *testing.T) {
	testApp, owner, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	calendar.Set("color", "#123456")
	if err := testApp.Save(calendar); err != nil {
		t.Fatal(err)
	}
	other := newUser(t, testApp, "occurrences-other@example.com")
	otherCalendar := newCalendar(t, testApp, other.Id, "Private")

	master := addOccurrenceEvent(t, testApp, map[string]any{
		"owner": owner.Id, "calendar": calendar.Id, "title": "Daily", "description": "Body",
		"start_date": "2026-09-01 09:00:00.000Z", "end_date": "2026-09-01 10:00:00.000Z",
		"rrule": "FREQ=DAILY;COUNT=20", "exdate": recurrenceDates(t, "EXDATE:20260913T090000Z"),
	})
	addOccurrenceEvent(t, testApp, map[string]any{
		"owner": owner.Id, "calendar": calendar.Id, "title": "Overlap",
		"start_date": "2026-09-14 23:30:00.000Z", "end_date": "2026-09-15 00:30:00.000Z",
	})
	addOccurrenceEvent(t, testApp, map[string]any{
		"owner": other.Id, "calendar": otherCalendar.Id, "title": "Private",
		"start_date": "2026-09-14 12:00:00.000Z",
	})

	items, err := calendarOccurrences(testApp, owner.Id, occurrenceParams{
		Start: mustTime(t, "2026-09-13T00:00:00Z"), End: mustTime(t, "2026-09-15T00:00:00Z"), Max: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d occurrences, want recurring hit plus overlap: %#v", len(items), items)
	}
	if items[0].MasterID != master.Id || items[0].Start != "2026-09-14T09:00:00Z" {
		t.Fatalf("master outside range did not expand or EXDATE failed: %#v", items[0])
	}
	if items[0].ID != master.Id+":2026-09-14T09:00:00Z" || items[0].Calendar != calendar.Id || items[0].Color != "#123456" || items[0].Description != "Body" {
		t.Fatalf("occurrence metadata is incomplete: %#v", items[0])
	}
	if items[1].Title != "Overlap" {
		t.Fatalf("nonrecurring overlap missing: %#v", items)
	}
	for _, item := range items {
		if item.Title == "Private" {
			t.Fatal("returned another owner's event")
		}
	}
}

func TestCalendarOccurrencesKeepsWallTimeAcrossDST(t *testing.T) {
	testApp, owner, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	addOccurrenceEvent(t, testApp, map[string]any{
		"owner": owner.Id, "calendar": calendar.Id, "title": "DST", "timezone": "America/New_York",
		"start_date": "2026-03-07 14:00:00.000Z", "end_date": "2026-03-07 15:00:00.000Z",
		"rrule": "FREQ=DAILY;COUNT=3",
	})
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	items, err := calendarOccurrences(testApp, owner.Id, occurrenceParams{
		Start: mustTime(t, "2026-03-07T00:00:00Z"), End: mustTime(t, "2026-03-10T00:00:00Z"), Timezone: zone, Max: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-03-07T09:00:00-05:00", "2026-03-08T09:00:00-04:00", "2026-03-09T09:00:00-04:00"}
	if len(items) != len(want) {
		t.Fatalf("got %d occurrences: %#v", len(items), items)
	}
	for index := range want {
		if items[index].Start != want[index] {
			t.Errorf("start %d = %q, want %q", index, items[index].Start, want[index])
		}
	}
}

func TestCalendarOccurrencesMergesDetachedOverrideAndCancellation(t *testing.T) {
	testApp, owner, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	master := addOccurrenceEvent(t, testApp, map[string]any{
		"owner": owner.Id, "calendar": calendar.Id, "uid": "series@example.com", "title": "Series",
		"start_date": "2026-09-10 09:00:00.000Z", "end_date": "2026-09-10 10:00:00.000Z", "rrule": "FREQ=DAILY;COUNT=3",
	})
	override, master, err := prepareRecurrenceOverride(testApp, owner.Id, master.Id, "2026-09-11T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	applyRecurrenceValues(override, master, map[string]any{
		"title": "Moved", "start_date": "2026-09-12 12:00:00.000Z", "end_date": "2026-09-12 13:00:00.000Z",
	})
	if err := testApp.Save(override); err != nil {
		t.Fatal(err)
	}
	cancelled, master, err := prepareRecurrenceOverride(testApp, owner.Id, master.Id, "2026-09-10T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	applyRecurrenceValues(cancelled, master, nil)
	cancelled.Set("recurrence_cancelled", true)
	if err := testApp.Save(cancelled); err != nil {
		t.Fatal(err)
	}

	items, err := calendarOccurrences(testApp, owner.Id, occurrenceParams{Start: mustTime(t, "2026-09-10T00:00:00Z"), End: mustTime(t, "2026-09-13T00:00:00Z"), Max: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d occurrences, want two: %#v", len(items), items)
	}
	if items[0].Title != "Series" || items[0].Start != "2026-09-12T09:00:00Z" {
		t.Fatalf("remaining series occurrence changed: %#v", items)
	}
	if items[1].Title != "Moved" || items[1].Start != "2026-09-12T12:00:00Z" || items[1].RecurrenceID != "2026-09-11T09:00:00Z" {
		t.Fatalf("override was not merged: %#v", items)
	}
}

func TestCalendarOccurrencesEnforcesGlobalLimit(t *testing.T) {
	testApp, owner, calendar := newCalendarTestApp(t)
	defer testApp.Cleanup()
	addOccurrenceEvent(t, testApp, map[string]any{
		"owner": owner.Id, "calendar": calendar.Id, "title": "Many", "start_date": "2026-09-01 09:00:00.000Z", "rrule": "FREQ=DAILY",
	})
	_, err := calendarOccurrences(testApp, owner.Id, occurrenceParams{
		Start: mustTime(t, "2026-09-01T00:00:00Z"), End: mustTime(t, "2026-09-10T00:00:00Z"), Max: 3,
	})
	if !errors.Is(err, calendarengine.ErrLimitExceeded) {
		t.Fatalf("error = %v, want occurrence limit", err)
	}
}

func TestParseOccurrenceParamsRejectsInvalidBounds(t *testing.T) {
	validStart := "2026-09-01T00:00:00Z"
	validEnd := "2026-10-01T00:00:00Z"
	tests := []url.Values{
		{"end": {validEnd}},
		{"start": {validStart}},
		{"start": {"2026-09-01"}, "end": {validEnd}},
		{"start": {validStart}, "end": {validStart}},
		{"start": {validEnd}, "end": {validStart}},
		{"start": {validStart}, "end": {validEnd}, "timezone": {"Mars/Olympus"}},
		{"start": {validStart}, "end": {validEnd}, "max": {"0"}},
		{"start": {validStart}, "end": {validEnd}, "max": {"5001"}},
		{"start": {validStart}, "end": {validEnd}, "max": {"many"}},
	}
	for _, values := range tests {
		if _, err := parseOccurrenceParams(values); err == nil {
			t.Errorf("expected invalid params to fail: %v", values)
		}
	}

	params, err := parseOccurrenceParams(url.Values{"start": {validStart}, "end": {validEnd}, "timezone": {"UTC"}, "max": {"5000"}})
	if err != nil {
		t.Fatal(err)
	}
	if params.Max != 5000 || params.Timezone.String() != "UTC" {
		t.Fatalf("valid upper bounds changed: %#v", params)
	}
}

func addOccurrenceEvent(t *testing.T, app core.App, fields map[string]any) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(collection)
	for name, value := range fields {
		record.Set(name, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func recurrenceDates(t *testing.T, line string) string {
	t.Helper()
	component := decodeCalendar(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nBEGIN:VEVENT\r\nUID:test\r\nDTSTART:20260901T090000Z\r\n"+line+"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n").Children[0]
	return storeProperties(component, ical.PropExceptionDates, ical.PropRecurrenceDates)
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
