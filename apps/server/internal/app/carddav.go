package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/andreasmolnardev/slopstack/go/ownership"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	maxCardDAVRequestSize = 1 << 20
	maxCardDAVResults     = 500
	cardDAVNamespace      = "urn:ietf:params:xml:ns:carddav"
	calendarServerNS      = "http://calendarserver.org/ns/"
)

func registerCardDAVRoutes(e *core.ServeEvent) {
	handler := func(event *core.RequestEvent) error {
		if event.Request.Method == http.MethodOptions {
			setCardDAVHeaders(event.Response.Header())
			event.Response.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
			return event.NoContent(http.StatusNoContent)
		}
		if event.Request.URL.Path == "/.well-known/carddav" {
			if _, ok := cardDAVUser(event); !ok {
				event.Response.Header().Set("WWW-Authenticate", `Basic realm="Taskboard CardDAV", charset="UTF-8"`)
				return event.UnauthorizedError("CardDAV authentication required", nil)
			}
			setCardDAVHeaders(event.Response.Header())
			event.Response.Header().Set("Location", "/carddav/")
			return event.Blob(http.StatusMovedPermanently, "application/xml", []byte(`<d:redirect xmlns:d="DAV:" d:href="/carddav/"/>`))
		}
		user, ok := cardDAVUser(event)
		if !ok {
			event.Response.Header().Set("WWW-Authenticate", `Basic realm="Taskboard CardDAV", charset="UTF-8"`)
			return event.UnauthorizedError("CardDAV authentication required", nil)
		}
		setCardDAVHeaders(event.Response.Header())
		return serveCardDAV(event, user)
	}
	e.Router.Any("/.well-known/carddav", handler)
	e.Router.Any("/carddav", handler)
	e.Router.Any("/carddav/{path...}", handler)
}

func setCardDAVHeaders(header http.Header) {
	header.Set("DAV", "1, addressbook")
	header.Set("MS-Author-Via", "DAV")
}

func cardDAVUser(event *core.RequestEvent) (*core.Record, bool) {
	auth := strings.TrimSpace(event.Request.Header.Get("Authorization"))
	parts := strings.Fields(auth)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	username, password, found := strings.Cut(string(decoded), ":")
	if !found {
		return nil, false
	}
	user, err := AuthenticateAppPassword(event.App, username, password, requestClientIP(event.Request.RemoteAddr))
	return user, err == nil
}

type cardDAVPath struct {
	kind         string
	userID       string
	bookID       string
	resourceName string
}

func parseCardDAVPath(path string) cardDAVPath {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "carddav" {
		return cardDAVPath{kind: "root"}
	}
	if len(parts) == 3 && parts[0] == "carddav" && parts[1] == "principals" {
		return cardDAVPath{kind: "principal", userID: parts[2]}
	}
	if len(parts) == 3 && parts[0] == "carddav" && parts[1] == "addressbooks" {
		bookID, err := url.PathUnescape(parts[2])
		if err != nil {
			return cardDAVPath{}
		}
		return cardDAVPath{kind: "book", bookID: bookID}
	}
	if len(parts) >= 2 && parts[0] == "carddav" {
		bookID, err := url.PathUnescape(parts[1])
		if err != nil {
			return cardDAVPath{}
		}
		if len(parts) == 2 {
			return cardDAVPath{kind: "book", bookID: bookID}
		}
		if len(parts) == 3 {
			name, unescapeErr := url.PathUnescape(parts[2])
			if unescapeErr != nil || name == "" || strings.ContainsAny(name, "/\\") || !strings.HasSuffix(strings.ToLower(name), ".vcf") {
				return cardDAVPath{}
			}
			return cardDAVPath{kind: "resource", bookID: bookID, resourceName: name}
		}
	}
	if len(parts) == 4 && parts[0] == "carddav" && parts[1] == "addressbooks" {
		bookID, err := url.PathUnescape(parts[2])
		name, nameErr := url.PathUnescape(parts[3])
		if err != nil || nameErr != nil || name == "" || strings.ContainsAny(name, "/\\") || !strings.HasSuffix(strings.ToLower(name), ".vcf") {
			return cardDAVPath{}
		}
		return cardDAVPath{kind: "resource", bookID: bookID, resourceName: name}
	}
	return cardDAVPath{}
}

