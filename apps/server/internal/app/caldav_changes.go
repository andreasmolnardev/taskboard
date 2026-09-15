package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

const calDAVChangeCollection = "caldav_changes"

func EnsureCalDAVChanges(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(calDAVChangeCollection)
	if err != nil {
		collection = core.NewBaseCollection(calDAVChangeCollection)
	}
	for _, field := range []core.Field{
		&core.TextField{Name: "owner", Required: true},
		&core.TextField{Name: "calendar", Required: true},
		&core.TextField{Name: "href", Required: true},
		&core.TextField{Name: "resource_kind", Required: true},
		&core.TextField{Name: "resource_id"},
		&core.TextField{Name: "etag"},
		&core.NumberField{Name: "sequence", OnlyInt: true, Required: true},
		&core.BoolField{Name: "deleted"},
		&core.DateField{Name: "changed_at", Required: true},
	} {
		if collection.Fields.GetByName(field.GetName()) == nil {
			collection.Fields.Add(field)
		}
	}
	ownerRule := "@request.auth.id = owner"
	collection.ListRule, collection.ViewRule, collection.CreateRule, collection.UpdateRule, collection.DeleteRule = &ownerRule, &ownerRule, &ownerRule, &ownerRule, &ownerRule
	if collection.GetIndex("idx_caldav_changes_owner_sequence") == "" {
		collection.AddIndex("idx_caldav_changes_owner_sequence", false, "owner, sequence", "")
	}
	if collection.GetIndex("idx_caldav_changes_owner_href") == "" {
		collection.AddIndex("idx_caldav_changes_owner_href", false, "owner, href", "")
	}
	return app.Save(collection)
}

func registerCalDAVChangeHooks(pb *pocketbase.PocketBase) {
	for _, collection := range []string{"events", "todos"} {
		name := collection
		pb.OnRecordAfterCreateSuccess(name).BindFunc(func(e *core.RecordEvent) error { return appendCalDAVChange(e.App, e.Record, name, false) })
		pb.OnRecordAfterUpdateSuccess(name).BindFunc(func(e *core.RecordEvent) error { return appendCalDAVChange(e.App, e.Record, name, false) })
		pb.OnRecordAfterDeleteSuccess(name).BindFunc(func(e *core.RecordEvent) error { return appendCalDAVChange(e.App, e.Record, name, true) })
	}
}

func appendCalDAVChange(app core.App, record *core.Record, kind string, deleted bool) error {
	owner := record.GetString("owner")
	calendarID := resourceCalendarID(record, kind)
	if owner == "" || calendarID == "" {
		return nil
	}

	oldCalendarID := ""
	if !deleted && record.Original() != nil {
		oldCalendarID = resourceCalendarID(record.Original(), kind)
		if record.Original().GetString("owner") != owner {
			oldCalendarID = ""
		}
		if oldCalendarID == calendarID {
			oldCalendarID = ""
		}
	}
	return app.RunInTransaction(func(txApp core.App) error {
		sequence := 1
		last, err := txApp.FindRecordsByFilter(calDAVChangeCollection, "owner = {:owner}", "-sequence", 1, 0, dbx.Params{"owner": owner})
		if err != nil {
			return err
		}
		if len(last) > 0 {
			sequence = last[0].GetInt("sequence") + 1
		}
		if oldCalendarID != "" {
			oldResource, err := makeCalDAVResource(record, kind, owner, oldCalendarID)
			if err != nil {
				return err
			}
			sequence, err = saveCalDAVChange(txApp, oldResource, kind, owner, oldCalendarID, true, sequence)
			if err != nil {
				return err
			}
		}
		resource, err := makeCalDAVResource(record, kind, owner, calendarID)
		if err != nil {
			return err
		}
		_, err = saveCalDAVChange(txApp, resource, kind, owner, calendarID, deleted, sequence)
		return err
	})
}

func saveCalDAVChange(app core.App, resource calDAVResource, kind, owner, calendarID string, deleted bool, sequence int) (int, error) {
	collection, err := app.FindCollectionByNameOrId(calDAVChangeCollection)
	if err != nil {
		return sequence, err
	}
	if deleted {
		resource.etag = ""
	}
	change := core.NewRecord(collection)
	change.Set("owner", owner)
	change.Set("calendar", calendarID)
	change.Set("href", resource.href)
	change.Set("resource_kind", kind)
	change.Set("resource_id", resource.record.Id)
	change.Set("etag", resource.etag)
	change.Set("sequence", sequence)
	change.Set("deleted", deleted)
	change.Set("changed_at", time.Now().UTC())
	if err := app.Save(change); err != nil {
		return sequence, err
	}
	return sequence + 1, nil
}

func calDAVToken(sequence int) string { return "" + strconv.Itoa(sequence) }

func parseCalDAVToken(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid sync token")
	}
	return value, nil
}

func currentCalDAVToken(app core.App, owner, calendarID string) (string, error) {
	records, err := app.FindRecordsByFilter(calDAVChangeCollection, "owner = {:owner} && calendar = {:calendar}", "-sequence", 1, 0, dbx.Params{"owner": owner, "calendar": calendarID})
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return calDAVToken(0), nil
	}
	return calDAVToken(records[0].GetInt("sequence")), nil
}

func calDAVChangesSince(app core.App, owner, calendarID string, token, limit int) ([]*core.Record, error) {
	return app.FindRecordsByFilter(calDAVChangeCollection, "owner = {:owner} && calendar = {:calendar} && sequence > {:token}", "sequence", limit, 0, dbx.Params{"owner": owner, "calendar": calendarID, "token": token})
}
