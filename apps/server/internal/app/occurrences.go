package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	calendarengine "github.com/slopstack/slopstack/apps/server/internal/calendar"
)

const (
	defaultOccurrenceLimit = 1000
	maxOccurrenceLimit     = 5000
)

type occurrenceParams struct {
	Start         time.Time
	End           time.Time
	Timezone      *time.Location
	Max           int
	RecurringOnly bool
}

type occurrenceResult struct {
	ID           string `json:"id"`
	MasterID     string `json:"masterId"`
	Start        string `json:"start"`
	End          string `json:"end"`
	StartLocal   string `json:"startLocal,omitempty"`
	EndLocal     string `json:"endLocal,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
	TimeMode     string `json:"timeMode,omitempty"`
	AllDay       bool   `json:"allDay"`
	Recurring    bool   `json:"recurring"`
	RecurrenceID string `json:"recurrenceId,omitempty"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	Calendar     string `json:"calendar,omitempty"`
	Color        string `json:"color,omitempty"`
}

// registerOccurrenceRoutes is kept separate until the endpoint is wired into app.go.
func registerOccurrenceRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/calendar/occurrences", occurrenceRoute)
	e.Router.POST("/api/calendar/occurrences/{masterId}/override", recurrenceOverrideRoute)
	e.Router.POST("/api/calendar/occurrences/{masterId}/cancel", recurrenceCancelRoute)
}

func occurrenceRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	params, err := parseOccurrenceParams(event.Request.URL.Query())
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	items, err := calendarOccurrences(event.App, user.Id, params)
	if errors.Is(err, calendarengine.ErrLimitExceeded) {
		return event.BadRequestError(fmt.Sprintf("more than %d occurrences match the requested range", params.Max), nil)
	}
	if err != nil {
		return err
	}
	return event.JSON(http.StatusOK, items)
}

type recurrenceMutation struct {
	RecurrenceID string         `json:"recurrenceId"`
	Values       map[string]any `json:"values"`
}

func recurrenceOverrideRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	var input recurrenceMutation
	if err := json.NewDecoder(event.Request.Body).Decode(&input); err != nil {
		return event.BadRequestError("invalid occurrence body", err)
	}
	record, master, err := prepareRecurrenceOverride(event.App, user.Id, event.Request.PathValue("masterId"), input.RecurrenceID)
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	applyRecurrenceValues(record, master, input.Values)
	record.Set("recurrence_cancelled", false)
	if err := event.App.Save(record); err != nil {
		return err
	}
	return event.JSON(http.StatusOK, map[string]string{"id": record.Id, "recurrenceId": input.RecurrenceID})
}

func recurrenceCancelRoute(event *core.RequestEvent) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	var input recurrenceMutation
	if err := json.NewDecoder(event.Request.Body).Decode(&input); err != nil {
		return event.BadRequestError("invalid occurrence body", err)
	}
	record, master, err := prepareRecurrenceOverride(event.App, user.Id, event.Request.PathValue("masterId"), input.RecurrenceID)
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	applyRecurrenceValues(record, master, nil)
	record.Set("recurrence_cancelled", true)
	record.Set("status", "CANCELLED")
	if err := event.App.Save(record); err != nil {
		return err
	}
	return event.NoContent(http.StatusNoContent)
}

