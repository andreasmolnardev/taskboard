package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const maxCardDAVRequestSize = 1 << 20

// registerCardDAVRoutes exposes the small RFC 6352 surface needed by common
// address-book clients. App passwords are used so DAV clients never need a
// user's PocketBase session token.
func registerCardDAVRoutes(e *core.ServeEvent) {
	handler := func(event *core.RequestEvent) error {
		user, ok := cardDAVUser(event)
		if !ok {
			event.Response.Header().Set("WWW-Authenticate", `Basic realm="Taskboard CardDAV"`)
			return event.UnauthorizedError("CardDAV authentication required", nil)
		}
		return serveCardDAV(event, user)
	}
	e.Router.Any("/.well-known/carddav", handler)
	e.Router.Any("/carddav", handler)
	e.Router.Any("/carddav/{path...}", handler)
}

func cardDAVUser(event *core.RequestEvent) (*core.Record, bool) {
	auth := strings.TrimSpace(event.Request.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Basic ") {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(auth, "Basic ")))
	if err != nil {
		return nil, false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, false
	}
	user, err := AuthenticateAppPassword(event.App, parts[0], parts[1], requestClientIP(event.Request.RemoteAddr))
	return user, err == nil
}

func serveCardDAV(event *core.RequestEvent, user *core.Record) error {
	path := strings.Trim(event.Request.URL.Path, "/")
	if path == ".well-known/carddav" {
		event.Response.Header().Set("Content-Type", "application/xml; charset=utf-8")
		event.Response.Header().Set("DAV", "1, addressbook")
		return event.Blob(http.StatusMovedPermanently, "application/xml", []byte(`<d:redirect xmlns:d="DAV:" d:href="/carddav/"/>`))
	}
	if event.Request.Method == http.MethodPut {
		return cardDAVPut(event, user)
	}
	if event.Request.Method == http.MethodDelete {
		return cardDAVDelete(event, user)
	}
	if event.Request.Method == "PROPFIND" || event.Request.Method == "REPORT" || event.Request.Method == http.MethodGet {
		return cardDAVRead(event, user)
	}
	return event.NoContent(http.StatusNoContent)
}

func cardDAVRead(event *core.RequestEvent, user *core.Record) error {
	pathParts := strings.Split(strings.Trim(event.Request.URL.Path, "/"), "/")
	if event.Request.Method == http.MethodGet && len(pathParts) == 3 {
		href, err := cardDAVHref(pathParts[2])
		if err != nil {
			return event.NotFoundError("contact not found", nil)
		}
		book, err := ownedAddressBook(event.App, user.Id, pathParts[1])
		if err != nil {
			return event.NotFoundError("address book not found", nil)
		}
		contact, err := findCardDAVContact(event.App, user.Id, book.Id, href)
		if err != nil {
			return event.NotFoundError("contact not found", nil)
		}
		vcard := contactVCard(contact)
		event.Response.Header().Set("Content-Type", "text/vcard; charset=utf-8")
		event.Response.Header().Set("ETag", contactETag(vcard))
		return event.Blob(http.StatusOK, "text/vcard", []byte(vcard))
	}
	event.Response.Header().Set("DAV", "1, addressbook")
	event.Response.Header().Set("Content-Type", "application/xml; charset=utf-8")
	books, err := event.App.FindRecordsByFilter("address_books", "owner = {:owner}", "name", 0, 0, dbx.Params{"owner": user.Id})
	if err != nil {
		return err
	}
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?><d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">`)
	for _, book := range books {
		href := "/carddav/" + book.Id + "/"
		ctag, err := cardDAVCTag(event.App, user.Id, book.Id)
		if err != nil {
			return err
		}
		body.WriteString(fmt.Sprintf(`<d:response><d:href>%s</d:href><d:propstat><d:prop><d:displayname>%s</d:displayname><d:resourcetype><d:collection/><card:addressbook/></d:resourcetype><d:getctag>%s</d:getctag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, href, xmlText(book.GetString("name")), xmlText(ctag)))
		contacts, err := event.App.FindRecordsByFilter("contacts", "owner = {:owner} && address_book = {:book}", "formatted_name", 0, 0, dbx.Params{"owner": user.Id, "book": book.Id})
		if err != nil {
			return err
		}
		for _, contact := range contacts {
			name := contact.GetString("dav_href")
			if name == "" {
				name = contact.Id + ".vcf"
			}
			vcard := contactVCard(contact)
			body.WriteString(fmt.Sprintf(`<d:response><d:href>%s%s</d:href><d:propstat><d:prop><d:getetag>%s</d:getetag><d:getcontenttype>text/vcard</d:getcontenttype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, href, url.PathEscape(name), contactETag(vcard)))
		}
	}
	body.WriteString(`</d:multistatus>`)
	status := http.StatusMultiStatus
	if event.Request.Method == http.MethodGet {
		status = http.StatusOK
	}
	return event.Blob(status, "application/xml", []byte(body.String()))
}

