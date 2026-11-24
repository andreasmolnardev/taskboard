package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type ssoLinkIntent struct {
	UserID    string
	ExpiresAt time.Time
}

var ssoLinkIntentMu sync.Mutex

func registerAuthSecurityHooks(pb *pocketbase.PocketBase, config Config) {
	pb.OnRecordCreateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		e.Record.SetEmail(strings.ToLower(strings.TrimSpace(e.Record.Email())))
		if collision, err := userEmailCollision(e.App, e.Record); err != nil {
			return e.InternalServerError("could not check email uniqueness", err)
		} else if collision {
			return e.BadRequestError("an account already uses this email address", nil)
		}
		return e.Next()
	})
	pb.OnRecordUpdateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() && e.Record.GetBool("disabled") != e.Record.Original().GetBool("disabled") {
			return e.ForbiddenError("only an administrator can change account access", nil)
		}
		if !strings.EqualFold(e.Record.Email(), e.Record.Original().Email()) {
			e.Record.SetEmail(strings.ToLower(strings.TrimSpace(e.Record.Email())))
			if collision, err := userEmailCollision(e.App, e.Record); err != nil {
				return e.InternalServerError("could not check email uniqueness", err)
			} else if collision {
				return e.BadRequestError("an account already uses this email address", nil)
			}
		}
		return e.Next()
	})
	pb.OnRecordAfterUpdateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool("disabled") && !e.Record.Original().GetBool("disabled") {
			revokeDisabledRealtimeClients(e.App, e.Record.Id)
		}
		return e.Next()
	})
	pb.OnRecordDeleteRequest(core.CollectionNameExternalAuths).BindFunc(func(e *core.RecordRequestEvent) error {
		return protectLastExternalAuth(e)
	})
	pb.OnRecordAuthWithOAuth2Request().BindFunc(func(e *core.RecordAuthWithOAuth2RequestEvent) error {
		if err := validateOAuthAssociation(e, config); err != nil {
			return err
		}
		return e.Next()
	})
	pb.OnRecordAuthWithPasswordRequest("users").BindFunc(func(e *core.RecordAuthWithPasswordRequestEvent) error {
		collection, err := e.App.FindCollectionByNameOrId("users")
		if err == nil && ssoOnlyBlocksPassword(config, collection.OAuth2.Enabled, len(collection.OAuth2.Providers)) {
			return e.ForbiddenError("password sign-in is disabled by workspace policy", nil)
		}
		return e.Next()
	})
	pb.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if e.Record.GetBool("disabled") {
			return e.ForbiddenError("account is disabled", nil)
		}
		if config.RequireVerifiedEmail && !e.Record.Verified() {
			return e.ForbiddenError("verify your email before signing in", nil)
		}
		return e.Next()
	})
	pb.OnRecordAuthRefreshRequest("users").BindFunc(func(e *core.RecordAuthRefreshRequestEvent) error {
		if e.Record.GetBool("disabled") {
			return e.ForbiddenError("account is disabled", nil)
		}
		return e.Next()
	})
}

func protectLastExternalAuth(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	user, err := e.App.FindRecordById("users", e.Record.GetString("recordRef"))
	if err != nil {
		return e.ForbiddenError("cannot remove the last connected sign-in identity", nil)
	}
	identities, err := e.App.FindAllExternalAuthsByRecord(user)
	if err != nil {
		return e.InternalServerError("could not check connected identities", err)
	}
	if len(identities) <= 1 {
		return e.ForbiddenError("cannot remove the last connected sign-in identity", nil)
	}
	return e.Next()
}

func revokeDisabledRealtimeClients(app core.App, userID string) int {
	revoked := 0
	broker := app.SubscriptionsBroker()
	for _, client := range broker.Clients() {
		authRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
		if authRecord != nil && authRecord.Collection().Name == "users" && authRecord.Id == userID {
			broker.Unregister(client.Id())
			revoked++
		}
	}
	return revoked
}