func prepareRecurrenceOverride(app core.App, owner, masterID, rawID string) (*core.Record, *core.Record, error) {
	master, err := app.FindRecordById("events", masterID)
	if err != nil || master.GetString("owner") != owner || master.GetString("recurrence_parent") != "" {
		return nil, nil, fmt.Errorf("recurring event not found")
	}
	id, err := parseRecurrenceID(rawID)
	if err != nil {
		return nil, nil, err
	}
	component, err := occurrenceComponent(master)
	if err != nil {
		return nil, nil, err
	}
	if !recordHasRecurrence(master) {
		return nil, nil, fmt.Errorf("event is not recurring")
	}
	windowStart, windowEnd := id.Add(-48*time.Hour), id.Add(48*time.Hour)
	occurrences, err := calendarengine.Expand(component, windowStart, windowEnd, 10000)
	if err != nil {
		return nil, nil, err
	}
	found := false
	for _, occurrence := range occurrences {
		if recurrenceID(occurrence.Start) == id.Format(time.RFC3339Nano) {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("occurrence is not part of this series")
	}
	matches, err := app.FindRecordsByFilter("events", "owner = {:owner} && recurrence_parent = {:parent} && recurrence_id = {:id}", "", 1, 0, dbx.Params{"owner": owner, "parent": masterID, "id": id.Format(time.RFC3339Nano)})
	if err != nil {
		return nil, nil, err
	}
	if len(matches) > 0 {
		return matches[0], master, nil
	}
	collection, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return nil, nil, err
	}
	record := core.NewRecord(collection)
	record.Set("owner", owner)
	record.Set("calendar", master.GetString("calendar"))
	record.Set("uid", master.GetString("uid"))
	record.Set("recurrence_parent", masterID)
	record.Set("recurrence_id", id.Format(time.RFC3339Nano))
	return record, master, nil
}

func parseRecurrenceID(raw string) (time.Time, error) {
	if value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw)); err == nil {
		return value.UTC(), nil
	}
	if value, err := time.Parse("20060102", strings.TrimSpace(raw)); err == nil {
		return value.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("recurrenceId must be an RFC3339 timestamp or date")
}

func applyRecurrenceValues(record, master *core.Record, values map[string]any) {
	fields := []string{"title", "description", "start_date", "start_local", "end_date", "end_local", "all_day", "timezone", "time_mode", "duration", "status", "location", "categories", "organizer", "attendees", "url", "class", "geo", "alarm"}
	if values == nil {
		for _, field := range fields {
			record.Set(field, master.Get(field))
		}
	}
	for _, field := range fields {
		if value, ok := values[field]; ok {
			record.Set(field, value)
		}
	}
	if values != nil {
		if _, ok := values["start_date"]; ok {
			if _, localOK := values["start_local"]; !localOK {
				record.Set("start_local", "")
			}
		}
		if _, ok := values["end_date"]; ok {
			if _, localOK := values["end_local"]; !localOK {
				record.Set("end_local", "")
			}
		}
	}
	if record.GetString("title") == "" {
		record.Set("title", master.GetString("title"))
	}
	if values == nil && record.GetString("start_local") == "" {
		record.Set("start_date", master.GetDateTime("start_date").Time())
		record.Set("start_local", master.GetString("start_local"))
	}
	if values == nil && record.GetString("end_local") == "" && !master.GetDateTime("end_date").Time().IsZero() {
		record.Set("end_date", master.GetDateTime("end_date").Time())
		record.Set("end_local", master.GetString("end_local"))
	}
	record.Set("rrule", "")
	record.Set("exdate", "")
	record.Set("dtstamp", time.Now().UTC())
}

func parseOccurrenceParams(values url.Values) (occurrenceParams, error) {
	start, err := requiredRFC3339(values.Get("start"), "start")
	if err != nil {
		return occurrenceParams{}, err
	}
	end, err := requiredRFC3339(values.Get("end"), "end")
	if err != nil {
		return occurrenceParams{}, err
	}
	if !start.Before(end) {
		return occurrenceParams{}, fmt.Errorf("start must be before end")
	}
	max, err := parseBoundedInt(values.Get("max"), defaultOccurrenceLimit, 1, maxOccurrenceLimit, "max")
	if err != nil {
		return occurrenceParams{}, err
	}

	var timezone *time.Location
	if name := strings.TrimSpace(values.Get("timezone")); name != "" {
		timezone, err = time.LoadLocation(name)
		if err != nil {
			return occurrenceParams{}, fmt.Errorf("timezone must be a valid IANA time zone")
		}
	}
	recurringOnly := false
	if raw := strings.TrimSpace(values.Get("recurringOnly")); raw != "" {
		recurringOnly, err = strconv.ParseBool(raw)
		if err != nil {
			return occurrenceParams{}, fmt.Errorf("recurringOnly must be a boolean")
		}
	}
	return occurrenceParams{Start: start, End: end, Timezone: timezone, Max: max, RecurringOnly: recurringOnly}, nil
}