func cardDAVPut(event *core.RequestEvent, user *core.Record) error {
	parts := strings.Split(strings.Trim(event.Request.URL.Path, "/"), "/")
	if len(parts) != 3 {
		return event.BadRequestError("contact path is required", nil)
	}
	book, err := ownedAddressBook(event.App, user.Id, parts[1])
	if err != nil {
		return event.NotFoundError("address book not found", nil)
	}
	href, err := cardDAVHref(parts[2])
	if err != nil {
		return event.BadRequestError("contact href must end in .vcf", err)
	}
	event.Request.Body = http.MaxBytesReader(event.Response, event.Request.Body, maxCardDAVRequestSize)
	data, err := io.ReadAll(event.Request.Body)
	if err != nil {
		return event.BadRequestError("vCard is too large", err)
	}
	vcard, err := normalizeVCard(string(data))
	if err != nil {
		return event.BadRequestError("invalid vCard", err)
	}

	contact, err := findCardDAVContact(event.App, user.Id, book.Id, href)
	created := err != nil
	if created {
		collection, collectionErr := event.App.FindCollectionByNameOrId("contacts")
		if collectionErr != nil {
			return collectionErr
		}
		contact = core.NewRecord(collection)
		contact.Set("uid", strings.TrimSuffix(href, ".vcf"))
		contact.Set("owner", user.Id)
		contact.Set("address_book", book.Id)
		contact.Set("dav_href", href)
	}
	if contact.GetString("owner") != user.Id || contact.GetString("address_book") != book.Id {
		return event.NotFoundError("contact not found", nil)
	}
	contact.Set("vcard", vcard)
	contact.Set("dav_href", href)
	parseVCard(contact, vcard)
	if err := event.App.Save(contact); err != nil {
		return err
	}
	event.Response.Header().Set("ETag", contactETag(vcard))
	event.Response.Header().Set("Location", "/carddav/"+book.Id+"/"+url.PathEscape(href))
	if created {
		return event.NoContent(http.StatusCreated)
	}
	return event.NoContent(http.StatusNoContent)
}

func cardDAVDelete(event *core.RequestEvent, user *core.Record) error {
	parts := strings.Split(strings.Trim(event.Request.URL.Path, "/"), "/")
	if len(parts) != 3 {
		return event.NotFoundError("contact not found", nil)
	}
	book, err := ownedAddressBook(event.App, user.Id, parts[1])
	if err != nil {
		return event.NotFoundError("address book not found", nil)
	}
	href, err := cardDAVHref(parts[2])
	if err != nil {
		return event.NotFoundError("contact not found", nil)
	}
	contact, err := findCardDAVContact(event.App, user.Id, book.Id, href)
	if err != nil {
		return event.NotFoundError("contact not found", nil)
	}
	if err := event.App.Delete(contact); err != nil {
		return err
	}
	return event.NoContent(http.StatusNoContent)
}

func ownedAddressBook(app core.App, owner, id string) (*core.Record, error) {
	book, err := app.FindRecordById("address_books", id)
	if err != nil || book.GetString("owner") != owner {
		return nil, fmt.Errorf("address book not found")
	}
	return book, nil
}

