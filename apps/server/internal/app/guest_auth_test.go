package app

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestEnsureGuestUserCreatesReusableUserAndToken(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()
	if err := EnsureCollections(testApp); err != nil {
		t.Fatal(err)
	}

	first, err := ensureGuestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureGuestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}
	if first.Id != second.Id {
		t.Fatalf("guest user changed from %q to %q", first.Id, second.Id)
	}
	if first.Email() != guestUserEmail || first.GetBool("disabled") || first.GetBool("isAdmin") || !first.Verified() {
		t.Fatalf("guest user has unexpected state: %#v", first)
	}

	token, err := first.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := testApp.FindAuthRecordByToken(token, core.TokenTypeAuth)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated == nil || authenticated.Id != first.Id {
		t.Fatalf("authenticated user = %#v", authenticated)
	}
}
