package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	maxCalendarImportSize = 10 << 20
	calendarDataVersion   = 1
)

type storedCalendarData struct {
	Version    int                    `json:"version"`
	Properties map[string][]ical.Prop `json:"properties,omitempty"`
	Components []*ical.Component      `json:"components,omitempty"`
}

func registerCalendarIORoutes(e *core.ServeEvent) {
	e.Router.POST("/api/calendars/import", importCalendar)
	e.Router.GET("/api/calendars/{id}/export.ics", exportCalendar)
}

func importCalendar(event *core.RequestEvent) error {
	if event.Auth == nil {
		return event.UnauthorizedError("authentication required", nil)
	}
	event.Request.Body = http.MaxBytesReader(event.Response, event.Request.Body, maxCalendarImportSize)
	if err := event.Request.ParseMultipartForm(maxCalendarImportSize); err != nil {
		return event.BadRequestError("calendar file is too large or invalid", err)
	}
	calendarID := strings.TrimSpace(event.Request.FormValue("calendar"))
	listID := strings.TrimSpace(event.Request.FormValue("list"))
	file, _, err := event.Request.FormFile("file")
	if err != nil {
		return event.BadRequestError("calendar file is required", err)
	}
	defer file.Close()

	decoded, err := ical.NewDecoder(file).Decode()
	if err != nil {
		return event.BadRequestError("invalid iCalendar file", err)
	}
	imported := 0
	for _, component := range decoded.Children {
		switch component.Name {
		case ical.CompEvent:
			if calendarID == "" {
				return event.BadRequestError("calendar is required for VEVENT", nil)
			}
			err = saveImportedEvent(event.App, event.Auth.Id, calendarID, component)
		case ical.CompToDo:
			if listID == "" {
				return event.BadRequestError("list is required for VTODO", nil)
			}
			err = saveImportedTodo(event.App, event.Auth.Id, listID, component)
		default:
			continue
		}
		if err != nil {
			return event.BadRequestError("could not import calendar component", err)
		}
		imported++
	}
	return event.JSON(http.StatusOK, map[string]int{"imported": imported})
}

// Duplicate policy: an import updates the record with the same owner, component
// type, and UID. UIDs owned by another user or used by another type are distinct.
func saveImportedEvent(app core.App, userID, calendarID string, component *ical.Component) error {
	return saveImportedEventForDAV(app, userID, calendarID, component, nil, "")
}

func saveImportedEventForDAV(app core.App, userID, calendarID string, component *ical.Component, preferred *core.Record, davHref string) error {
	if err := validateImportCalendar(app, userID, calendarID); err != nil {
		return err
	}
	uid, err := validateImportedComponent(component, ical.CompEvent)
	if err != nil {
		return err
	}
	startProp := component.Props.Get(ical.PropDateTimeStart)
	if startProp == nil {
		return fmt.Errorf("VEVENT %q is missing DTSTART", uid)
	}
	start, startLocal, startMode, startZone, err := importedTemporal(startProp, "VEVENT", uid)
	if err != nil {
		return err
	}

	var record *core.Record
	if preferred != nil {
		record = preferred
	} else if recurrenceProp := component.Props.Get(ical.PropRecurrenceID); recurrenceProp != nil {
		recurrenceTime, err := recurrenceProp.DateTime(time.Local)
		if err != nil {
			return fmt.Errorf("VEVENT %q has invalid RECURRENCE-ID: %w", uid, err)
		}
		masters, err := app.FindRecordsByFilter("events", "owner = {:owner} && uid = {:uid} && recurrence_parent = ''", "", 1, 0, dbx.Params{"owner": userID, "uid": uid})
		if err != nil || len(masters) != 1 {
			return fmt.Errorf("recurring master %q not found", uid)
		}
		record, _, err = prepareRecurrenceOverride(app, userID, masters[0].Id, recurrenceID(recurrenceTime))
		if err != nil {
			return err
		}
	} else {
		record, err = importedRecord(app, "events", userID, uid)
		if err != nil {
			return err
		}
	}
	record.Set("calendar", calendarID)
	setCommonImportedFields(record, component)
	record.Set("location", calendarText(component, ical.PropLocation))
	record.Set("start_date", temporalIndex(start, startProp))
	record.Set("start_local", startLocal)
	record.Set("time_mode", startMode)
	record.Set("timezone", startZone)
	record.Set("all_day", startProp.ValueType() == ical.ValueDate)
	if endProp := component.Props.Get(ical.PropDateTimeEnd); endProp != nil {
		end, endLocal, _, _, err := importedTemporal(endProp, "VEVENT", uid)
		if err != nil {
			return err
		}
		record.Set("end_date", temporalIndex(end, endProp))
		record.Set("end_local", endLocal)
	} else {
		record.Set("end_date", nil)
		record.Set("end_local", "")
	}
	if component.Props.Get(ical.PropRecurrenceID) != nil {
		record.Set("recurrence_cancelled", strings.EqualFold(calendarText(component, ical.PropStatus), "CANCELLED"))
	}
	record.Set("duration", calendarRaw(component, ical.PropDuration))
	if davHref != "" {
		record.Set("dav_href", davHref)
	}
	return app.Save(record)
}

