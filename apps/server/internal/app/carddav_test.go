package app

import (
	"strings"
	"testing"
)

func TestCardDAVHrefAcceptsClientResourceNames(t *testing.T) {
	got, err := cardDAVHref("client-contact.vcf")
	if err != nil || got != "client-contact.vcf" {
		t.Fatalf("href = %q, error = %v", got, err)
	}
	if _, err := cardDAVHref("../contact.vcf"); err == nil {
		t.Fatal("path traversal href was accepted")
	}
}

func TestNormalizeVCardUsesCRLF(t *testing.T) {
	got, err := normalizeVCard("BEGIN:VCARD\nVERSION:3.0\nFN:Example\nEND:VCARD\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\r\n") || strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("vCard was not normalized: %q", got)
	}
	if !strings.HasSuffix(got, "END:VCARD\r\n") {
		t.Fatalf("vCard does not end with CRLF: %q", got)
	}
}

func TestVCardParserUnfoldsEscapesAndPreservesParameters(t *testing.T) {
	input := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person@example.com\r\nFN:Jane Q. Doe\r\nNOTE:line one\\nline two\r\nEMAIL;TYPE=work: jane@example.com\r\n X\r\nX-CUSTOM;X-FOO=bar:baz\r\nEND:VCARD\r\n"
	card, err := parseVCardDocument([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if got := firstVCardValue(card, "NOTE"); got != "line one\nline two" {
		t.Fatalf("NOTE = %q", got)
	}
	values := propertyValues(card, "EMAIL")
	if len(values) != 1 || len(values[0].Parameters) != 1 || values[0].Parameters[0].Name != "TYPE" || values[0].Parameters[0].Values[0] != "work" {
		t.Fatalf("EMAIL parameters = %#v", values)
	}
	encoded, err := encodeVCardDocument(card)
	if err != nil || !strings.Contains(string(encoded), "X-CUSTOM;X-FOO=bar:baz") || !strings.Contains(string(encoded), "NOTE:line one\\nline two") {
		t.Fatalf("encoded vCard = %q, error = %v", encoded, err)
	}
}

func TestVCardParserRejectsInvalidCards(t *testing.T) {
	for _, input := range []string{
		"BEGIN:VCARD\r\nFN:Missing version\r\nEND:VCARD\r\n",
		"BEGIN:VCARD\r\nVERSION:3.0\r\nEND:VCARD\r\n",
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:one\r\nBEGIN:VCARD\r\nEND:VCARD\r\n",
	} {
		if _, err := parseVCardDocument([]byte(input)); err == nil {
			t.Fatalf("expected invalid vCard to fail: %q", input)
		}
	}
}

func TestCardDAVQueryMatchesProperties(t *testing.T) {
	card, err := parseVCardDocument([]byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:one\r\nFN:Jane Doe\r\nEMAIL:jane@example.com\r\nEND:VCARD\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cardDAVCardMatches(card, cardDAVFilter{Properties: []cardDAVPropFilter{{Name: "FN", TextMatch: &cardDAVTextMatch{Value: "doe"}}}}) {
		t.Fatal("FN text filter did not match")
	}
	if !cardDAVCardMatches(card, cardDAVFilter{Properties: []cardDAVPropFilter{{Name: "TEL", IsNotDefined: &struct{}{}}}}) {
		t.Fatal("is-not-defined filter did not match the missing TEL")
	}
}