func cardDAVBookHref(bookID string) string { return "/carddav/" + url.PathEscape(bookID) + "/" }

func cardDAVResourceHref(bookID, name string) string {
	return cardDAVBookHref(bookID) + url.PathEscape(name)
}

func cardDAVHref(raw string) (string, error) {
	href, err := url.PathUnescape(raw)
	if err != nil || href == "" || strings.ContainsAny(href, "/\\\\") || !strings.HasSuffix(strings.ToLower(href), ".vcf") {
		return "", fmt.Errorf("invalid contact href")
	}
	return href, nil
}

func serveCardDAV(event *core.RequestEvent, user *core.Record) error {
	path := parseCardDAVPath(event.Request.URL.Path)
	if path.kind == "" || (path.userID != "" && path.userID != user.Id) {
		return event.NotFoundError("DAV resource not found", nil)
	}
	switch event.Request.Method {
	case "PROPFIND":
		return cardDAVPropfind(event, user, path)
	case "REPORT":
		return cardDAVReport(event, user, path)
	case http.MethodGet, http.MethodHead:
		return cardDAVGet(event, user, path)
	case http.MethodPut:
		return cardDAVPut(event, user, path)
	case http.MethodDelete:
		return cardDAVDelete(event, user, path)
	default:
		event.Response.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
		return event.NoContent(http.StatusMethodNotAllowed)
	}
}

type cardDAVXMLRequest struct {
	XMLName   xml.Name
	Prop      cardDAVPropertyRequest `xml:"prop"`
	Hrefs     []string               `xml:"href"`
	Filter    *cardDAVFilter         `xml:"filter"`
	SyncToken string                 `xml:"sync-token"`
	SyncLevel string                 `xml:"sync-level"`
}

type cardDAVPropertyRequest struct {
	Names []cardDAVRequestedProperty `xml:",any"`
}

type cardDAVRequestedProperty struct{ XMLName xml.Name }

type cardDAVFilter struct {
	Test       string              `xml:"test,attr"`
	Properties []cardDAVPropFilter `xml:"prop-filter"`
}

type cardDAVPropFilter struct {
	Name         string               `xml:"name,attr"`
	IsNotDefined *struct{}            `xml:"is-not-defined"`
	TextMatch    *cardDAVTextMatch    `xml:"text-match"`
	ParamFilters []cardDAVParamFilter `xml:"param-filter"`
}

type cardDAVParamFilter struct {
	Name         string            `xml:"name,attr"`
	IsNotDefined *struct{}         `xml:"is-not-defined"`
	TextMatch    *cardDAVTextMatch `xml:"text-match"`
}

type cardDAVTextMatch struct {
	Collation string `xml:"collation,attr"`
	MatchType string `xml:"match-type,attr"`
	Value     string `xml:",chardata"`
}

type cardDAVProperty struct {
	Name  xml.Name
	Inner string
}

type cardDAVResponse struct {
	Href       string
	OK         []cardDAVProperty
	NotFound   []xml.Name
	StatusCode int
}

func parseCardDAVXML(body string) (cardDAVXMLRequest, error) {
	var request cardDAVXMLRequest
	if strings.TrimSpace(body) == "" {
		return request, nil
	}
	if err := xml.Unmarshal([]byte(body), &request); err != nil {
		return request, fmt.Errorf("invalid CardDAV XML: %w", err)
	}
	return request, nil
}

func readCardDAVXML(event *core.RequestEvent) (cardDAVXMLRequest, error) {
	event.Request.Body = http.MaxBytesReader(event.Response, event.Request.Body, maxCardDAVRequestSize)
	data, err := io.ReadAll(event.Request.Body)
	if err != nil {
		return cardDAVXMLRequest{}, err
	}
	return parseCardDAVXML(string(data))
}

