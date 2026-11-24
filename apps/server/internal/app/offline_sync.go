package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

const (
	syncChanges  = "sync_changes"
	syncReceipts = "sync_receipts"
)

var syncCollections = map[string]bool{"todos": true, "events": true, "lists": true, "calendars": true}

func EnsureOfflineSync(app core.App) error {
	for _, spec := range []struct {
		name    string
		fields  []core.Field
		indexes []string
	}{
		{syncChanges, []core.Field{&core.TextField{Name: "owner", Required: true}, &core.NumberField{Name: "revision", OnlyInt: true, Required: true}, &core.TextField{Name: "collection", Required: true}, &core.TextField{Name: "record_id", Required: true}, &core.BoolField{Name: "deleted"}, &core.JSONField{Name: "record"}}, []string{"owner, revision"}},
		{syncReceipts, []core.Field{&core.TextField{Name: "owner", Required: true}, &core.TextField{Name: "operation_id", Required: true}, &core.JSONField{Name: "result", Required: true}}, []string{"owner, operation_id"}},
	} {
		c, err := app.FindCollectionByNameOrId(spec.name)
		if err != nil {
			c = core.NewBaseCollection(spec.name)
		}
		for _, f := range spec.fields {
			if c.Fields.GetByName(f.GetName()) == nil {
				c.Fields.Add(f)
			}
		}
		for _, idx := range spec.indexes {
			if c.GetIndex("idx_"+spec.name+"_"+strings.ReplaceAll(idx, ", ", "_")) == "" {
				c.AddIndex("idx_"+spec.name+"_"+strings.ReplaceAll(idx, ", ", "_"), true, idx, "")
			}
		}
		if err := app.Save(c); err != nil {
			return err
		}
	}
	return nil
}

func registerOfflineSyncHooks(pb *pocketbase.PocketBase) {
	for name := range syncCollections {
		collection := name
		pb.OnRecordCreate(collection).BindFunc(func(e *core.RecordEvent) error { return transactSyncWrite(e, collection, false) })
		pb.OnRecordUpdate(collection).BindFunc(func(e *core.RecordEvent) error { return transactSyncWrite(e, collection, false) })
		pb.OnRecordDelete(collection).BindFunc(func(e *core.RecordEvent) error { return transactSyncWrite(e, collection, true) })
	}
}

func transactSyncWrite(e *core.RecordEvent, collection string, deleted bool) error {
	return e.App.RunInTransaction(func(tx core.App) error {
		original := e.App
		e.App = tx
		err := e.Next()
		e.App = original
		if err != nil {
			return err
		}
		if e.Record.GetString("owner") == "" {
			return nil
		}
		_, err = appendSyncChangeTx(tx, e.Record.GetString("owner"), collection, e.Record.Id, e.Record.PublicExport(), deleted)
		if err == nil && collection == "events" && e.Record.GetString("recurrence_parent") != "" {
			if master, findErr := tx.FindRecordById("events", e.Record.GetString("recurrence_parent")); findErr == nil && master.GetString("owner") == e.Record.GetString("owner") {
				_, err = appendSyncChangeTx(tx, master.GetString("owner"), "events", master.Id, master.PublicExport(), false)
			}
		}
		return err
	})
}

func appendSyncChangeTx(app core.App, owner, collection, id string, record map[string]any, deleted bool) (int, error) {
	latest, err := app.FindRecordsByFilter(syncChanges, "owner = {:owner}", "-revision", 1, 0, dbx.Params{"owner": owner})
	if err != nil {
		return 0, err
	}
	revision := 1
	if len(latest) > 0 {
		revision = latest[0].GetInt("revision") + 1
	}
	c, err := app.FindCollectionByNameOrId(syncChanges)
	if err != nil {
		return 0, err
	}
	change := core.NewRecord(c)
	change.Set("owner", owner)
	change.Set("revision", revision)
	change.Set("collection", collection)
	change.Set("record_id", id)
	change.Set("deleted", deleted)
	if !deleted {
		change.Set("record", record)
	}
	if err := app.Save(change); err != nil {
		return 0, err
	}
	return revision, nil
}