func saveImportedTodo(app core.App, userID, listID string, component *ical.Component) error {
	return saveImportedTodoForTarget(app, userID, "list", listID, component, nil, "")
}

func saveImportedTodoForDAV(app core.App, userID, calendarID string, component *ical.Component, preferred *core.Record, davHref string) error {
	return saveImportedTodoForTarget(app, userID, "calendar", calendarID, component, preferred, davHref)
}

func saveImportedTodoForTarget(app core.App, userID, targetField, targetID string, component *ical.Component, preferred *core.Record, davHref string) error {
	if targetField == "list" {
		if err := validateImportList(app, userID, targetID); err != nil {
			return err
		}
	} else if err := validateImportCalendar(app, userID, targetID); err != nil {
		return err
	}
	uid, err := validateImportedComponent(component, ical.CompToDo)
	if err != nil {
		return err
	}
	var record *core.Record
	if preferred != nil {
		record = preferred
	} else {
		record, err = importedRecord(app, "todos", userID, uid)
		if err != nil {
			return err
		}
	}
	record.Set(targetField, targetID)
	if targetField == "calendar" && record.GetString("list") == "" {
		lists, findErr := app.FindRecordsByFilter("lists", "owner = {:owner} && is_default = true", "", 1, 0, dbx.Params{"owner": userID})
		if findErr != nil {
			return findErr
		}
		if len(lists) == 1 {
			record.Set("list", lists[0].Id)
		}
	}
	setCommonImportedFields(record, component)

	for _, item := range []struct {
		property string
		field    string
	}{
		{ical.PropDateTimeStart, "start_date"},
		{ical.PropDue, "due_date"},
		{ical.PropCompleted, "completed_at"},
	} {
		prop := component.Props.Get(item.property)
		if prop == nil {
			record.Set(item.field, nil)
			if item.field == "start_date" || item.field == "due_date" {
				record.Set(strings.TrimSuffix(item.field, "_date")+"_local", "")
			}
			continue
		}
		value, local, mode, zone, err := importedTemporal(prop, "VTODO", uid)
		if err != nil {
			return err
		}
		record.Set(item.field, temporalIndex(value, prop))
		if item.field == "start_date" || item.field == "due_date" {
			record.Set(strings.TrimSuffix(item.field, "_date")+"_local", local)
		}
		if zone != "" {
			record.Set("timezone", zone)
		}
		if mode != "" {
			record.Set("time_mode", mode)
		}
	}
	record.Set("duration", calendarRaw(component, ical.PropDuration))
	record.Set("percent_complete", calendarRaw(component, ical.PropPercentComplete))
	record.Set("priority", calendarRaw(component, ical.PropPriority))
	record.Set("completed", component.Props.Get(ical.PropCompleted) != nil || strings.EqualFold(calendarText(component, ical.PropStatus), "COMPLETED"))
	if davHref != "" {
		record.Set("dav_href", davHref)
	}
	return app.Save(record)
}

func validateImportCalendar(app core.App, userID, calendarID string) error {
	calendar, err := app.FindRecordById("calendars", calendarID)
	if err != nil || calendar.GetString("owner") != userID {
		return fmt.Errorf("calendar not found")
	}
	return nil
}

func validateImportList(app core.App, userID, listID string) error {
	list, err := app.FindRecordById("lists", listID)
	if err != nil || list.GetString("owner") != userID {
		return fmt.Errorf("list not found")
	}
	return nil
}