func cardDAVPropfind(event *core.RequestEvent, user *core.Record, path cardDAVPath) error {
	depth := event.Request.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}
	if depth != "0" && depth != "1" && depth != "infinity" {
		return event.Blob(http.StatusForbidden, "application/xml; charset=utf-8", []byte(`<?xml version="1.0"?><d:error xmlns:d="DAV:"><d:propfind-finite-depth/></d:error>`))
	}
	if path.kind == "home" {
		path.kind = "book"
	}
	if path.kind == "book" || path.kind == "resource" {
		if _, err := ownedAddressBook(event.App, user.Id, path.bookID); err != nil {
			return event.NotFoundError("address book not found", nil)
		}
	}
	request, err := readCardDAVXML(event)
	if err != nil {
		return event.BadRequestError("invalid PROPFIND body", err)
	}
	responses, err := cardDAVProperties(event.App, user, path, depth != "0", request)
	if err != nil {
		return err
	}
	return event.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", cardDAVMultistatus(responses))
}

func cardDAVProperties(app core.App, user *core.Record, path cardDAVPath, children bool, request cardDAVXMLRequest) ([]cardDAVResponse, error) {
	principalHref := "/carddav/principals/" + url.PathEscape(user.Id) + "/"
	homeHref := "/carddav/"
	base := map[xml.Name]string{
		davName("current-user-principal"): `<d:href>` + principalHref + `</d:href>`,
		davName("principal-URL"):          `<d:href>` + principalHref + `</d:href>`,
		cardName("addressbook-home-set"):  `<d:href>` + homeHref + `</d:href>`,
	}
	makeResponse := func(href string, values map[xml.Name]string) cardDAVResponse {
		ok, missing := requestedCardDAVProperties(request, values)
		return cardDAVResponse{Href: href, OK: ok, NotFound: missing}
	}
	switch path.kind {
	case "root", "home":
		values := cloneCardDAVProperties(base)
		values[davName("displayname")] = xmlText(user.GetString("email"))
		values[davName("resourcetype")] = `<d:collection/>`
		responses := []cardDAVResponse{makeResponse(homeHref, values)}
		if !children {
			return responses, nil
		}
		books, err := app.FindRecordsByFilter("address_books", "owner = {:owner}", "name", 0, 0, dbx.Params{"owner": user.Id})
		if err != nil {
			return nil, err
		}
		for _, book := range books {
			responses = append(responses, makeResponse(cardDAVBookHref(book.Id), cardDAVBookProperties(app, user.Id, book)))
			// Keep the legacy root listing useful for clients that omit Depth.
			contacts, contactsErr := cardDAVContacts(app, user.Id, book.Id)
			if contactsErr != nil {
				return nil, contactsErr
			}
			for _, contact := range contacts {
				responses = append(responses, cardDAVContactResponse(contact, book.Id, request))
			}
		}
		return responses, nil
	case "principal":
		values := cloneCardDAVProperties(base)
		values[davName("displayname")] = xmlText(user.GetString("display_name"))
		values[davName("resourcetype")] = `<d:principal/>`
		return []cardDAVResponse{makeResponse(principalHref, values)}, nil
	case "book":
		book, err := ownedAddressBook(app, user.Id, path.bookID)
		if err != nil {
			return nil, err
		}
		responses := []cardDAVResponse{makeResponse(cardDAVBookHref(book.Id), cardDAVBookProperties(app, user.Id, book))}
		if !children {
			return responses, nil
		}
		contacts, err := cardDAVContacts(app, user.Id, book.Id)
		if err != nil {
			return nil, err
		}
		for _, contact := range contacts {
			vcard := contactVCard(contact)
			values := map[xml.Name]string{
				davName("getetag"):          contactETag(vcard),
				davName("getcontenttype"):   "text/vcard; charset=utf-8",
				davName("getcontentlength"): fmt.Sprintf("%d", len([]byte(vcard))),
				cardName("address-data"):    xmlText(vcard),
			}
			responses = append(responses, makeResponse(cardDAVResourceHref(book.Id, contactHref(contact)), values))
		}
		return responses, nil
	case "resource":
		contact, err := findCardDAVContact(app, user.Id, path.bookID, path.resourceName)
		if err != nil {
			return nil, err
		}
		vcard := contactVCard(contact)
		values := map[xml.Name]string{
			davName("getetag"):          contactETag(vcard),
			davName("getcontenttype"):   "text/vcard; charset=utf-8",
			davName("getcontentlength"): fmt.Sprintf("%d", len([]byte(vcard))),
			cardName("address-data"):    xmlText(vcard),
		}
		return []cardDAVResponse{makeResponse(cardDAVResourceHref(path.bookID, path.resourceName), values)}, nil
	default:
		return nil, fmt.Errorf("DAV resource not found")
	}
}