type syncChange struct {
	Collection string         `json:"collection"`
	ID         string         `json:"id"`
	Revision   int            `json:"revision"`
	Deleted    bool           `json:"deleted"`
	Record     map[string]any `json:"record"`
}
type syncPullResponse struct {
	Cursor  int          `json:"cursor"`
	Changes []syncChange `json:"changes"`
}
type syncOperation struct {
	OperationID  string         `json:"operationId"`
	Collection   string         `json:"collection"`
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	BaseRevision int            `json:"baseRevision"`
	Base         map[string]any `json:"base"`
	Patch        map[string]any `json:"patch"`
}
type syncResult struct {
	OperationID    string         `json:"operationId"`
	Status         string         `json:"status"`
	Revision       int            `json:"revision"`
	Record         map[string]any `json:"record"`
	MasterRevision int            `json:"masterRevision,omitempty"`
	MasterRecord   map[string]any `json:"masterRecord,omitempty"`
	Deleted        bool           `json:"deleted,omitempty"`
	Message        string         `json:"message,omitempty"`
}

func registerOfflineSyncAPI(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "syncPull", Method: http.MethodGet, Path: "/api/sync/pull", Summary: "Pull changes for the current user", Tags: []string{"Offline sync"}, Security: []map[string][]string{{"pocketbaseAuth": {}}}}, func(_ context.Context, input *struct {
		Cursor int `query:"cursor"`
	}) (*struct{ Body syncPullResponse }, error) {
		return nil, nil
	})
	huma.Register(api, huma.Operation{OperationID: "syncPush", Method: http.MethodPost, Path: "/api/sync/push", Summary: "Push offline edits", Tags: []string{"Offline sync"}, Security: []map[string][]string{{"pocketbaseAuth": {}}}}, func(_ context.Context, input *struct {
		Body struct {
			Operations []syncOperation `json:"operations"`
		}
	}) (*struct {
		Body struct {
			Results []syncResult `json:"results"`
			Cursor  int          `json:"cursor"`
		}
	}, error) {
		return nil, nil
	})
}

func registerOfflineSyncRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/sync/pull", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		cursor := 0
		if raw := event.Request.URL.Query().Get("cursor"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 {
				return event.BadRequestError("invalid cursor", nil)
			}
			cursor = parsed
		}
		if cursor == 0 {
			if err := bootstrapSyncJournal(event.App, event.Auth.Id); err != nil {
				return event.InternalServerError("sync snapshot failed", err)
			}
		}
		rows, err := event.App.FindRecordsByFilter(syncChanges, "owner = {:owner} && revision > {:cursor}", "revision", 500, 0, dbx.Params{"owner": event.Auth.Id, "cursor": cursor})
		if err != nil {
			return event.InternalServerError("sync pull failed", err)
		}
		changes := make([]syncChange, 0, len(rows))
		for _, row := range rows {
			c := syncChange{Collection: row.GetString("collection"), ID: row.GetString("record_id"), Revision: row.GetInt("revision"), Deleted: row.GetBool("deleted")}
			if !c.Deleted {
				_ = row.UnmarshalJSONField("record", &c.Record)
			}
			changes = append(changes, c)
		}
		out := syncPullResponse{Cursor: cursor, Changes: changes}
		if len(changes) > 0 {
			out.Cursor = changes[len(changes)-1].Revision
		}
		return event.JSON(http.StatusOK, out)
	})
	e.Router.POST("/api/sync/push", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		var body struct {
			Operations []syncOperation `json:"operations"`
		}
		if err := event.BindBody(&body); err != nil {
			return event.BadRequestError("invalid sync request", err)
		}
		if len(body.Operations) > 100 {
			return event.BadRequestError("batch exceeds 100 operations", nil)
		}
		results := make([]syncResult, 0, len(body.Operations))
		for _, op := range body.Operations {
			result, err := applySyncOperation(event.App, event.Auth.Id, op)
			if err != nil {
				results = append(results, syncResult{OperationID: op.OperationID, Status: "error", Message: err.Error()})
			} else {
				results = append(results, result)
			}
		}
		cursor, err := syncCursor(event.App, event.Auth.Id)
		if err != nil {
			return event.InternalServerError("sync cursor failed", err)
		}
		return event.JSON(http.StatusOK, map[string]any{"results": results, "cursor": cursor})
	})
}

