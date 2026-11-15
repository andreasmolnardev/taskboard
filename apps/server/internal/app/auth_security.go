package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
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
	pb.OnRecordAuthWithOAuth2Request().BindFunc(func(e *core.RecordAuthWithOAuth2RequestEvent) error {
		if !publicOAuthSignupAllowed(config, e.IsNewRecord, e.HasSuperuserAuth()) {
			return e.ForbiddenError("registration is disabled", nil)
		}
		if e.Auth != nil && e.Auth.Collection().Name == "users" {
			if e.Record != nil && e.Record.Id != e.Auth.Id {
				return e.ForbiddenError("this identity is linked to another account", nil)
			}
			if !consumeSsoLinkIntent(e.App, e.Auth.Id, e.Request.Header.Get("X-Taskboard-Link-Intent")) {
				return e.ForbiddenError("reauthenticate and confirm linking before adding an identity", nil)
			}
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

func registerAuthSecurityRoutes(e *core.ServeEvent, config Config) {
	e.Router.GET("/api/auth/policy", func(event *core.RequestEvent) error {
		return event.JSON(http.StatusOK, map[string]any{
			"registrationEnabled":  config.RegistrationMode != RegistrationDisabled,
			"requireVerifiedEmail": config.RequireVerifiedEmail,
			"ssoOnly":              config.SSOOnly,
		})
	})
	e.Router.POST("/api/me/sso/link-intent", createSsoLinkIntent)
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