func cardDAVBookProperties(app core.App, owner string, book *core.Record) map[xml.Name]string {
	ctag, _ := cardDAVCTag(app, owner, book.Id)
	token, _ := currentCardDAVToken(app, owner, book.Id)
	return map[xml.Name]string{
		davName("displayname"):                xmlText(book.GetString("name")),
		davName("resourcetype"):               `<d:collection/><card:addressbook/>`,
		davName("owner"):                      `<d:href>/carddav/principals/` + url.PathEscape(owner) + `/</d:href>`,
		davName("current-user-privilege-set"): `<d:privilege><d:all/></d:privilege>`,
		davName("getctag"):                    xmlText(ctag),
		calendarServerName("getctag"):         xmlText(ctag),
		davName("sync-token"):                 xmlText(token),
		cardName("supported-address-data"):    `<card:address-data content-type="text/vcard" version="3.0"/><card:address-data content-type="text/vcard" version="4.0"/>`,
		cardName("addressbook-description"):   xmlText(book.GetString("description")),
	}
}

func cardDAVGet(event *core.RequestEvent, user *core.Record, path cardDAVPath) error {
	if path.kind != "resource" {
		if path.kind == "root" || path.kind == "home" || path.kind == "book" {
			request := cardDAVXMLRequest{}
			responses, err := cardDAVProperties(event.App, user, path, true, request)
			if err != nil {
				return event.NotFoundError("address book not found", nil)
			}
			status := http.StatusOK
			if event.Request.Method == http.MethodHead {
				return event.NoContent(status)
			}
			return event.Blob(status, "application/xml; charset=utf-8", cardDAVMultistatus(responses))
		}
		return event.NotFoundError("contact not found", nil)
	}
	contact, err := findCardDAVContact(event.App, user.Id, path.bookID, path.resourceName)
	if err != nil {
		return event.NotFoundError("contact not found", nil)
	}
	vcard := contactVCard(contact)
	etag := contactETag(vcard)
	event.Response.Header().Set("ETag", etag)
	event.Response.Header().Set("Content-Length", fmt.Sprintf("%d", len([]byte(vcard))))
	event.Response.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	if etagListContains(event.Request.Header.Get("If-None-Match"), etag) {
		return event.NoContent(http.StatusNotModified)
	}
	if event.Request.Method == http.MethodHead {
		return event.NoContent(http.StatusOK)
	}
	return event.Blob(http.StatusOK, "text/vcard; charset=utf-8", []byte(vcard))
}