func bootstrapSyncJournal(app core.App, owner string) error {
	return app.RunInTransaction(func(tx core.App) error {
		for name := range syncCollections {
			records, err := tx.FindRecordsByFilter(name, "owner = {:owner}", "id", 0, 0, dbx.Params{"owner": owner})
			if err != nil {
				return err
			}
			for _, r := range records {
				seen, err := tx.FindRecordsByFilter(syncChanges, "owner = {:owner} && collection = {:collection} && record_id = {:id}", "", 1, 0, dbx.Params{"owner": owner, "collection": name, "id": r.Id})
				if err != nil {
					return err
				}
				if len(seen) == 0 {
					if _, err := appendSyncChangeTx(tx, owner, name, r.Id, r.PublicExport(), false); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func applySyncOperation(app core.App, owner string, op syncOperation) (syncResult, error) {
	if op.OperationID == "" || !syncCollections[op.Collection] || (op.Kind != "create" && op.Kind != "update" && op.Kind != "delete" && op.Kind != "override" && op.Kind != "cancel") {
		return syncResult{}, fmt.Errorf("invalid operation")
	}
	var result syncResult
	err := app.RunInTransaction(func(tx core.App) error {
		prior, err := tx.FindRecordsByFilter(syncReceipts, "owner = {:owner} && operation_id = {:operation}", "", 1, 0, dbx.Params{"owner": owner, "operation": op.OperationID})
		if err != nil {
			return err
		}
		if len(prior) > 0 {
			return prior[0].UnmarshalJSONField("result", &result)
		}
		result = syncResult{OperationID: op.OperationID, Status: "applied"}
		if op.Kind == "override" || op.Kind == "cancel" {
			if op.Collection != "events" || op.BaseRevision != latestRecordRevision(tx, owner, "events", op.ID) {
				result.Status = "conflict"
				result.Message = "recurring event changed since the base revision"
				if master, err := tx.FindRecordById("events", op.ID); err == nil && master.GetString("owner") == owner {
					result.Record = master.PublicExport()
					result.Revision = latestRecordRevision(tx, owner, "events", op.ID)
				}
			} else {
				rawID, _ := op.Patch["recurrenceId"].(string)
				record, master, err := prepareRecurrenceOverride(tx, owner, op.ID, rawID)
				if err != nil {
					result.Status = "error"
					result.Message = err.Error()
				} else if op.Kind == "override" {
					values, _ := op.Patch["values"].(map[string]any)
					applyRecurrenceValues(record, master, nil)
					applyRecurrenceValues(record, master, values)
					record.Set("recurrence_cancelled", false)
					if err := tx.Save(record); err != nil {
						return err
					}
				} else {
					applyRecurrenceValues(record, master, nil)
					record.Set("recurrence_cancelled", true)
					record.Set("status", "CANCELLED")
					if err := tx.Save(record); err != nil {
						return err
					}
				}
				if result.Status == "applied" {
					result.Record = record.PublicExport()
					result.Revision = latestRecordRevision(tx, owner, "events", record.Id)
					result.MasterRecord = master.PublicExport()
					result.MasterRevision = latestRecordRevision(tx, owner, "events", master.Id)
				}
			}
			if err := saveSyncReceipt(tx, owner, op.OperationID, result); err != nil {
				return err
			}
			return nil
		}
		current, findErr := tx.FindRecordById(op.Collection, op.ID)
		exists := findErr == nil
		if exists && current.GetString("owner") != owner {
			result.Status = "error"
			result.Message = "record not found"
		} else if op.Kind == "create" {
			if exists {
				result.Status = "conflict"
				result.Message = "record already exists"
				result.Record = current.PublicExport()
				result.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
			} else {
				col, err := tx.FindCollectionByNameOrId(op.Collection)
				if err != nil {
					return err
				}
				current = core.NewRecord(col)
				current.Id = op.ID
				current.Set("owner", owner)
				applyPatch(current, op.Patch)
				if err := validateSyncRecord(tx, owner, op.Collection, current); err != nil {
					result.Status = "error"
					result.Message = err.Error()
				} else if err := tx.Save(current); err != nil {
					return err
				}
			}
		} else if !exists {
			result.Status = "conflict"
			result.Message = "record was deleted or does not exist"
			result.Deleted = latestRecordDeleted(tx, owner, op.Collection, op.ID)
			result.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
		} else if op.Kind == "delete" {
			if op.BaseRevision != latestRecordRevision(tx, owner, op.Collection, op.ID) {
				result.Status = "conflict"
				result.Message = "delete conflicts with a newer edit"
				result.Record = current.PublicExport()
				result.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
			} else if (op.Collection == "lists" || op.Collection == "calendars") && syncContainerHasEntries(tx, op.Collection, current.Id) {
				result.Status = "error"
				result.Message = "non-empty containers must be archived"
			} else if err := tx.Delete(current); err != nil {
				return err
			} else {
				if op.Collection == "events" && current.GetString("recurrence_parent") == "" {
					overrides, err := tx.FindRecordsByFilter("events", "owner = {:owner} && recurrence_parent = {:parent}", "", 0, 0, dbx.Params{"owner": owner, "parent": current.Id})
					if err != nil {
						return err
					}
					for _, override := range overrides {
						if err := tx.Delete(override); err != nil {
							return err
						}
					}
				}
				result.Deleted = true
			}
		} else {
			latest := latestRecordRevision(tx, owner, op.Collection, op.ID)
			patch := op.Patch
			serverBase, baseErr := syncRecordAtRevision(tx, owner, op.Collection, op.ID, op.BaseRevision)
			if baseErr != nil || !syncMapsEqual(serverBase, op.Base) {
				result.Status = "error"
				result.Message = "base snapshot does not match the supplied revision"
			}
			if result.Status == "applied" && latest != op.BaseRevision {
				merged, conflict := mergeSyncPatch(serverBase, current.PublicExport(), patch)
				if conflict != "" {
					result.Status = "conflict"
					result.Message = conflict
				} else {
					patch = merged
				}
			}
			if result.Status == "applied" {
				applyPatch(current, patch)
				if err := validateSyncRecord(tx, owner, op.Collection, current); err != nil {
					result.Status = "error"
					result.Message = err.Error()
				} else if err := tx.Save(current); err != nil {
					return err
				}
			}
		}
		if result.Status == "applied" {
			result.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
			if !result.Deleted {
				result.Record = current.PublicExport()
			}
		}
		if (result.Status == "conflict" || result.Status == "error") && result.Record == nil {
			if current, err := tx.FindRecordById(op.Collection, op.ID); err == nil && current.GetString("owner") == owner {
				result.Record = current.PublicExport()
				result.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
			} else if latestRecordDeleted(tx, owner, op.Collection, op.ID) {
				result.Deleted = true
				result.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
			}
		}
		if err := saveSyncReceipt(tx, owner, op.OperationID, result); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		failed := syncResult{OperationID: op.OperationID, Status: "error", Message: err.Error()}
		receiptErr := app.RunInTransaction(func(tx core.App) error {
			prior, findErr := tx.FindRecordsByFilter(syncReceipts, "owner = {:owner} && operation_id = {:operation}", "", 1, 0, dbx.Params{"owner": owner, "operation": op.OperationID})
			if findErr != nil {
				return findErr
			}
			if len(prior) > 0 {
				return prior[0].UnmarshalJSONField("result", &failed)
			}
			if current, findErr := tx.FindRecordById(op.Collection, op.ID); findErr == nil && current.GetString("owner") == owner {
				failed.Record = current.PublicExport()
				failed.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
			} else if latestRecordDeleted(tx, owner, op.Collection, op.ID) {
				failed.Deleted = true
				failed.Revision = latestRecordRevision(tx, owner, op.Collection, op.ID)
			}
			return saveSyncReceipt(tx, owner, op.OperationID, failed)
		})
		return failed, receiptErr
	}
	return result, nil
}

func latestRecordRevision(app core.App, owner, collection, id string) int {
	rows, err := app.FindRecordsByFilter(syncChanges, "owner = {:owner} && collection = {:collection} && record_id = {:id}", "-revision", 1, 0, dbx.Params{"owner": owner, "collection": collection, "id": id})
	if err != nil || len(rows) == 0 {
		return 0
	}
	return rows[0].GetInt("revision")
}

func latestRecordDeleted(app core.App, owner, collection, id string) bool {
	rows, err := app.FindRecordsByFilter(syncChanges, "owner = {:owner} && collection = {:collection} && record_id = {:id}", "-revision", 1, 0, dbx.Params{"owner": owner, "collection": collection, "id": id})
	return err == nil && len(rows) > 0 && rows[0].GetBool("deleted")
}

func validateSyncRecord(app core.App, owner, collection string, record *core.Record) error {
	if record.GetString("owner") != owner {
		return fmt.Errorf("owner cannot be changed")
	}
	if collection == "todos" || collection == "events" {
		if err := validateEntryContainer(app, collection, record.GetString(containerField(collection)), owner); err != nil {
			return err
		}
	}
	return nil
}

func syncContainerHasEntries(app core.App, collection, id string) bool {
	entries, err := app.FindRecordsByFilter(nameForEntryCollection(collection), containerField(collection)+" = {:container}", "", 1, 0, dbx.Params{"container": id})
	return err != nil || len(entries) > 0
}

func syncRecordAtRevision(app core.App, owner, collection, id string, revision int) (map[string]any, error) {
	if revision < 1 {
		return nil, fmt.Errorf("invalid base revision")
	}
	rows, err := app.FindRecordsByFilter(syncChanges, "owner = {:owner} && collection = {:collection} && record_id = {:id} && revision = {:revision} && deleted = false", "", 1, 0, dbx.Params{"owner": owner, "collection": collection, "id": id, "revision": revision})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("base revision not found")
	}
	var record map[string]any
	if err := rows[0].UnmarshalJSONField("record", &record); err != nil {
		return nil, err
	}
	return record, nil
}

func syncMapsEqual(a, b map[string]any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func syncValuesEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
func syncCursor(app core.App, owner string) (int, error) {
	rows, err := app.FindRecordsByFilter(syncChanges, "owner = {:owner}", "-revision", 1, 0, dbx.Params{"owner": owner})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0].GetInt("revision"), nil
}
func saveSyncReceipt(app core.App, owner, id string, result syncResult) error {
	c, err := app.FindCollectionByNameOrId(syncReceipts)
	if err != nil {
		return err
	}
	r := core.NewRecord(c)
	r.Set("owner", owner)
	r.Set("operation_id", id)
	r.Set("result", result)
	return app.Save(r)
}
func applyPatch(r *core.Record, patch map[string]any) {
	for k, v := range patch {
		if k != "id" && k != "owner" && k != "created" && k != "updated" {
			r.Set(k, v)
		}
	}
}
func mergeSyncPatch(base, current, patch map[string]any) (map[string]any, string) {
	if base == nil {
		return nil, "base snapshot required for merge"
	}
	groups := [][]string{{"start_date", "start_local", "end_date", "end_local", "timezone", "time_mode", "duration", "all_day"}, {"completed", "status", "completed_at", "percent_complete", "due_date", "due_local", "start_date", "start_local"}, {"rrule", "exdate", "recurrence_parent", "recurrence_id", "recurrence_cancelled"}, {"list", "calendar"}}
	merged := map[string]any{}
	for k, v := range patch {
		merged[k] = v
	}
	handled := map[string]bool{}
	for _, group := range groups {
		local, remote := false, false
		for _, k := range group {
			handled[k] = true
			if v, ok := patch[k]; ok && !syncValuesEqual(v, base[k]) {
				local = true
			}
			if !syncValuesEqual(current[k], base[k]) {
				remote = true
			}
		}
		if local && remote && !equalGroup(base, current, patch, group) {
			return nil, "conflicting edits to linked fields: " + strings.Join(group, ",")
		}
	}
	keys := map[string]bool{}
	for k := range patch {
		keys[k] = true
	}
	for k := range current {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	for _, k := range ordered {
		if handled[k] {
			continue
		}
		local, has := patch[k]
		remote := !syncValuesEqual(current[k], base[k])
		if has && !syncValuesEqual(local, base[k]) && remote && !syncValuesEqual(local, current[k]) {
			return nil, "conflicting edit to field: " + k
		}
	}
	return merged, ""
}
func equalGroup(base, current, patch map[string]any, keys []string) bool {
	for _, k := range keys {
		l, ok := patch[k]
		if !ok {
			l = base[k]
		}
		if !syncValuesEqual(l, current[k]) {
			return false
		}
	}
	return true
}

var _ = json.Valid
