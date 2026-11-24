package app

import (
	"fmt"

	"github.com/andreasmolnardev/slopstack/go/ownership"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// registerOwnershipHooks keeps browser-supplied owner and container values from
// crossing user or collection boundaries. PocketBase rules still protect reads;
// these hooks protect writes made through the SDK and custom routes.
func registerOwnershipHooks(pb *pocketbase.PocketBase) error {
	// Keep the existing native update/delete response for anonymous requests:
	// these app hooks historically returned Forbidden for missing user auth.
	guardAnonymousWrite := func(h *hook.TaggedHook[*core.RecordRequestEvent]) {
		h.BindFunc(func(e *core.RecordRequestEvent) error {
			if e.HasSuperuserAuth() || e.Auth != nil {
				return e.Next()
			}
			return e.ForbiddenError("record is not owned by the current user", nil)
		})
	}
	for _, collection := range []string{"todos", "events", "lists", "calendars", "contacts"} {
		guardAnonymousWrite(pb.OnRecordUpdateRequest(collection))
		guardAnonymousWrite(pb.OnRecordDeleteRequest(collection))
	}
	validateEntry := func(collection string) func(*core.RecordRequestEvent) error {
		return func(e *core.RecordRequestEvent) error {
			if err := validateEntryContainer(e.App, collection, e.Record.GetString(containerField(collection)), e.Auth.Id); err != nil {
				return e.BadRequestError(err.Error(), nil)
			}
			return nil
		}
	}
	policies := []ownership.Policy{
		{Collection: "todos", OwnerField: "owner", ValidateCreate: validateEntry("todos"), ValidateUpdate: validateEntry("todos")},
		{Collection: "events", OwnerField: "owner", ValidateCreate: validateEntry("events"), ValidateUpdate: validateEntry("events"), ValidateDelete: deleteEventOverrides},
		{Collection: "lists", OwnerField: "owner", ValidateDelete: validateEmptyContainer("lists")},
		{Collection: "calendars", OwnerField: "owner", ValidateDelete: validateEmptyContainer("calendars")},
		{Collection: "contacts", OwnerField: "owner", ValidateCreate: validateContactAddressBook, ValidateUpdate: validateContactAddressBook},
	}
	for i := range policies {
		policies[i].AllowSuperusers = true
	}
	return ownership.RegisterRequestHooks(pb, policies...)
}

func deleteEventOverrides(e *core.RecordRequestEvent) error {
	if e.Record.GetString("recurrence_parent") != "" {
		return nil
	}
	overrides, err := e.App.FindRecordsByFilter("events", "owner = {:owner} && recurrence_parent = {:parent}", "id", 0, 0, dbx.Params{"owner": e.Record.GetString("owner"), "parent": e.Record.Id})
	if err != nil {
		return err
	}
	for _, override := range overrides {
		if err := e.App.Delete(override); err != nil {
			return err
		}
		logAuditMutation(e.Request.Context(), e.App, e.Auth.Id, "delete", "events", override.Id, "pocketbase-api-cascade")
	}
	return nil
}

func validateEmptyContainer(collection string) func(*core.RecordRequestEvent) error {
	return func(e *core.RecordRequestEvent) error {
		entryCollection := nameForEntryCollection(collection)
		field := containerField(entryCollection)
		entries, err := e.App.FindRecordsByFilter(entryCollection, field+" = {:container}", "", 1, 0, dbx.Params{"container": e.Record.Id})
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return e.BadRequestError("non-empty containers must be archived", nil)
		}
		return nil
	}
}

func validateContactAddressBook(e *core.RecordRequestEvent) error {
	if err := validateAddressBook(e.App, e.Record.GetString("address_book"), e.Auth.Id); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	return nil
}

func validateAddressBook(app core.App, addressBookID, owner string) error {
	if addressBookID == "" {
		return fmt.Errorf("address_book is required")
	}
	book, err := app.FindRecordById("address_books", addressBookID)
	if err != nil || !ownership.IsOwner(book, "owner", owner) {
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
	if err != nil || !ownership.IsOwner(container, "owner", owner) {
		return fmt.Errorf("container not found")
	}
	if container.GetBool("archived") {
		return fmt.Errorf("container is archived")
	}
	return nil
}