func cardDAVPut(event *core.RequestEvent, user *core.Record, path cardDAVPath) error {
	if path.kind != "resource" {
		return event.NotFoundError("contact path is required", nil)
	}
	book, err := ownedAddressBook(event.App, user.Id, path.bookID)
	if err != nil {
		return event.NotFoundError("address book not found", nil)
	}
	mediaType, _, contentTypeErr := mime.ParseMediaType(event.Request.Header.Get("Content-Type"))
	if contentTypeErr != nil || (mediaType != "" && !strings.EqualFold(mediaType, "text/vcard")) {
		return event.BadRequestError("Content-Type must be text/vcard", nil)
	}
	event.Request.Body = http.MaxBytesReader(event.Response, event.Request.Body, maxCardDAVRequestSize)
	data, err := io.ReadAll(event.Request.Body)
	if err != nil {
		return event.BadRequestError("vCard is too large", err)
	}
	card, err := parseVCardDocument(data)
	if err != nil {
		return event.BadRequestError("invalid vCard", err)
	}
	uid := firstVCardValue(card, "UID")
	if uid == "" {
		return cardDAVConflict(event, "UID is required")
	}
	normalizedData, err := encodeVCardDocument(card)
	if err != nil {
		return event.BadRequestError("invalid vCard", err)
	}
	existing, existingErr := findCardDAVContact(event.App, user.Id, book.Id, path.resourceName)
	created := existingErr != nil
	currentETag := ""
	if !created {
		currentETag = contactETag(contactVCard(existing))
	}
	if status := checkCalDAVPreconditions(event.Request.Header, !created, currentETag); status != 0 {
		return event.NoContent(status)
	}
	if !created && existing.GetString("uid") != "" && existing.GetString("uid") != uid {
		return cardDAVConflict(event, "UID cannot change")
	}
	matches, err := event.App.FindRecordsByFilter("contacts", "owner = {:owner} && address_book = {:book} && uid = {:uid}", "", 2, 0, dbx.Params{"owner": user.Id, "book": book.Id, "uid": uid})
	if err != nil {
		return err
	}
	for _, match := range matches {
		if created || match.Id != existing.Id {
			return cardDAVConflict(event, "UID already exists in this address book")
		}
	}
	var saved *core.Record
	err = event.App.RunInTransaction(func(txApp core.App) error {
		var record *core.Record
		if created {
			collection, collectionErr := txApp.FindCollectionByNameOrId("contacts")
			if collectionErr != nil {
				return collectionErr
			}
			record = core.NewRecord(collection)
			record.Set("owner", user.Id)
			record.Set("address_book", book.Id)
		} else {
			record, err = findCardDAVContact(txApp, user.Id, book.Id, path.resourceName)
			if err != nil {
				return err
			}
		}
		record.Set("uid", uid)
		record.Set("dav_href", path.resourceName)
		record.Set("vcard", string(normalizedData))
		projectVCardToContact(record, card)
		if err := txApp.Save(record); err != nil {
			return err
		}
		saved = record
		return nil
	})
	if err != nil {
		return err
	}
	action := "update"
	if created {
		action = "create"
	}
	logAuditMutation(event.Request.Context(), event.App, user.Id, action, "contacts", saved.Id, "carddav")
	etag := contactETag(string(normalizedData))
	event.Response.Header().Set("ETag", etag)
	event.Response.Header().Set("Location", cardDAVResourceHref(book.Id, path.resourceName))
	if created {
		return event.NoContent(http.StatusCreated)
	}
	return event.NoContent(http.StatusNoContent)
}

func cardDAVDelete(event *core.RequestEvent, user *core.Record, path cardDAVPath) error {
	if path.kind != "resource" {
		return event.NotFoundError("contact not found", nil)
	}
	contact, err := findCardDAVContact(event.App, user.Id, path.bookID, path.resourceName)
	if err != nil {
		return event.NotFoundError("contact not found", nil)
	}
	if status := checkCalDAVPreconditions(event.Request.Header, true, contactETag(contactVCard(contact))); status != 0 {
		return event.NoContent(status)
	}
	if err := event.App.Delete(contact); err != nil {
		return err
	}
	logAuditMutation(event.Request.Context(), event.App, user.Id, "delete", "contacts", contact.Id, "carddav")
	return event.NoContent(http.StatusNoContent)
}

