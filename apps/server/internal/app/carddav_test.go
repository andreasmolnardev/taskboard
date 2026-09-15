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
