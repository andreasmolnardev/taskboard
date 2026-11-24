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

const cardDAVChangeCollection = "carddav_changes"

func EnsureCardDAVChanges(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(cardDAVChangeCollection)
	if err != nil {
		collection = core.NewBaseCollection(cardDAVChangeCollection)
	}
	for _, field := range []core.Field{
		&core.TextField{Name: "owner", Required: true},
		&core.TextField{Name: "address_book", Required: true},
		&core.TextField{Name: "href", Required: true},
		&core.TextField{Name: "etag"},
		&core.NumberField{Name: "sequence", OnlyInt: true, Required: true},
		&core.BoolField{Name: "deleted"},
		&core.DateField{Name: "changed_at", Required: true},
	} {
		if collection.Fields.GetByName(field.GetName()) == nil {
			collection.Fields.Add(field)
		}
	}
	rule := "@request.auth.id = owner"
	collection.ListRule, collection.ViewRule, collection.CreateRule, collection.UpdateRule, collection.DeleteRule = &rule, &rule, &rule, &rule, &rule
	if collection.GetIndex("idx_carddav_changes_owner_sequence") == "" {
		collection.AddIndex("idx_carddav_changes_owner_sequence", false, "owner, sequence", "")
	}
	if collection.GetIndex("idx_carddav_changes_owner_book_href") == "" {
		collection.AddIndex("idx_carddav_changes_owner_book_href", false, "owner, address_book, href", "")
	}
	return app.Save(collection)
}

func registerCardDAVChangeHooks(pb *pocketbase.PocketBase) {
	pb.OnRecordAfterCreateSuccess("contacts").BindFunc(func(e *core.RecordEvent) error {
		if err := appendCardDAVChange(e.App, e.Record, false); err != nil {
			return err
		}
		return e.Next()
	})
	pb.OnRecordAfterUpdateSuccess("contacts").BindFunc(func(e *core.RecordEvent) error {
		if err := appendCardDAVChange(e.App, e.Record, false); err != nil {
			return err
		}
		return e.Next()
	})
	pb.OnRecordAfterDeleteSuccess("contacts").BindFunc(func(e *core.RecordEvent) error {
		if err := appendCardDAVChange(e.App, e.Record, true); err != nil {
			return err
		}
		return e.Next()
	})
}

func appendCardDAVChange(app core.App, contact *core.Record, deleted bool) error {
	owner := contact.GetString("owner")
	book := contact.GetString("address_book")
	if owner == "" || book == "" {
		return nil
	}
	href := contactHref(contact)
	etag := ""
	if !deleted {
		etag = contactETag(contactVCard(contact))
	}
	return app.RunInTransaction(func(txApp core.App) error {
		sequence := 1
		last, err := txApp.FindRecordsByFilter(cardDAVChangeCollection, "owner = {:owner}", "-sequence", 1, 0, dbx.Params{"owner": owner})
		if err != nil {
			return err
		}
		if len(last) > 0 {
			sequence = last[0].GetInt("sequence") + 1
		}
		collection, err := txApp.FindCollectionByNameOrId(cardDAVChangeCollection)
		if err != nil {
			return err
		}
		change := core.NewRecord(collection)
		change.Set("owner", owner)
		change.Set("address_book", book)
		change.Set("href", href)
		change.Set("etag", etag)
		change.Set("sequence", sequence)
		change.Set("deleted", deleted)
		change.Set("changed_at", time.Now().UTC())
		return txApp.Save(change)
	})
}

func currentCardDAVToken(app core.App, owner, book string) (string, error) {
	records, err := app.FindRecordsByFilter(cardDAVChangeCollection, "owner = {:owner} && address_book = {:book}", "-sequence", 1, 0, dbx.Params{"owner": owner, "book": book})
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "0", nil
	}
	return strconv.Itoa(records[0].GetInt("sequence")), nil
}

func parseCardDAVToken(raw string) (int, error) {
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

func cardDAVChangesSince(app core.App, owner, book string, token, limit int) ([]*core.Record, error) {
	return app.FindRecordsByFilter(cardDAVChangeCollection, "owner = {:owner} && address_book = {:book} && sequence > {:token}", "sequence", limit, 0, dbx.Params{"owner": owner, "book": book, "token": token})
}