func cardDAVReport(event *core.RequestEvent, user *core.Record, path cardDAVPath) error {
	if path.kind == "home" {
		path.kind = "book"
	}
	if path.kind != "book" || ownedBookErr(event.App, user.Id, path.bookID) != nil {
		return event.NotFoundError("address book not found", nil)
	}
	request, err := readCardDAVXML(event)
	if err != nil {
		return event.BadRequestError("invalid REPORT body", err)
	}
	if request.XMLName.Local == "" {
		responses, propErr := cardDAVProperties(event.App, user, path, true, request)
		if propErr != nil {
			return propErr
		}
		return event.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", cardDAVMultistatus(responses))
	}
	if request.XMLName.Space == davNamespace && request.XMLName.Local == "sync-collection" {
		return cardDAVSyncCollection(event, user, path, request)
	}
	if request.XMLName.Space != cardDAVNamespace {
		return event.BadRequestError("unsupported CardDAV REPORT", nil)
	}
	contacts, err := cardDAVContacts(event.App, user.Id, path.bookID)
	if err != nil {
		return err
	}
	responses := make([]cardDAVResponse, 0)
	switch request.XMLName.Local {
	case "addressbook-multiget":
		if len(request.Hrefs) > maxCardDAVResults {
			return cardDAVLimitError(event)
		}
		for _, href := range request.Hrefs {
			resourcePath := parseCardDAVPath(cardDAVHrefPath(href))
			if resourcePath.kind != "resource" || resourcePath.bookID != path.bookID {
				responses = append(responses, cardDAVResponse{Href: href, StatusCode: http.StatusNotFound})
				continue
			}
			contact, findErr := findCardDAVContact(event.App, user.Id, path.bookID, resourcePath.resourceName)
			if findErr != nil {
				responses = append(responses, cardDAVResponse{Href: href, StatusCode: http.StatusNotFound})
				continue
			}
			responses = append(responses, cardDAVContactResponse(contact, path.bookID, request))
		}
	case "addressbook-query":
		for _, contact := range contacts {
			card := contactCard(contact)
			if request.Filter != nil && !cardDAVCardMatches(card, *request.Filter) {
				continue
			}
			responses = append(responses, cardDAVContactResponse(contact, path.bookID, request))
			if len(responses) > maxCardDAVResults {
				return cardDAVLimitError(event)
			}
		}
	default:
		return event.BadRequestError("unsupported CardDAV REPORT", nil)
	}
	return event.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", cardDAVMultistatus(responses))
}

func cardDAVSyncCollection(event *core.RequestEvent, user *core.Record, path cardDAVPath, request cardDAVXMLRequest) error {
	if request.SyncLevel != "" && request.SyncLevel != "1" {
		return event.BadRequestError("only sync-level 1 is supported", nil)
	}
	token, err := parseCardDAVToken(request.SyncToken)
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	current, err := currentCardDAVToken(event.App, user.Id, path.bookID)
	if err != nil {
		return err
	}
	changes, err := cardDAVChangesSince(event.App, user.Id, path.bookID, token, maxCardDAVResults+1)
	if err != nil {
		return err
	}
	if len(changes) > maxCardDAVResults {
		return cardDAVLimitError(event)
	}
	latest := make(map[string]*core.Record)
	for _, change := range changes {
		latest[change.GetString("href")] = change
	}
	responses := make([]cardDAVResponse, 0, len(latest))
	for href, change := range latest {
		if change.GetBool("deleted") {
			responses = append(responses, cardDAVResponse{Href: cardDAVResourceHref(path.bookID, href), StatusCode: http.StatusNotFound})
			continue
		}
		contact, findErr := findCardDAVContact(event.App, user.Id, path.bookID, href)
		if findErr != nil {
			continue
		}
		responses = append(responses, cardDAVContactResponse(contact, path.bookID, request))
	}
	return event.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", cardDAVMultistatusWithToken(responses, current))
}

func cardDAVContactResponse(contact *core.Record, bookID string, request cardDAVXMLRequest) cardDAVResponse {
	vcard := contactVCard(contact)
	values := map[xml.Name]string{
		davName("getetag"):          contactETag(vcard),
		davName("getcontenttype"):   "text/vcard; charset=utf-8",
		davName("getcontentlength"): fmt.Sprintf("%d", len([]byte(vcard))),
		cardName("address-data"):    xmlText(vcard),
	}
	ok, missing := requestedCardDAVProperties(request, values)
	return cardDAVResponse{Href: cardDAVResourceHref(bookID, contactHref(contact)), OK: ok, NotFound: missing}
}