// validateOAuthAssociation prevents PocketBase's email fallback from attaching
// an unlinked provider identity to an existing user without explicit consent.
func validateOAuthAssociation(e *core.RecordAuthWithOAuth2RequestEvent, config Config) error {
	if !publicOAuthSignupAllowed(config, e.IsNewRecord, e.HasSuperuserAuth()) {
		return e.ForbiddenError("registration is disabled", nil)
	}
	if config.RequireVerifiedEmail && e.IsNewRecord && (e.OAuth2User == nil || strings.TrimSpace(e.OAuth2User.Email) == "") {
		return e.ForbiddenError("the identity provider did not return a verified email address", nil)
	}

	userAuth := e.Auth != nil && e.Auth.Collection().Name == "users"
	if userAuth {
		if e.Record != nil && e.Record.Id != e.Auth.Id {
			return e.ForbiddenError("this identity is linked to another account", nil)
		}
		if !consumeSsoLinkIntent(e.App, e.Auth.Id, e.Request.Header.Get("X-Taskboard-Link-Intent")) {
			return e.ForbiddenError("reauthenticate and confirm linking before adding an identity", nil)
		}
		return nil
	}

	if e.Record == nil {
		return nil
	}
	if e.OAuth2User == nil || strings.TrimSpace(e.OAuth2User.Id) == "" {
		return e.ForbiddenError("could not validate the provider identity", nil)
	}
	externalAuth, err := e.App.FindFirstExternalAuthByExpr(dbx.HashExp{
		"collectionRef": e.Collection.Id,
		"provider":      e.ProviderName,
		"providerId":    e.OAuth2User.Id,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return e.InternalServerError("could not validate the provider identity", err)
	}
	if externalAuth == nil || externalAuth.RecordRef() != e.Record.Id {
		return e.ForbiddenError("sign in to the existing account and explicitly link this identity", nil)
	}
	return nil
}

func registerAuthSecurityRoutes(e *core.ServeEvent, config Config) {
	e.Router.GET("/api/auth/policy", func(event *core.RequestEvent) error {
		return event.JSON(http.StatusOK, map[string]any{
			"registrationEnabled":  config.RegistrationMode != RegistrationDisabled,
			"requireVerifiedEmail": config.RequireVerifiedEmail,
			"ssoOnly":              config.SSOOnly,
		})
	})
	e.Router.POST("/api/me/sso/link-intent", createSsoLinkIntent)
	e.Router.GET("/api/admin/sso", getAdminSsoSettings)
	e.Router.PATCH("/api/admin/sso", updateAdminSsoSettings)
}

type adminOAuthProviderInput struct {
	PKCE         *bool          `json:"pkce"`
	Name         string         `json:"name"`
	ClientID     string         `json:"clientId"`
	ClientSecret string         `json:"clientSecret"`
	AuthURL      string         `json:"authURL"`
	TokenURL     string         `json:"tokenURL"`
	UserInfoURL  string         `json:"userInfoURL"`
	DisplayName  string         `json:"displayName"`
	Extra        map[string]any `json:"extra"`
}

type adminOAuthProviderOutput struct {
	PKCE        *bool          `json:"pkce"`
	Name        string         `json:"name"`
	ClientID    string         `json:"clientId"`
	AuthURL     string         `json:"authURL"`
	TokenURL    string         `json:"tokenURL"`
	UserInfoURL string         `json:"userInfoURL"`
	DisplayName string         `json:"displayName"`
	Extra       map[string]any `json:"extra"`
}

func getAdminSsoSettings(event *core.RequestEvent) error {
	if !event.HasSuperuserAuth() {
		return event.ForbiddenError("superuser access required", nil)
	}
	collection, err := event.App.FindCollectionByNameOrId("users")
	if err != nil {
		return event.InternalServerError("could not load OAuth settings", err)
	}
	providers := make([]adminOAuthProviderOutput, 0, len(collection.OAuth2.Providers))
	for _, provider := range collection.OAuth2.Providers {
		providers = append(providers, redactOAuthProvider(provider))
	}
	return event.JSON(http.StatusOK, map[string]any{"oauth2": map[string]any{"enabled": collection.OAuth2.Enabled, "providers": providers}})
}

func redactOAuthProvider(provider core.OAuth2ProviderConfig) adminOAuthProviderOutput {
	return adminOAuthProviderOutput{
		PKCE: provider.PKCE, Name: provider.Name, ClientID: provider.ClientId,
		AuthURL: provider.AuthURL, TokenURL: provider.TokenURL, UserInfoURL: provider.UserInfoURL,
		DisplayName: provider.DisplayName, Extra: provider.Extra,
	}
}

func updateAdminSsoSettings(event *core.RequestEvent) error {
	if !event.HasSuperuserAuth() {
		return event.ForbiddenError("superuser access required", nil)
	}
	var input struct {
		OAuth2 struct {
			Enabled   bool                      `json:"enabled"`
			Providers []adminOAuthProviderInput `json:"providers"`
		} `json:"oauth2"`
	}
	if err := json.NewDecoder(event.Request.Body).Decode(&input); err != nil {
		return event.BadRequestError("invalid OAuth settings", err)
	}
	collection, err := event.App.FindCollectionByNameOrId("users")
	if err != nil {
		return event.InternalServerError("could not load OAuth settings", err)
	}
	collection.OAuth2.Enabled = input.OAuth2.Enabled
	collection.OAuth2.Providers = preserveOAuthProviderSecrets(collection.OAuth2.Providers, input.OAuth2.Providers)
	for i := range collection.OAuth2.Providers {
		provider := &collection.OAuth2.Providers[i]
		if provider.Name != "oidc2" {
			continue
		}
		if input.OAuth2.Enabled {
			if err := validateOIDCProvider(*provider); err != nil {
				return event.BadRequestError(err.Error(), nil)
			}
		}
		provider.UserInfoURL = ""
		pkce := true
		provider.PKCE = &pkce
	}
	if err := event.App.Save(collection); err != nil {
		return event.BadRequestError("could not save OAuth settings", err)
	}
	return getAdminSsoSettings(event)
}

func validateOIDCProvider(provider core.OAuth2ProviderConfig) error {
	issuers, _ := provider.Extra["issuers"].([]any)
	if issuers == nil {
		if stringsList, ok := provider.Extra["issuers"].([]string); ok {
			for _, value := range stringsList {
				issuers = append(issuers, value)
			}
		}
	}
	jwksURL, _ := provider.Extra["jwksURL"].(string)
	if len(issuers) == 0 || strings.TrimSpace(jwksURL) == "" || strings.TrimSpace(provider.ClientId) == "" {
		return errors.New("enabled custom OIDC requires a client ID, issuer, and JWKS URL")
	}
	issuer, _ := issuers[0].(string)
	for _, value := range []string{issuer, jwksURL, provider.AuthURL, provider.TokenURL} {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Hostname() != "localhost") {
			return errors.New("custom OIDC issuer, JWKS, authorization, and token URLs must use HTTPS")
		}
	}
	return nil
}

