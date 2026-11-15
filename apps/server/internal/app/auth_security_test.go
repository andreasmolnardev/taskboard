package app

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestConsumeSsoLinkIntentIsBoundAndOneTime(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	app.Store().Set("sso-link-intent:valid", ssoLinkIntent{UserID: "user-a", ExpiresAt: time.Now().Add(time.Minute)})
	if !consumeSsoLinkIntent(app, "user-a", "valid") || consumeSsoLinkIntent(app, "user-a", "valid") {
		t.Fatal("intent must be valid once for its owner")
	}
	app.Store().Set("sso-link-intent:other", ssoLinkIntent{UserID: "user-a", ExpiresAt: time.Now().Add(time.Minute)})
	if consumeSsoLinkIntent(app, "user-b", "other") || consumeSsoLinkIntent(app, "user-a", "other") {
		t.Fatal("wrong-owner use must consume and reject intent")
	}
	app.Store().Set("sso-link-intent:expired", ssoLinkIntent{UserID: "user-a", ExpiresAt: time.Now().Add(-time.Minute)})
	if consumeSsoLinkIntent(app, "user-a", "expired") {
		t.Fatal("expired intent must be rejected")
	}
}

func TestSsoOnlyPasswordPolicyHasLockoutFallback(t *testing.T) {
	config := Config{SSOOnly: true}
	if !ssoOnlyBlocksPassword(config, true, 1) {
		t.Fatal("configured SSO-only policy must block password login")
	}
	if ssoOnlyBlocksPassword(config, false, 1) || ssoOnlyBlocksPassword(config, true, 0) {
		t.Fatal("password login must remain when no provider is usable")
	}
}

func TestOAuthSignupPolicyBlocksOnlyNewPublicAccounts(t *testing.T) {
	config := Config{RegistrationMode: RegistrationDisabled}
	if publicOAuthSignupAllowed(config, true, false) {
		t.Fatal("public first-time OAuth signup must be blocked")
	}
	if !publicOAuthSignupAllowed(config, false, false) {
		t.Fatal("existing users must still be able to sign in")
	}
	if !publicOAuthSignupAllowed(config, true, true) {
		t.Fatal("superusers must retain account recovery")
	}
}

func TestUserEmailCollisionIgnoresCase(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := EnsureCollections(app); err != nil {
		t.Fatal(err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("member@example.com")
	user.SetPassword("secret123456")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	candidate := core.NewRecord(users)
	candidate.SetEmail("MEMBER@EXAMPLE.COM")
	collision, err := userEmailCollision(app, candidate)
	if err != nil || !collision {
		t.Fatalf("case variant should collide: collision=%v err=%v", collision, err)
	}
}