func validateImportedComponent(component *ical.Component, want string) (string, error) {
	if component == nil || component.Name != want {
		return "", fmt.Errorf("expected %s component", want)
	}
	uid := strings.TrimSpace(calendarText(component, ical.PropUID))
	if uid == "" {
		return "", fmt.Errorf("%s is missing UID", want)
	}
	if len(component.Props[ical.PropUID]) != 1 {
		return "", fmt.Errorf("%s %q has multiple UID properties", want, uid)
	}
	for _, name := range []string{ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropDue, ical.PropCompleted} {
		if len(component.Props[name]) > 1 {
			return "", fmt.Errorf("%s %q has multiple %s properties", want, uid, name)
		}
	}
	if want == ical.CompEvent && component.Props.Get(ical.PropDateTimeEnd) != nil && component.Props.Get(ical.PropDuration) != nil {
		return "", fmt.Errorf("VEVENT %q has both DTEND and DURATION", uid)
	}
	if want == ical.CompToDo && component.Props.Get(ical.PropDue) != nil && component.Props.Get(ical.PropDuration) != nil {
		return "", fmt.Errorf("VTODO %q has both DUE and DURATION", uid)
	}
	if want == ical.CompToDo && component.Props.Get(ical.PropDuration) != nil && component.Props.Get(ical.PropDateTimeStart) == nil {
		return "", fmt.Errorf("VTODO %q has DURATION without DTSTART", uid)
	}
	if stamp := component.Props.Get(ical.PropDateTimeStamp); stamp != nil {
		if _, err := importedDateTime(stamp, want, uid); err != nil {
			return "", err
		}
	}
	return uid, nil
}

func importedDateTime(prop *ical.Prop, componentName, uid string) (time.Time, error) {
	value, _, _, _, err := importedTemporal(prop, componentName, uid)
	return value, err
}

func importedTemporal(prop *ical.Prop, componentName, uid string) (time.Time, string, string, string, error) {
	value, err := prop.DateTime(time.Local)
	if err != nil {
		return time.Time{}, "", "", "", fmt.Errorf("%s %q has invalid %s: %w", componentName, uid, prop.Name, err)
	}
	mode := "floating"
	zone := strings.TrimSpace(prop.Params.Get(ical.PropTimezoneID))
	if prop.ValueType() == ical.ValueDate {
		mode = "date"
	} else if strings.HasSuffix(prop.Value, "Z") {
		mode = "utc"
	} else if zone != "" {
		mode = "zoned"
	}
	return value, prop.Value, mode, zone, nil
}

func temporalIndex(value time.Time, prop *ical.Prop) time.Time {
	if prop.ValueType() == ical.ValueDate {
		return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	}
	return value.UTC()
}

func importedRecord(app core.App, collectionName, userID, uid string) (*core.Record, error) {
	records, err := app.FindRecordsByFilter(collectionName, "owner = {:owner} && uid = {:uid}", "", 2, 0, dbx.Params{"owner": userID, "uid": uid})
	if err != nil {
		return nil, err
	}
	if len(records) > 1 {
		return nil, fmt.Errorf("%s UID %q is ambiguous: found %d owned records", collectionName, uid, len(records))
	}
	if len(records) == 1 {
		return records[0], nil
	}
	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(collection)
	record.Set("owner", userID)
	record.Set("uid", uid)
	return record, nil
}

func setCommonImportedFields(record *core.Record, component *ical.Component) {
	title := strings.TrimSpace(calendarText(component, ical.PropSummary))
	if title == "" {
		title = strings.TrimSpace(calendarText(component, ical.PropUID))
	}
	record.Set("title", title)
	record.Set("description", calendarText(component, ical.PropDescription))
	record.Set("status", calendarText(component, ical.PropStatus))
	record.Set("rrule", calendarRaw(component, ical.PropRecurrenceRule))
	record.Set("exdate", storeProperties(component, ical.PropExceptionDates, ical.PropRecurrenceDates))
	record.Set("categories", calendarRaw(component, ical.PropCategories))
	record.Set("organizer", storeProperties(component, ical.PropOrganizer))
	record.Set("attendees", storeProperties(component, ical.PropAttendee))
	record.Set("url", calendarRaw(component, ical.PropURL))
	record.Set("class", calendarText(component, ical.PropClass))
	record.Set("geo", calendarRaw(component, ical.PropGeo))
	record.Set("alarm", storeComponents(component.Children, ical.CompAlarm))

	stamp := time.Now().UTC()
	if prop := component.Props.Get(ical.PropDateTimeStamp); prop != nil {
		if parsed, err := prop.DateTime(time.Local); err == nil {
			stamp = parsed.UTC()
		}
	}
	record.Set("dtstamp", stamp)
}

