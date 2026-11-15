package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/auth"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
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

func TestOAuthEmailMatchCannotSilentlyLinkExistingAccount(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := EnsureCollections(app); err != nil {
		t.Fatal(err)
	}
	user, err := makeUser(app, "member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	event := oauthAssociationEvent(app, users, user, "provider-user-1", "member@example.com", false)
	assertForbidden(t, validateOAuthAssociation(event, Config{RegistrationMode: RegistrationApproval}))

	linked := core.NewExternalAuth(app)
	linked.SetCollectionRef(users.Id)
	linked.SetRecordRef(user.Id)
	linked.SetProvider("oidc2")
	linked.SetProviderId("provider-user-1")
	if err := app.Save(linked); err != nil {
		t.Fatal(err)
	}
	if err := validateOAuthAssociation(event, Config{RegistrationMode: RegistrationApproval}); err != nil {
		t.Fatalf("previously linked identity should remain usable: %v", err)
	}
}

func TestOAuthAssociationCannotSwitchAuthenticatedAccount(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := EnsureCollections(app); err != nil {
		t.Fatal(err)
	}
	current, err := makeUser(app, "current@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other, err := makeUser(app, "other@example.com")
	if err != nil {
		t.Fatal(err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	event := oauthAssociationEvent(app, users, other, "provider-user-2", "other@example.com", false)
	event.Auth = current
	assertForbidden(t, validateOAuthAssociation(event, Config{RegistrationMode: RegistrationApproval}))
}

func TestOAuthSignupRequiresProviderEmailWhenVerificationIsRequired(t *testing.T) {
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
	event := oauthAssociationEvent(app, users, nil, "provider-user-3", "", true)
	assertForbidden(t, validateOAuthAssociation(event, Config{RegistrationMode: RegistrationApproval, RequireVerifiedEmail: true}))
	event.OAuth2User.Email = "verified@example.com"
	if err := validateOAuthAssociation(event, Config{RegistrationMode: RegistrationApproval, RequireVerifiedEmail: true}); err != nil {
		t.Fatalf("provider email should allow signup under this policy: %v", err)
	}
}

func TestOAuthAdminSettingsPreserveSecretsByProvider(t *testing.T) {
	oldSecret := "old-secret"
	existing := []core.OAuth2ProviderConfig{
		{Name: "google", ClientSecret: "google-secret"},
		{Name: "oidc2", ClientSecret: oldSecret},
	}
	incoming := []adminOAuthProviderInput{
		{Name: "google", ClientID: "google-client"},
		{Name: "oidc2", ClientID: "oidc-client"},
	}
	providers := preserveOAuthProviderSecrets(existing, incoming)
	if len(providers) != 2 || providers[0].ClientSecret != "google-secret" || providers[1].ClientSecret != oldSecret {
		t.Fatalf("blank secrets must preserve secrets for submitted providers: %#v", providers)
	}
	publicConfig, err := json.Marshal(redactOAuthProvider(existing[1]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicConfig), "secret") {
		t.Fatalf("OAuth settings response exposed a secret: %s", publicConfig)
	}
	incoming[1].ClientSecret = "rotated-secret"
	providers = preserveOAuthProviderSecrets(existing, incoming)
	if providers[1].ClientSecret != "rotated-secret" {
		t.Fatalf("explicit secret must replace the saved value, got %q", providers[1].ClientSecret)
	}
	providers = preserveOAuthProviderSecrets(existing, incoming[1:])
	if len(providers) != 1 || providers[0].Name != "oidc2" {
		t.Fatalf("omitted provider should be removed while submitted providers remain: %#v", providers)
	}
}

func TestAdminSsoSettingsAreSuperuserOnlyAndRedactSecrets(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.OAuth2.Enabled = true
	users.OAuth2.Providers = []core.OAuth2ProviderConfig{{Name: "oidc2", ClientId: "client", ClientSecret: "keep-this-secret", AuthURL: "https://idp.example/authorize", TokenURL: "https://idp.example/token", UserInfoURL: "https://idp.example/userinfo"}}
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}
	regular, err := makeUser(app, "ordinary@example.com")
	if err != nil {
		t.Fatal(err)
	}
	denied := &core.RequestEvent{App: app, Auth: regular, Event: router.Event{Request: httptest.NewRequest(http.MethodGet, "/api/admin/sso", nil), Response: httptest.NewRecorder()}}
	assertForbidden(t, getAdminSsoSettings(denied))

	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	admin := core.NewRecord(superusers)
	admin.SetEmail("admin@example.com")
	admin.SetPassword("secret123456")
	if err := app.Save(admin); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	event := &core.RequestEvent{App: app, Auth: admin, Event: router.Event{Request: httptest.NewRequest(http.MethodGet, "/api/admin/sso", nil), Response: response}}
	if err := getAdminSsoSettings(event); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(response.Body.String(), "keep-this-secret") || strings.Contains(response.Body.String(), "clientSecret") {
		t.Fatalf("admin settings GET exposed provider secret: %s", response.Body.String())
	}
}

func TestDisabledUserRealtimeClientsAreRevoked(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := EnsureCollections(app); err != nil {
		t.Fatal(err)
	}
	user, err := makeUser(app, "realtime@example.com")
	if err != nil {
		t.Fatal(err)
	}
	client := subscriptions.NewDefaultClient()
	client.Set(apis.RealtimeClientAuthKey, user)
	app.SubscriptionsBroker().Register(client)
	if revoked := revokeDisabledRealtimeClients(app, user.Id); revoked != 1 || !client.IsDiscarded() {
		t.Fatalf("disable should discard the user's realtime client: revoked=%d discarded=%v", revoked, client.IsDiscarded())
	}
}

func TestOIDCProviderRequiresIssuerAndJWKSAndUsesSignedIDToken(t *testing.T) {
	provider := core.OAuth2ProviderConfig{
		Name: "oidc2", ClientId: "client", AuthURL: "https://id.example/authorize", TokenURL: "https://id.example/token",
		UserInfoURL: "https://id.example/userinfo", Extra: map[string]any{"issuers": []string{"https://id.example/"}, "jwksURL": "https://id.example/keys"},
	}
	if err := validateOIDCProvider(provider); err != nil {
		t.Fatalf("valid OIDC provider rejected: %v", err)
	}
	provider.Extra["jwksURL"] = ""
	if err := validateOIDCProvider(provider); err == nil {
		t.Fatal("OIDC without JWKS must be rejected")
	}
	provider.Extra["jwksURL"] = "https://id.example/keys"
	provider.AuthURL = "http://id.example/authorize"
	if err := validateOIDCProvider(provider); err == nil {
		t.Fatal("insecure OIDC endpoint must be rejected")
	}
}

func oauthAssociationEvent(app core.App, users *core.Collection, record *core.Record, providerID, email string, isNew bool) *core.RecordAuthWithOAuth2RequestEvent {
	event := &core.RecordAuthWithOAuth2RequestEvent{
		RequestEvent: &core.RequestEvent{App: app},
		ProviderName: "oidc2",
		OAuth2User:   &auth.AuthUser{Id: providerID, Email: email},
		Record:       record,
		IsNewRecord:  isNew,
	}
	event.Request = httptest.NewRequest("POST", "/api/collections/users/auth-with-oauth2", nil)
	event.Collection = users
	return event
}

func makeUser(app core.App, email string) (*core.Record, error) {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("secret123456")
	return user, app.Save(user)
}

func assertForbidden(t *testing.T, err error) {
	t.Helper()
	var apiError *router.ApiError
	if !errors.As(err, &apiError) || apiError.Status != 403 {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}
