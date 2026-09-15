package app

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

const (
	davNamespace    = "DAV:"
	calDAVNamespace = "urn:ietf:params:xml:ns:caldav"
	appleNamespace  = "http://apple.com/ns/ical/"
)

type calDAVXMLRequest struct {
	XMLName   xml.Name
	Prop      calDAVPropertyRequest `xml:"prop"`
	Hrefs     []string              `xml:"href"`
	Filter    calDAVFilter          `xml:"filter"`
	SyncToken string                `xml:"sync-token"`
	SyncLevel string                `xml:"sync-level"`
}

type calDAVPropertyRequest struct {
	Names []calDAVRequestedProperty `xml:",any"`
}

type calDAVRequestedProperty struct {
	XMLName xml.Name
}

type calDAVFilter struct {
	Components []calDAVComponentFilter `xml:"comp-filter"`
}

type calDAVComponentFilter struct {
	Name       string                  `xml:"name,attr"`
	TimeRange  *calDAVTimeRange        `xml:"time-range"`
	Components []calDAVComponentFilter `xml:"comp-filter"`
}

type calDAVTimeRange struct {
	Start string `xml:"start,attr"`
	End   string `xml:"end,attr"`
}

type calDAVProperty struct {
	Name  xml.Name
	Inner string
}

type calDAVResponse struct {
	Href       string
	OK         []calDAVProperty
	NotFound   []xml.Name
	StatusCode int
}

func parseCalDAVXML(body string) (calDAVXMLRequest, error) {
	var request calDAVXMLRequest
	if strings.TrimSpace(body) == "" {
		return request, nil
	}
	if err := xml.Unmarshal([]byte(body), &request); err != nil {
		return request, fmt.Errorf("invalid DAV XML: %w", err)
	}
	return request, nil
}

func calDAVMultistatus(responses []calDAVResponse) []byte {
	return calDAVMultistatusWithToken(responses, "")
}

func calDAVMultistatusWithToken(responses []calDAVResponse, token string) []byte {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:a="http://apple.com/ns/ical/">`)
	for _, response := range responses {
		out.WriteString(`<d:response><d:href>` + xmlText(response.Href) + `</d:href>`)
		if response.StatusCode != 0 {
			out.WriteString(`<d:status>HTTP/1.1 ` + fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode)) + `</d:status></d:response>`)
			continue
		}
		writeCalDAVPropstat(&out, response.OK, http.StatusOK)
		if len(response.NotFound) > 0 {
			missing := make([]calDAVProperty, 0, len(response.NotFound))
			for _, name := range response.NotFound {
				missing = append(missing, calDAVProperty{Name: name})
			}
			writeCalDAVPropstat(&out, missing, http.StatusNotFound)
		}
		out.WriteString(`</d:response>`)
	}
	if token != "" {
		out.WriteString(`<d:sync-token>` + xmlText(token) + `</d:sync-token>`)
	}
	out.WriteString(`</d:multistatus>`)
	return []byte(out.String())
}

func writeCalDAVPropstat(out *strings.Builder, properties []calDAVProperty, status int) {
	if len(properties) == 0 {
		return
	}
	out.WriteString(`<d:propstat><d:prop>`)
	for _, property := range properties {
		prefix := "d"
		switch property.Name.Space {
		case calDAVNamespace:
			prefix = "c"
		case appleNamespace:
			prefix = "a"
		}
		out.WriteString(`<` + prefix + `:` + property.Name.Local + `>` + property.Inner + `</` + prefix + `:` + property.Name.Local + `>`)
	}
	out.WriteString(`</d:prop><d:status>HTTP/1.1 ` + fmt.Sprintf("%d %s", status, http.StatusText(status)) + `</d:status></d:propstat>`)
}

func requestedCalDAVProperties(request calDAVXMLRequest, available map[xml.Name]string) (ok []calDAVProperty, missing []xml.Name) {
	if len(request.Prop.Names) == 0 {
		for name, value := range available {
			ok = append(ok, calDAVProperty{Name: name, Inner: value})
		}
		return ok, nil
	}
	for _, property := range request.Prop.Names {
		name := property.XMLName
		if value, found := available[name]; found {
			ok = append(ok, calDAVProperty{Name: name, Inner: value})
		} else {
			missing = append(missing, name)
		}
	}
	return ok, missing
}

func davName(local string) xml.Name    { return xml.Name{Space: davNamespace, Local: local} }
func calDAVName(local string) xml.Name { return xml.Name{Space: calDAVNamespace, Local: local} }
func appleName(local string) xml.Name  { return xml.Name{Space: appleNamespace, Local: local} }
