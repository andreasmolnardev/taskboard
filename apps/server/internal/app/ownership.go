package app

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// registerOwnershipHooks keeps browser-supplied owner and container values from
// crossing user or collection boundaries. PocketBase rules still protect reads;
// these hooks protect writes made through the SDK and custom routes.
func registerOwnershipHooks(pb *pocketbase.PocketBase) {
	for _, collection := range []string{"todos", "events"} {
		name := collection
		pb.OnRecordCreateRequest(name).BindFunc(func(e *core.RecordRequestEvent) error {
			if e.HasSuperuserAuth() {
				return e.Next()
			}
			if e.Auth == nil {
				return e.UnauthorizedError("authentication required", nil)
			}
			e.Record.Set("owner", e.Auth.Id)
			if err := validateEntryContainer(e.App, name, e.Record.GetString(containerField(name)), e.Auth.Id); err != nil {
				return e.BadRequestError(err.Error(), nil)
			}
			return e.Next()
		})
		pb.OnRecordUpdateRequest(name).BindFunc(func(e *core.RecordRequestEvent) error {
			if e.HasSuperuserAuth() {
				return e.Next()
			}
			if e.Auth == nil || e.Record.Original().GetString("owner") != e.Auth.Id {
				return e.ForbiddenError("record is not owned by the current user", nil)
			}
			e.Record.Set("owner", e.Auth.Id)
			if err := validateEntryContainer(e.App, name, e.Record.GetString(containerField(name)), e.Auth.Id); err != nil {
				return e.BadRequestError(err.Error(), nil)
			}
			return e.Next()
		})
	}

	registerContactOwnershipHooks(pb)

	for _, collection := range []string{"lists", "calendars"} {
		name := collection
		pb.OnRecordCreateRequest(name).BindFunc(func(e *core.RecordRequestEvent) error {
			if e.HasSuperuserAuth() {
				return e.Next()
			}
			if e.Auth == nil {
				return e.UnauthorizedError("authentication required", nil)
			}
			e.Record.Set("owner", e.Auth.Id)
			return e.Next()
		})
		pb.OnRecordUpdateRequest(name).BindFunc(func(e *core.RecordRequestEvent) error {
			if e.HasSuperuserAuth() {
				return e.Next()
			}
			if e.Auth == nil || e.Record.Original().GetString("owner") != e.Auth.Id {
				return e.ForbiddenError("record is not owned by the current user", nil)
			}
			e.Record.Set("owner", e.Auth.Id)
			return e.Next()
		})
		pb.OnRecordDeleteRequest(name).BindFunc(func(e *core.RecordRequestEvent) error {
			if e.HasSuperuserAuth() {
				return e.Next()
			}
			if e.Auth == nil || e.Record.GetString("owner") != e.Auth.Id {
				return e.ForbiddenError("record is not owned by the current user", nil)
			}
			field := containerField(nameForEntryCollection(name))
			entries, err := e.App.FindRecordsByFilter(nameForEntryCollection(name), field+" = {:container}", "", 1, 0, dbx.Params{"container": e.Record.Id})
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return e.BadRequestError("non-empty containers must be archived", nil)
			}
			return e.Next()
		})
	}
}

func registerContactOwnershipHooks(pb *pocketbase.PocketBase) {
	pb.OnRecordCreateRequest("contacts").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return e.UnauthorizedError("authentication required", nil)
		}
		e.Record.Set("owner", e.Auth.Id)
		if err := validateAddressBook(e.App, e.Record.GetString("address_book"), e.Auth.Id); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}
		return e.Next()
	})
	pb.OnRecordUpdateRequest("contacts").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil || e.Record.Original().GetString("owner") != e.Auth.Id {
			return e.ForbiddenError("record is not owned by the current user", nil)
		}
		e.Record.Set("owner", e.Auth.Id)
		if err := validateAddressBook(e.App, e.Record.GetString("address_book"), e.Auth.Id); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}
		return e.Next()
	})
}

func validateAddressBook(app core.App, addressBookID, owner string) error {
	if addressBookID == "" {
		return fmt.Errorf("address_book is required")
	}
	book, err := app.FindRecordById("address_books", addressBookID)
	if err != nil || book.GetString("owner") != owner {
		return fmt.Errorf("address book not found")
	}
	return nil
}

func containerField(collection string) string {
	if collection == "events" || collection == "calendars" {
		return "calendar"
	}
	return "list"
}

func nameForEntryCollection(container string) string {
	if container == "calendars" {
		return "events"
	}
	return "todos"
}

func validateEntryContainer(app core.App, entryCollection, containerID, owner string) error {
	if containerID == "" {
		return fmt.Errorf("%s is required", containerField(entryCollection))
	}
	containerCollection := "lists"
	if entryCollection == "events" {
		containerCollection = "calendars"
	}
	container, err := app.FindRecordById(containerCollection, containerID)
	if err != nil || container.GetString("owner") != owner {
		return fmt.Errorf("container not found")
	}
	if container.GetBool("archived") {
		return fmt.Errorf("container is archived")
	}
	return nil
}
