package calendar

import (
	"errors"
	"strings"
	"testing"
	"time"

	ical "github.com/emersion/go-ical"
)

func TestExpandDailyCount(t *testing.T) {
	component := parseEvent(t, "DTSTART:20260913T090000Z\r\nDTEND:20260913T100000Z\r\nRRULE:FREQ=DAILY;COUNT=3")
	got, err := Expand(component, utc("2026-09-01T00:00:00Z"), utc("2026-10-01T00:00:00Z"), 10)
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, got, "2026-09-13T09:00:00Z", "2026-09-14T09:00:00Z", "2026-09-15T09:00:00Z")
	if got[0].End != utc("2026-09-13T10:00:00Z") {
		t.Fatalf("end = %v", got[0].End)
	}
}

func TestExpandWeeklyUntil(t *testing.T) {
	component := parseEvent(t, "DTSTART:20260907T090000Z\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20260916T090000Z")
	got, err := Expand(component, utc("2026-09-01T00:00:00Z"), utc("2026-10-01T00:00:00Z"), 10)
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, got,
		"2026-09-07T09:00:00Z", "2026-09-09T09:00:00Z",
		"2026-09-14T09:00:00Z", "2026-09-16T09:00:00Z",
	)
}

func TestExpandExDateAndRDate(t *testing.T) {
	component := parseEvent(t, "DTSTART:20260913T090000Z\r\nRRULE:FREQ=DAILY;COUNT=3\r\nEXDATE:20260914T090000Z,20260915T090000Z\r\nRDATE:20260920T090000Z")
	got, err := Expand(component, utc("2026-09-01T00:00:00Z"), utc("2026-10-01T00:00:00Z"), 10)
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, got, "2026-09-13T09:00:00Z", "2026-09-20T09:00:00Z")
}

func TestExpandAllDay(t *testing.T) {
	component := parseEvent(t, "DTSTART;VALUE=DATE:20260913\r\nDTEND;VALUE=DATE:20260915\r\nRRULE:FREQ=DAILY;COUNT=2")
	got, err := Expand(component, utc("2026-09-14T12:00:00Z"), utc("2026-09-16T00:00:00Z"), 10)
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, got, "2026-09-13T00:00:00Z", "2026-09-14T00:00:00Z")
	if !got[0].AllDay || got[0].End != utc("2026-09-15T00:00:00Z") {
		t.Fatalf("first occurrence = %#v", got[0])
	}
}

func TestExpandKeepsLocalTimeAcrossDST(t *testing.T) {
	component := parseEvent(t, "DTSTART;TZID=America/New_York:20260307T090000\r\nDTEND;TZID=America/New_York:20260307T100000\r\nRRULE:FREQ=DAILY;COUNT=3")
	got, err := Expand(component, utc("2026-03-01T00:00:00Z"), utc("2026-03-15T00:00:00Z"), 10)
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, got, "2026-03-07T14:00:00Z", "2026-03-08T13:00:00Z", "2026-03-09T13:00:00Z")
	for _, occurrence := range got {
		if occurrence.Start.Hour() != 9 || occurrence.Start.Location().String() != "America/New_York" {
			t.Fatalf("start lost wall time or zone: %v", occurrence.Start)
		}
	}
}

func TestExpandRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing start", "RRULE:FREQ=DAILY"},
		{"bad rule", "DTSTART:20260913T090000Z\r\nRRULE:FREQ=NOPE"},
		{"bad zone", "DTSTART;TZID=Mars/Olympus:20260913T090000"},
		{"end before start", "DTSTART:20260913T100000Z\r\nDTEND:20260913T090000Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Expand(parseEvent(t, test.body), utc("2026-09-01T00:00:00Z"), utc("2026-10-01T00:00:00Z"), 10)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestExpandBounds(t *testing.T) {
	component := parseEvent(t, "DTSTART:20260913T090000Z\r\nRRULE:FREQ=DAILY")
	_, err := Expand(component, utc("2026-09-01T00:00:00Z"), utc("2026-10-01T00:00:00Z"), 2)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("error = %v, want ErrLimitExceeded", err)
	}
	if _, err := Expand(component, utc("2026-10-01T00:00:00Z"), utc("2026-09-01T00:00:00Z"), 10); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("error = %v, want ErrInvalidWindow", err)
	}
	if _, err := Expand(component, utc("2026-09-01T00:00:00Z"), utc("2026-10-01T00:00:00Z"), 0); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("error = %v, want ErrInvalidLimit", err)
	}
}

func parseEvent(t *testing.T, body string) *ical.Component {
	t.Helper()
	input := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nBEGIN:VEVENT\r\nUID:test@example.com\r\n" + body + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	decoded, err := ical.NewDecoder(strings.NewReader(input)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	return decoded.Events()[0].Component
}

func utc(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func assertStarts(t *testing.T, got []Occurrence, values ...string) {
	t.Helper()
	if len(got) != len(values) {
		t.Fatalf("got %d occurrences, want %d: %#v", len(got), len(values), got)
	}
	for i, value := range values {
		want, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		if !got[i].Start.Equal(want) {
			t.Errorf("occurrence %d start = %v, want %v", i, got[i].Start, want)
		}
	}
}