func exportCalendar(event *core.RequestEvent) error {
	if event.Auth == nil {
		return event.UnauthorizedError("authentication required", nil)
	}
	calendar, err := event.App.FindRecordById("calendars", event.Request.PathValue("id"))
	if err != nil || calendar.GetString("owner") != event.Auth.Id {
		return event.NotFoundError("calendar not found", nil)
	}
	output, err := buildCalendarExport(event.App, event.Auth.Id, calendar.Id)
	if err != nil {
		return err
	}

	var body bytes.Buffer
	if err := ical.NewEncoder(&body).Encode(output); err != nil {
		return err
	}
	filename := strings.NewReplacer("/", "-", "\\", "-", `"`, "").Replace(calendar.GetString("name")) + ".ics"
	event.Response.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return event.Blob(http.StatusOK, "text/calendar; charset=utf-8", body.Bytes())
}

func buildCalendarExport(app core.App, userID, calendarID string) (*ical.Calendar, error) {
	events, err := app.FindRecordsByFilter("events", "owner = {:owner} && calendar = {:calendar}", "start_date", 0, 0, dbx.Params{"owner": userID, "calendar": calendarID})
	if err != nil {
		return nil, err
	}
	todos, err := app.FindRecordsByFilter("todos", "owner = {:owner} && (calendar = {:calendar} || (calendar = '' && list = {:calendar}))", "due_date", 0, 0, dbx.Params{"owner": userID, "calendar": calendarID})
	if err != nil {
		return nil, err
	}
	output := ical.NewCalendar()
	output.Props.SetText(ical.PropVersion, "2.0")
	output.Props.SetText(ical.PropProductID, "-//Taskboard//Calendar//EN")
	for _, record := range events {
		component, err := exportEvent(record)
		if err != nil {
			return nil, err
		}
		output.Children = append(output.Children, component)
	}
	for _, record := range todos {
		component, err := exportTodo(record)
		if err != nil {
			return nil, err
		}
		output.Children = append(output.Children, component)
	}
	return output, nil
}

func exportEvent(record *core.Record) (*ical.Component, error) {
	component := ical.NewComponent(ical.CompEvent)
	setCommonExportedFields(component, record)
	if rawID := record.GetString("recurrence_id"); rawID != "" {
		if recurrenceTime, err := parseRecurrenceID(rawID); err == nil {
			if record.GetBool("all_day") || record.GetString("time_mode") == "date" {
				component.Props.SetDate(ical.PropRecurrenceID, recurrenceTime)
			} else {
				component.Props.SetDateTime(ical.PropRecurrenceID, recurrenceTime)
			}
		}
	}
	setStoredDateTime(component, ical.PropDateTimeStart, record, "start_date", "start_local")
	setStoredDateTime(component, ical.PropDateTimeEnd, record, "end_date", "end_local")
	setOptionalRaw(component, ical.PropDuration, record.GetString("duration"))
	setOptionalText(component, ical.PropLocation, record.GetString("location"))
	return component, nil
}

func exportTodo(record *core.Record) (*ical.Component, error) {
	component := ical.NewComponent(ical.CompToDo)
	setCommonExportedFields(component, record)
	setOptionalDateTime(component, ical.PropDateTimeStart, record, "start_date")
	setOptionalDateTime(component, ical.PropDue, record, "due_date")
	setOptionalDateTime(component, ical.PropCompleted, record, "completed_at")
	setOptionalRaw(component, ical.PropDuration, record.GetString("duration"))
	setOptionalRaw(component, ical.PropPercentComplete, record.GetString("percent_complete"))
	setOptionalRaw(component, ical.PropPriority, record.GetString("priority"))
	return component, nil
}

func setCommonExportedFields(component *ical.Component, record *core.Record) {
	component.Props.SetText(ical.PropUID, record.GetString("uid"))
	component.Props.SetText(ical.PropSummary, record.GetString("title"))
	setOptionalText(component, ical.PropDescription, record.GetString("description"))
	setOptionalText(component, ical.PropStatus, record.GetString("status"))
	setOptionalRaw(component, ical.PropRecurrenceRule, record.GetString("rrule"))
	loadProperties(component, record.GetString("exdate"), ical.PropExceptionDates)
	setOptionalRaw(component, ical.PropCategories, record.GetString("categories"))
	loadProperties(component, record.GetString("organizer"), ical.PropOrganizer)
	loadProperties(component, record.GetString("attendees"), ical.PropAttendee)
	setOptionalRaw(component, ical.PropURL, record.GetString("url"))
	setOptionalText(component, ical.PropClass, record.GetString("class"))
	setOptionalRaw(component, ical.PropGeo, record.GetString("geo"))
	loadComponents(component, record.GetString("alarm"), ical.CompAlarm)

	stamp := record.GetDateTime("dtstamp").Time()
	if stamp.IsZero() {
		stamp = record.GetDateTime("updated").Time()
	}
	component.Props.SetDateTime(ical.PropDateTimeStamp, stamp.UTC())
}