func cardDAVCardMatches(card vCardDocument, filter cardDAVFilter) bool {
	if len(filter.Properties) == 0 {
		return true
	}
	matches := make([]bool, 0, len(filter.Properties))
	for _, propertyFilter := range filter.Properties {
		properties := propertyValues(card, propertyFilter.Name)
		if propertyFilter.IsNotDefined != nil {
			matches = append(matches, len(properties) == 0)
			continue
		}
		matched := false
		for _, property := range properties {
			if vCardPropertyMatches(property, propertyFilter) {
				matched = true
				break
			}
		}
		matches = append(matches, matched)
	}
	if strings.EqualFold(filter.Test, "any") {
		for _, matched := range matches {
			if matched {
				return true
			}
		}
		return false
	}
	for _, matched := range matches {
		if !matched {
			return false
		}
	}
	return true
}

func contactCard(contact *core.Record) vCardDocument {
	card, err := parseVCardDocument([]byte(contactVCard(contact)))
	if err != nil {
		return vCardDocument{}
	}
	return card
}

func cardDAVHrefPath(href string) string {
	parsed, err := url.Parse(href)
	if err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return href
}

func cardDAVLimitError(event *core.RequestEvent) error {
	return event.Blob(http.StatusInsufficientStorage, "application/xml; charset=utf-8", []byte(`<?xml version="1.0"?><d:error xmlns:d="DAV:"><d:number-of-matches-within-limits/></d:error>`))
}

func cardDAVConflict(event *core.RequestEvent, message string) error {
	return event.Blob(http.StatusConflict, "application/xml; charset=utf-8", []byte(`<?xml version="1.0"?><d:error xmlns:d="DAV:"><d:no-uid-conflict/><d:responsedescription>`+xmlText(message)+`</d:responsedescription></d:error>`))
}

func ownedBookErr(app core.App, owner, id string) error {
	_, err := ownedAddressBook(app, owner, id)
	return err
}

func ownedAddressBook(app core.App, owner, id string) (*core.Record, error) {
	book, err := app.FindRecordById("address_books", id)
	if err != nil || !ownership.IsOwner(book, "owner", owner) {
		return nil, fmt.Errorf("address book not found")
	}
	return book, nil
}

func cardDAVContacts(app core.App, owner, bookID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("contacts", "owner = {:owner} && address_book = {:book}", "formatted_name", 0, 0, dbx.Params{"owner": owner, "book": bookID})
}

func contactHref(contact *core.Record) string {
	if href := contact.GetString("dav_href"); href != "" {
		return href
	}
	return contact.Id + ".vcf"
}