func preserveOAuthProviderSecrets(existing []core.OAuth2ProviderConfig, incoming []adminOAuthProviderInput) []core.OAuth2ProviderConfig {
	secrets := make(map[string]string, len(existing))
	for _, provider := range existing {
		secrets[provider.Name] = provider.ClientSecret
	}
	providers := make([]core.OAuth2ProviderConfig, 0, len(incoming))
	for _, provider := range incoming {
		secret := provider.ClientSecret
		if secret == "" {
			secret = secrets[provider.Name]
		}
		providers = append(providers, core.OAuth2ProviderConfig{
			PKCE: provider.PKCE, Name: provider.Name, ClientId: provider.ClientID, ClientSecret: secret,
			AuthURL: provider.AuthURL, TokenURL: provider.TokenURL, UserInfoURL: provider.UserInfoURL,
			DisplayName: provider.DisplayName, Extra: provider.Extra,
		})
	}
	return providers
}

func userEmailCollision(app core.App, record *core.Record) (bool, error) {
	email := strings.TrimSpace(record.Email())
	if email == "" {
		return false, nil
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return false, err
	}
	var exists int
	err = app.RecordQuery(users).Select("(1)").AndWhere(dbx.NewExp("lower([[email]]) = lower({:email}) AND [[id]] != {:id}", dbx.Params{"email": email, "id": record.Id})).Limit(1).Row(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func ssoOnlyBlocksPassword(config Config, oauthEnabled bool, providerCount int) bool {
	return config.SSOOnly && oauthEnabled && providerCount > 0
}

func publicOAuthSignupAllowed(config Config, isNewRecord, superuser bool) bool {
	return superuser || !isNewRecord || config.RegistrationMode != RegistrationDisabled
}

func consumeSsoLinkIntent(app core.App, userID, intent string) bool {
	if intent == "" {
		return false
	}
	ssoLinkIntentMu.Lock()
	defer ssoLinkIntentMu.Unlock()
	key := "sso-link-intent:" + intent
	entry, ok := app.Store().Get(key).(ssoLinkIntent)
	app.Store().Remove(key)
	return ok && entry.UserID == userID && time.Now().Before(entry.ExpiresAt)
}

func createSsoLinkIntent(event *core.RequestEvent) error {
	if event.Auth == nil || event.Auth.Collection().Name != "users" {
		return event.UnauthorizedError("sign in first", nil)
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(event.Request.Body).Decode(&input); err != nil || input.Password == "" || !event.Auth.ValidatePassword(input.Password) {
		return event.ForbiddenError("confirm your password to link a sign-in method", nil)
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return event.InternalServerError("could not create link intent", err)
	}
	intent := hex.EncodeToString(random)
	event.App.Store().Set("sso-link-intent:"+intent, ssoLinkIntent{UserID: event.Auth.Id, ExpiresAt: time.Now().Add(5 * time.Minute)})
	return event.JSON(http.StatusOK, map[string]string{"intent": intent})
}