func calendarText(component *ical.Component, name string) string {
	prop := component.Props.Get(name)
	if prop == nil {
		return ""
	}
	value, err := prop.Text()
	if err != nil {
		return prop.Value
	}
	return value
}

func calendarRaw(component *ical.Component, name string) string {
	props := component.Props.Values(name)
	values := make([]string, 0, len(props))
	for _, prop := range props {
		values = append(values, prop.Value)
	}
	return strings.Join(values, ",")
}

func storeProperties(component *ical.Component, names ...string) string {
	properties := make(map[string][]ical.Prop)
	for _, name := range names {
		if values := component.Props.Values(name); len(values) > 0 {
			properties[name] = values
		}
	}
	if len(properties) == 0 {
		return ""
	}
	encoded, err := json.Marshal(storedCalendarData{Version: calendarDataVersion, Properties: properties})
	if err != nil {
		return ""
	}
	return string(encoded)
}

func loadProperties(component *ical.Component, value, legacyName string) {
	if value == "" {
		return
	}
	var stored storedCalendarData
	if err := json.Unmarshal([]byte(value), &stored); err == nil && stored.Version == calendarDataVersion {
		for name, properties := range stored.Properties {
			component.Props[name] = append(component.Props[name], properties...)
		}
		return
	}
	setOptionalRaw(component, legacyName, value)
}

func storeComponents(children []*ical.Component, name string) string {
	components := make([]*ical.Component, 0)
	for _, child := range children {
		if child.Name == name {
			components = append(components, child)
		}
	}
	if len(components) == 0 {
		return ""
	}
	encoded, err := json.Marshal(storedCalendarData{Version: calendarDataVersion, Components: components})
	if err != nil {
		return ""
	}
	return string(encoded)
}

func loadComponents(component *ical.Component, value, name string) {
	if value == "" {
		return
	}
	var stored storedCalendarData
	if err := json.Unmarshal([]byte(value), &stored); err != nil || stored.Version != calendarDataVersion {
		return
	}
	for _, child := range stored.Components {
		if child != nil && child.Name == name {
			component.Children = append(component.Children, child)
		}
	}
}

func setOptionalText(component *ical.Component, name, value string) {
	if value != "" {
		component.Props.SetText(name, value)
	}
}

func setOptionalRaw(component *ical.Component, name, value string) {
	if value == "" {
		return
	}
	prop := ical.NewProp(name)
	prop.Value = value
	component.Props.Set(prop)
}

func setOptionalDateTime(component *ical.Component, name string, record *core.Record, field string) {
	setStoredDateTime(component, name, record, field, strings.TrimSuffix(field, "_date")+"_local")
}

func setStoredDateTime(component *ical.Component, name string, record *core.Record, field, localField string) {
	value := record.GetDateTime(field)
	if value.IsZero() && record.GetString(localField) == "" {
		return
	}
	if raw := record.GetString(localField); raw != "" {
		prop := ical.NewProp(name)
		prop.SetValueType(ical.ValueType(record.GetString("time_mode")))
		if record.GetBool("all_day") || record.GetString("time_mode") == "date" {
			prop.SetValueType(ical.ValueDate)
		} else {
			prop.SetValueType(ical.ValueDateTime)
			if zone := record.GetString("timezone"); zone != "" && record.GetString("time_mode") == "zoned" {
				prop.Params.Set(ical.PropTimezoneID, zone)
			}
		}
		prop.Value = raw
		component.Props.Set(prop)
		return
	}
	if record.GetBool("all_day") || record.GetString("time_mode") == "date" {
		component.Props.SetDate(name, value.Time())
		return
	}
	if zone := record.GetString("timezone"); zone != "" {
		if location, err := time.LoadLocation(zone); err == nil {
			component.Props.SetDateTime(name, value.Time().In(location))
			return
		}
	}
	component.Props.SetDateTime(name, value.Time().UTC())
}