func cardDAVHref(raw string) (string, error) {
	href, err := url.PathUnescape(raw)
	if err != nil || href == "" || strings.ContainsAny(href, "/\\") || !strings.HasSuffix(href, ".vcf") {
		return "", fmt.Errorf("invalid contact href")
	}
	return href, nil
}

func findCardDAVContact(app core.App, owner, bookID, href string) (*core.Record, error) {
	matches, err := app.FindRecordsByFilter("contacts", "owner = {:owner} && address_book = {:book} && dav_href = {:href}", "", 1, 0, dbx.Params{"owner": owner, "book": bookID, "href": href})
	if err != nil {
		return nil, err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	// Contacts created before client hrefs were stored used the PocketBase ID
	// as their resource name. Keep those resources readable and writable.
	legacyID := strings.TrimSuffix(href, ".vcf")
	contact, err := app.FindRecordById("contacts", legacyID)
	if err != nil || contact.GetString("owner") != owner || contact.GetString("address_book") != bookID || contact.GetString("dav_href") != "" {
		return nil, fmt.Errorf("contact not found")
	}
	return contact, nil
}

func normalizeVCard(value string) (string, error) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimRight(value, "\n")
	if value == "" || !strings.Contains(value, "BEGIN:VCARD\n") || !strings.HasSuffix(value, "\nEND:VCARD") {
		return "", fmt.Errorf("BEGIN:VCARD and END:VCARD are required")
	}
	return strings.ReplaceAll(value, "\n", "\r\n") + "\r\n", nil
}

func contactVCard(contact *core.Record) string {
	if value := contact.GetString("vcard"); value != "" {
		if normalized, err := normalizeVCard(value); err == nil {
			return normalized
		}
	}
	return recordVCard(contact)
}

func contactETag(vcard string) string {
	digest := sha256.Sum256([]byte(vcard))
	return `"` + hex.EncodeToString(digest[:]) + `"`
}

func cardDAVCTag(app core.App, owner, bookID string) (string, error) {
	contacts, err := app.FindRecordsByFilter("contacts", "owner = {:owner} && address_book = {:book}", "", 0, 0, dbx.Params{"owner": owner, "book": bookID})
	if err != nil {
		return "", err
	}
	values := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		name := contact.GetString("dav_href")
		if name == "" {
			name = contact.Id + ".vcf"
		}
		values = append(values, name+"\x00"+contactETag(contactVCard(contact)))
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(digest[:]), nil
}

func recordVCard(contact *core.Record) string {
	lines := []string{"BEGIN:VCARD", "VERSION:3.0", "UID:" + contact.GetString("uid"), "FN:" + contact.GetString("formatted_name"), "N:" + contact.GetString("family_name") + ";" + contact.GetString("given_name") + ";;;"}
	for _, field := range []struct{ key, value string }{
		{"EMAIL", contact.GetString("email")},
		{"TEL", contact.GetString("phone")},
		{"ORG", contact.GetString("organization")},
		{"TITLE", contact.GetString("title")},
	} {
		if field.value != "" {
			lines = append(lines, field.key+":"+field.value)
		}
	}
	return strings.Join(append(lines, "END:VCARD", ""), "\r\n")
}

func parseVCard(contact *core.Record, vcard string) {
	for _, line := range strings.Split(vcard, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.Split(key, ";")[0])
		value = strings.TrimSpace(value)
		switch key {
		case "FN":
			contact.Set("formatted_name", value)
		case "N":
			names := strings.Split(value, ";")
			if len(names) > 0 {
				contact.Set("family_name", names[0])
			}
			if len(names) > 1 {
				contact.Set("given_name", names[1])
			}
		case "EMAIL":
			contact.Set("email", value)
		case "TEL":
			contact.Set("phone", value)
		case "ORG":
			contact.Set("organization", value)
		case "TITLE":
			contact.Set("title", value)
		case "NOTE":
			contact.Set("notes", value)
		case "UID":
			contact.Set("uid", value)
		}
	}
}

func xmlText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}