func requiredRFC3339(raw, name string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, fmt.Errorf("%s is required and must be an RFC3339 timestamp", name)
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s is required and must be an RFC3339 timestamp", name)
	}
	return value, nil
}

func calendarOccurrences(app core.App, owner string, params occurrenceParams) ([]occurrenceResult, error) {
	events, err := app.FindRecordsByFilter("events", "owner = {:owner}", "id", 0, 0, dbx.Params{"owner": owner})
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	calendars, err := app.FindRecordsByFilter("calendars", "owner = {:owner}", "id", 0, 0, dbx.Params{"owner": owner})
	if err != nil {
		return nil, fmt.Errorf("load calendars: %w", err)
	}
	colors := make(map[string]string, len(calendars))
	for _, calendar := range calendars {
		colors[calendar.Id] = calendar.GetString("color")
	}

	items := make([]occurrenceResult, 0, min(params.Max, 64))
	overrides := make(map[string]map[string]*core.Record)
	masters := make([]*core.Record, 0, len(events))
	oneOffs := make([]*core.Record, 0, len(events))
	masterIDs := make(map[string]struct{}, len(events))
	for _, record := range events {
		parent := record.GetString("recurrence_parent")
		if parent == "" {
			if recordHasRecurrence(record) {
				masters = append(masters, record)
				masterIDs[record.Id] = struct{}{}
			} else {
				oneOffs = append(oneOffs, record)
			}
			continue
		}
		if overrides[parent] == nil {
			overrides[parent] = make(map[string]*core.Record)
		}
		overrides[parent][record.GetString("recurrence_id")] = record
	}
	for _, record := range masters {
		component, err := occurrenceComponent(record)
		if err != nil {
			return nil, fmt.Errorf("build event %s: %w", record.Id, err)
		}
		remaining := params.Max - len(items)
		if remaining <= 0 {
			return nil, calendarengine.ErrLimitExceeded
		}
		expanded, err := calendarengine.Expand(component, params.Start, params.End, remaining+1)
		if err != nil {
			return nil, fmt.Errorf("expand event %s: %w", record.Id, err)
		}
		for _, occurrence := range expanded {
			key := recurrenceID(occurrence.Start)
			effective := record
			shown := occurrence
			if override := overrides[record.Id][key]; override != nil {
				if override.GetBool("recurrence_cancelled") {
					continue
				}
				overrideComponent, componentErr := occurrenceComponent(override)
				if componentErr != nil {
					return nil, fmt.Errorf("build override %s: %w", override.Id, componentErr)
				}
				overrideOccurrences, expandErr := calendarengine.Expand(overrideComponent, params.Start, params.End, 1)
				if expandErr != nil {
					return nil, fmt.Errorf("expand override %s: %w", override.Id, expandErr)
				}
				if len(overrideOccurrences) == 0 {
					continue
				}
				effective, shown = override, overrideOccurrences[0]
			}
			if len(items) >= params.Max {
				return nil, calendarengine.ErrLimitExceeded
			}
			items = append(items, makeOccurrenceResult(record.Id, key, effective, shown, params.Timezone, colors))
		}
	}
	// A moved override can land inside the requested window even when its
	// original generated occurrence does not. Include those detached records.
	for parentID, records := range overrides {
		if _, ok := masterIDs[parentID]; !ok {
			continue
		}
		for key, override := range records {
			if override.GetBool("recurrence_cancelled") {
				continue
			}
			already := false
			for _, item := range items {
				if item.MasterID == parentID && item.RecurrenceID == key {
					already = true
					break
				}
			}
			if already {
				continue
			}
			component, err := occurrenceComponent(override)
			if err != nil {
				return nil, fmt.Errorf("build override %s: %w", override.Id, err)
			}
			expanded, err := calendarengine.Expand(component, params.Start, params.End, 1)
			if err != nil {
				return nil, fmt.Errorf("expand override %s: %w", override.Id, err)
			}
			for _, occurrence := range expanded {
				if len(items) >= params.Max {
					return nil, calendarengine.ErrLimitExceeded
				}
				items = append(items, makeOccurrenceResult(parentID, key, override, occurrence, params.Timezone, colors))
			}
		}
	}
	// Keep ordinary events available to existing callers, but let the web client
	// request only recurrence data so unrelated events cannot exhaust its limit.
	if params.RecurringOnly {
		oneOffs = nil
	}
	for _, record := range oneOffs {
		component, err := occurrenceComponent(record)
		if err != nil {
			continue
		}
		expanded, err := calendarengine.Expand(component, params.Start, params.End, 1)
		if err != nil {
			continue
		}
		for _, occurrence := range expanded {
			if len(items) >= params.Max {
				return nil, calendarengine.ErrLimitExceeded
			}
			items = append(items, makeOccurrenceResult(record.Id, recurrenceID(occurrence.Start), record, occurrence, params.Timezone, colors))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Start != items[j].Start {
			return items[i].Start < items[j].Start
		}
		if items[i].MasterID != items[j].MasterID {
			return items[i].MasterID < items[j].MasterID
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func recurrenceID(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func recordHasRecurrence(record *core.Record) bool {
	if strings.TrimSpace(record.GetString("rrule")) != "" {
		return true
	}
	var stored storedCalendarData
	if err := json.Unmarshal([]byte(record.GetString("exdate")), &stored); err != nil {
		return false
	}
	return len(stored.Properties[ical.PropRecurrenceDates]) > 0
}

func makeOccurrenceResult(masterID, id string, record *core.Record, occurrence calendarengine.Occurrence, timezone *time.Location, colors map[string]string) occurrenceResult {
	start, end := occurrence.Start, occurrence.End
	// Date-only and floating values represent wall-clock values, not instants.
	// Do not shift them when formatting for the requesting user's zone.
	if timezone != nil && !occurrence.AllDay && record.GetString("time_mode") != "floating" {
		start, end = start.In(timezone), end.In(timezone)
	}
	localZone := start.Location()
	if timezone != nil && !occurrence.AllDay {
		localZone = timezone
	}
	startLocal, endLocal := start.In(localZone), end.In(localZone)
	localFormat := "20060102T150405"
	if occurrence.AllDay {
		localFormat = "20060102"
	}
	return occurrenceResult{
		ID: masterID + ":" + id, MasterID: masterID, RecurrenceID: id,
		Start: start.Format(time.RFC3339), End: end.Format(time.RFC3339),
		StartLocal: startLocal.Format(localFormat), EndLocal: endLocal.Format(localFormat),
		Timezone: func() string {
			if timezone != nil && !occurrence.AllDay {
				return timezone.String()
			}
			return record.GetString("timezone")
		}(),
		TimeMode: record.GetString("time_mode"), AllDay: occurrence.AllDay, Recurring: recordHasRecurrence(record),
		Title: record.GetString("title"), Description: record.GetString("description"),
		Calendar: record.GetString("calendar"), Color: colors[record.GetString("calendar")],
	}
}

func occurrenceComponent(record *core.Record) (*ical.Component, error) {
	component := ical.NewComponent(ical.CompEvent)
	start := record.GetDateTime("start_date").Time()
	if start.IsZero() && record.GetString("start_local") == "" {
		return nil, errors.New("missing start_date")
	}
	if name := strings.TrimSpace(record.GetString("timezone")); name != "" {
		if _, err := time.LoadLocation(name); err != nil {
			return nil, fmt.Errorf("invalid timezone %q", name)
		}
	}
	setStoredDateTime(component, ical.PropDateTimeStart, record, "start_date", "start_local")
	if end := record.GetDateTime("end_date").Time(); !end.IsZero() || record.GetString("end_local") != "" {
		setStoredDateTime(component, ical.PropDateTimeEnd, record, "end_date", "end_local")
	} else {
		setOptionalRaw(component, ical.PropDuration, record.GetString("duration"))
	}
	setOptionalRaw(component, ical.PropRecurrenceRule, record.GetString("rrule"))
	loadProperties(component, record.GetString("exdate"), ical.PropExceptionDates)
	return component, nil
}