func findCardDAVContact(app core.App, owner, bookID, href string) (*core.Record, error) {
	matches, err := app.FindRecordsByFilter("contacts", "owner = {:owner} && address_book = {:book} && dav_href = {:href}", "", 1, 0, dbx.Params{"owner": owner, "book": bookID, "href": href})
	if err != nil {
		return nil, err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	legacyID := strings.TrimSuffix(href, ".vcf")
	contact, err := app.FindRecordById("contacts", legacyID)
	if err != nil || !ownership.IsOwner(contact, "owner", owner) || contact.GetString("address_book") != bookID || contact.GetString("dav_href") != "" {
		return nil, fmt.Errorf("contact not found")
	}
	return contact, nil
}

func normalizeVCard(value string) (string, error) {
	card, err := parseVCardDocument([]byte(value))
	if err != nil {
		return "", err
	}
	encoded, err := encodeVCardDocument(card)
	return string(encoded), err
}

func parseVCard(contact *core.Record, value string) {
	card, err := parseVCardDocument([]byte(value))
	if err == nil {
		projectVCardToContact(contact, card)
	}
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
	contacts, err := cardDAVContacts(app, owner, bookID)
	if err != nil {
		return "", err
	}
	values := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		values = append(values, contactHref(contact)+"\x00"+contactETag(contactVCard(contact)))
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(digest[:]), nil
}

func recordVCard(contact *core.Record) string {
	card := vCardDocument{Properties: []vCardProperty{
		{Name: "VERSION", Value: "3.0"},
		{Name: "UID", Value: escapeVCardText(contact.GetString("uid"))},
		{Name: "FN", Value: escapeVCardText(contact.GetString("formatted_name"))},
		{Name: "N", Value: escapeVCardText(contact.GetString("family_name")) + ";" + escapeVCardText(contact.GetString("given_name")) + ";;;"},
	}}
	for _, field := range []struct{ key, value string }{
		{"EMAIL", contact.GetString("email")},
		{"TEL", contact.GetString("phone")},
		{"ORG", contact.GetString("organization")},
		{"TITLE", contact.GetString("title")},
		{"ADR", contact.GetString("address")},
		{"NOTE", contact.GetString("notes")},
	} {
		if field.value != "" {
			card.Properties = append(card.Properties, vCardProperty{Name: field.key, Value: escapeVCardText(field.value)})
		}
	}
	encoded, _ := encodeVCardDocument(card)
	return string(encoded)
}

func requestedCardDAVProperties(request cardDAVXMLRequest, available map[xml.Name]string) (ok []cardDAVProperty, missing []xml.Name) {
	if len(request.Prop.Names) == 0 {
		for name, value := range available {
			ok = append(ok, cardDAVProperty{Name: name, Inner: value})
		}
		sort.Slice(ok, func(left, right int) bool {
			return ok[left].Name.Space+ok[left].Name.Local < ok[right].Name.Space+ok[right].Name.Local
		})
		return ok, nil
	}
	for _, property := range request.Prop.Names {
		name := property.XMLName
		if value, found := available[name]; found {
			ok = append(ok, cardDAVProperty{Name: name, Inner: value})
		} else {
			missing = append(missing, name)
		}
	}
	return ok, missing
}

func cloneCardDAVProperties(source map[xml.Name]string) map[xml.Name]string {
	result := make(map[xml.Name]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cardName(local string) xml.Name { return xml.Name{Space: cardDAVNamespace, Local: local} }
func calendarServerName(local string) xml.Name {
	return xml.Name{Space: calendarServerNS, Local: local}
}

func cardDAVMultistatus(responses []cardDAVResponse) []byte {
	return cardDAVMultistatusWithToken(responses, "")
}

func cardDAVMultistatusWithToken(responses []cardDAVResponse, token string) []byte {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?><d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav" xmlns:cs="http://calendarserver.org/ns/">`)
	for _, response := range responses {
		out.WriteString(`<d:response><d:href>` + xmlText(response.Href) + `</d:href>`)
		if response.StatusCode != 0 {
			out.WriteString(`<d:status>HTTP/1.1 ` + fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode)) + `</d:status></d:response>`)
			continue
		}
		writeCardDAVPropstat(&out, response.OK, http.StatusOK)
		if len(response.NotFound) > 0 {
			missing := make([]cardDAVProperty, 0, len(response.NotFound))
			for _, name := range response.NotFound {
				missing = append(missing, cardDAVProperty{Name: name})
			}
			writeCardDAVPropstat(&out, missing, http.StatusNotFound)
		}
		out.WriteString(`</d:response>`)
	}
	if token != "" {
		out.WriteString(`<d:sync-token>` + xmlText(token) + `</d:sync-token>`)
	}
	out.WriteString(`</d:multistatus>`)
	return []byte(out.String())
}

func writeCardDAVPropstat(out *strings.Builder, properties []cardDAVProperty, status int) {
	if len(properties) == 0 {
		return
	}
	out.WriteString(`<d:propstat><d:prop>`)
	for _, property := range properties {
		prefix := "d"
		switch property.Name.Space {
		case cardDAVNamespace:
			prefix = "card"
		case calendarServerNS:
			prefix = "cs"
		}
		opening := `<` + prefix + `:` + property.Name.Local + `>`
		if property.Name == cardName("address-data") {
			version := "3.0"
			if strings.Contains(property.Inner, "VERSION:4.0") {
				version = "4.0"
			}
			opening = `<card:address-data content-type="text/vcard" version="` + version + `">`
		}
		out.WriteString(opening + property.Inner + `</` + prefix + `:` + property.Name.Local + `>`)
	}
	out.WriteString(`</d:prop><d:status>HTTP/1.1 ` + fmt.Sprintf("%d %s", status, http.StatusText(status)) + `</d:status></d:propstat>`)
}

func xmlText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}
