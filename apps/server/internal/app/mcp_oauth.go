package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var errInvalidMCPOAuthToken = errors.New("invalid MCP OAuth token")

// mcpOAuthClaims binds a user token to the MCP resource server. Tokens issued
// for PocketBase or another API must not be accepted by the MCP endpoint.
type mcpOAuthClaims struct {
	jwt.RegisteredClaims
}

func mcpResourceURL(config Config) string {
	if strings.TrimSpace(config.MCPResourceURL) != "" {
		return strings.TrimRight(config.MCPResourceURL, "/")
	}
	path := config.MCPPath
	if path == "" {
		path = "/mcp"
	}
	return strings.TrimRight(config.PublicURL, "/") + "/" + strings.Trim(path, "/")
}

func mcpIssuer(config Config) string {
	return strings.TrimRight(config.PublicURL, "/")
}

// mcpUserFromRequest verifies a resource-bound OAuth access token and returns
// its active PocketBase user. The authorization server must mint HS256 tokens
// with iss, aud, sub, and exp claims matching these checks.
func mcpUserFromRequest(app core.App, request *http.Request, config Config) (*core.Record, error) {
	if len(config.MCPOAuthSigningKey) < 32 {
		return nil, errInvalidMCPOAuthToken
	}
	parts := strings.Fields(request.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, errInvalidMCPOAuthToken
	}
	claims := &mcpOAuthClaims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errInvalidMCPOAuthToken
		}
		return []byte(config.MCPOAuthSigningKey), nil
	}, jwt.WithIssuer(mcpIssuer(config)), jwt.WithAudience(mcpResourceURL(config)))
	if err != nil || token == nil || !token.Valid || claims.Subject == "" {
		return nil, errInvalidMCPOAuthToken
	}
	if claims.ID != "" {
		var exists int
		if err := app.DB().NewQuery(`SELECT 1 FROM mcp_oauth_sessions WHERE id={:id} AND expires > {:now}`).Bind(dbx.Params{"id": claims.ID, "now": time.Now().Unix()}).Row(&exists); err != nil {
			return nil, errInvalidMCPOAuthToken
		}
	}
	user, err := app.FindRecordById("users", claims.Subject)
	if err != nil || user.GetBool("disabled") {
		return nil, errInvalidMCPOAuthToken
	}
	return user, nil
}

// validMCPCodeChallenge checks the S256 PKCE binding for an authorization code.
func validMCPCodeChallenge(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 || challenge == "" {
		return false
	}
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:]) == challenge
}

func validMCPCodeChallengeFormat(challenge string) bool {
	if len(challenge) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)
	return err == nil && len(decoded) == sha256.Size
}

func mcpProtectedResourceMetadata(config Config) map[string]any {
	return map[string]any{
		"resource":                 mcpResourceURL(config),
		"authorization_servers":    []string{mcpIssuer(config)},
		"bearer_methods_supported": []string{"header"},
	}
}

// registerMCPOAuthRoutes installs the public OAuth 2.1 endpoints used by MCP
// clients. Codes are one-use, short-lived, and PKCE-bound. OAuth clients are
// public clients; the client ID and redirect URI are checked at each step.
func registerMCPOAuthRoutes(e *core.ServeEvent, config Config) error {
	if len(config.MCPOAuthSigningKey) < 32 {
		return nil // OAuth remains disabled until an operator sets a strong key.
	}
	if _, err := e.App.DB().NewQuery(`CREATE TABLE IF NOT EXISTS mcp_oauth_clients (id TEXT PRIMARY KEY, redirects TEXT NOT NULL, created TEXT NOT NULL)`).Execute(); err != nil {
		return fmt.Errorf("create MCP OAuth client table: %w", err)
	}
	if _, err := e.App.DB().NewQuery(`CREATE TABLE IF NOT EXISTS mcp_oauth_codes (code_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL, client_id TEXT NOT NULL, redirect_uri TEXT NOT NULL, challenge TEXT NOT NULL, resource TEXT NOT NULL, expires INTEGER NOT NULL)`).Execute(); err != nil {
		return fmt.Errorf("create MCP OAuth code table: %w", err)
	}
	if _, err := e.App.DB().NewQuery(`CREATE TABLE IF NOT EXISTS mcp_oauth_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, client_id TEXT NOT NULL, resource TEXT NOT NULL, refresh_hash TEXT NOT NULL UNIQUE, expires INTEGER NOT NULL, created INTEGER NOT NULL)`).Execute(); err != nil {
		return fmt.Errorf("create MCP OAuth session table: %w", err)
	}
	e.Router.GET("/api/mcp/clients", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		var rows []struct {
			ClientID string `db:"client_id" json:"id"`
			Created  string `db:"created" json:"created"`
			Sessions int    `db:"sessions" json:"sessions"`
		}
		err := event.App.DB().NewQuery(`SELECT c.id AS client_id,c.created AS created,COUNT(s.id) AS sessions FROM mcp_oauth_clients c JOIN mcp_oauth_sessions s ON s.client_id=c.id WHERE s.user_id={:user} AND s.expires>{:now} GROUP BY c.id,c.created ORDER BY c.created DESC`).Bind(dbx.Params{"user": event.Auth.Id, "now": time.Now().Unix()}).All(&rows)
		if err != nil {
			return event.InternalServerError("could not list authorized MCP clients", err)
		}
		if rows == nil {
			rows = []struct {
				ClientID string `db:"client_id" json:"id"`
				Created  string `db:"created" json:"created"`
				Sessions int    `db:"sessions" json:"sessions"`
			}{}
		}
		return event.JSON(http.StatusOK, rows)
	})
	e.Router.DELETE("/api/mcp/clients/{id}", func(event *core.RequestEvent) error {
		if event.Auth == nil {
			return event.UnauthorizedError("authentication required", nil)
		}
		result, err := event.App.DB().NewQuery(`DELETE FROM mcp_oauth_sessions WHERE user_id={:user} AND client_id={:client}`).Bind(dbx.Params{"user": event.Auth.Id, "client": event.Request.PathValue("id")}).Execute()
		if err != nil {
			return event.InternalServerError("could not revoke MCP client", err)
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return event.NotFoundError("authorized MCP client not found", nil)
		}
		return event.NoContent(http.StatusNoContent)
	})
	e.Router.GET("/.well-known/oauth-protected-resource", func(event *core.RequestEvent) error {
		return event.JSON(http.StatusOK, mcpProtectedResourceMetadata(config))
	})
	metadataPath := "/.well-known/oauth-protected-resource" + "/" + strings.Trim(strings.TrimSpace(config.MCPPath), "/")
	if metadataPath != "/.well-known/oauth-protected-resource/" {
		e.Router.GET(metadataPath, func(event *core.RequestEvent) error {
			return event.JSON(http.StatusOK, mcpProtectedResourceMetadata(config))
		})
	}
	e.Router.GET("/.well-known/oauth-authorization-server", func(event *core.RequestEvent) error {
		base := mcpIssuer(config)
		return event.JSON(http.StatusOK, map[string]any{
			"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token",
			"registration_endpoint": base + "/oauth/register", "response_types_supported": []string{"code"},
			"response_modes_supported": []string{"query"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
			"revocation_endpoint":          base + "/oauth/revoke",
			"resource_parameter_supported": true,
		})
	})
	e.Router.POST("/oauth/register", func(event *core.RequestEvent) error {
		var input struct {
			RedirectURIs []string `json:"redirect_uris"`
		}
		if err := event.BindBody(&input); err != nil || len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > 20 {
			return event.BadRequestError("valid redirect_uris are required", err)
		}
		for _, redirect := range input.RedirectURIs {
			if !validMCPRedirectURI(redirect) {
				return event.BadRequestError("redirect URI must use HTTPS (localhost is allowed for development)", nil)
			}
		}
		id, err := randomMCPValue(24)
		if err != nil {
			return event.InternalServerError("could not register client", err)
		}
		data, _ := json.Marshal(input.RedirectURIs)
		if _, err := event.App.DB().NewQuery(`INSERT INTO mcp_oauth_clients (id, redirects, created) VALUES ({:id}, {:redirects}, {:created})`).Bind(dbx.Params{"id": id, "redirects": string(data), "created": time.Now().UTC().Format(time.RFC3339)}).Execute(); err != nil {
			return event.InternalServerError("could not register client", err)
		}
		return event.JSON(http.StatusCreated, map[string]any{"client_id": id, "client_id_issued_at": time.Now().Unix(), "redirect_uris": input.RedirectURIs, "token_endpoint_auth_method": "none"})
	})
	e.Router.GET("/oauth/authorize", func(event *core.RequestEvent) error {
		q := event.Request.URL.Query()
		if q.Get("response_type") != "code" || q.Get("client_id") == "" || q.Get("state") == "" || q.Get("resource") != mcpResourceURL(config) || q.Get("code_challenge_method") != "S256" || !validMCPCodeChallengeFormat(q.Get("code_challenge")) {
			return event.BadRequestError("invalid OAuth authorization request", nil)
		}
		if !mcpRegisteredRedirect(event.App, q.Get("client_id"), q.Get("redirect_uri")) {
			return event.BadRequestError("unregistered redirect URI", nil)
		}
		csrf, err := randomMCPValue(24)
		if err != nil {
			return event.InternalServerError("could not start login", err)
		}
		http.SetCookie(event.Response, &http.Cookie{Name: "mcp_oauth_csrf", Value: csrf, Path: "/oauth", HttpOnly: true, Secure: event.Request.URL.Scheme == "https" || event.Request.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: 600})
		form := template.HTML(`<form method="post" action="/oauth/authorize"><input type="hidden" name="csrf" value="` + template.HTMLEscapeString(csrf) + `">`)
		for key, values := range q {
			if key != "" {
				for _, value := range values {
					form += template.HTML(`<input type="hidden" name="` + template.HTMLEscapeString(key) + `" value="` + template.HTMLEscapeString(value) + `">`)
				}
			}
		}
		form += `Email <input name="email" type="email" required><br>Password <input name="password" type="password" required><br><label><input name="approve" type="checkbox" value="yes" required>Allow this client to access my Taskboard data</label><button>Allow</button></form>`
		return event.HTML(http.StatusOK, `<!doctype html><meta name="viewport" content="width=device-width"><title>Taskboard access</title><h1>Sign in to Taskboard</h1>`+string(form))
	})
	e.Router.POST("/oauth/authorize", func(event *core.RequestEvent) error {
		if err := event.Request.ParseForm(); err != nil {
			return event.BadRequestError("invalid form", err)
		}
		loginKey := appPasswordThrottleKey("mcp_oauth_email", strings.ToLower(strings.TrimSpace(event.Request.FormValue("email"))))
		ipKey := appPasswordThrottleKey("mcp_oauth_ip", event.RemoteIP())
		if !appPasswordLimiter.allowed(loginKey, ipKey) {
			return event.UnauthorizedError("email or password is incorrect", nil)
		}
		cookie, err := event.Request.Cookie("mcp_oauth_csrf")
		if err != nil || cookie.Value == "" || cookie.Value != event.Request.FormValue("csrf") {
			return event.ForbiddenError("invalid login session", nil)
		}
		q := event.Request.Form
		if q.Get("approve") != "yes" || q.Get("response_type") != "code" || !mcpRegisteredRedirect(event.App, q.Get("client_id"), q.Get("redirect_uri")) {
			return event.BadRequestError("invalid OAuth authorization request", nil)
		}
		user, err := event.App.FindAuthRecordByEmail("users", q.Get("email"))
		if err != nil || user == nil || user.GetBool("disabled") || !user.ValidatePassword(q.Get("password")) {
			appPasswordLimiter.recordFailure(loginKey, ipKey)
			return event.UnauthorizedError("email or password is incorrect", nil)
		}
		appPasswordLimiter.reset(loginKey, ipKey)
		code, err := randomMCPValue(32)
		if err != nil {
			return event.InternalServerError("could not issue authorization code", err)
		}
		codeHash := sha256.Sum256([]byte(code))
		if _, err := event.App.DB().NewQuery(`INSERT INTO mcp_oauth_codes (code_hash, user_id, client_id, redirect_uri, challenge, resource, expires) VALUES ({:hash}, {:user}, {:client}, {:redirect}, {:challenge}, {:resource}, {:expires})`).Bind(dbx.Params{"hash": base64.RawURLEncoding.EncodeToString(codeHash[:]), "user": user.Id, "client": q.Get("client_id"), "redirect": q.Get("redirect_uri"), "challenge": q.Get("code_challenge"), "resource": q.Get("resource"), "expires": time.Now().Add(5 * time.Minute).Unix()}).Execute(); err != nil {
			return event.InternalServerError("could not issue authorization code", err)
		}
		cookie.MaxAge = -1
		http.SetCookie(event.Response, cookie)
		callback, _ := url.Parse(q.Get("redirect_uri"))
		params := callback.Query()
		params.Set("code", code)
		params.Set("state", q.Get("state"))
		callback.RawQuery = params.Encode()
		return event.Redirect(http.StatusFound, callback.String())
	})
	e.Router.POST("/oauth/token", func(event *core.RequestEvent) error {
		if err := event.Request.ParseForm(); err != nil {
			return oauthError(event, http.StatusBadRequest, "invalid_request")
		}
		grantType := event.Request.FormValue("grant_type")
		if grantType == "refresh_token" {
			return rotateMCPRefreshToken(event, config)
		}
		if grantType != "authorization_code" {
			return oauthError(event, http.StatusBadRequest, "unsupported_grant_type")
		}
		if event.Request.FormValue("resource") != mcpResourceURL(config) {
			return oauthError(event, http.StatusBadRequest, "invalid_target")
		}
		code := event.Request.FormValue("code")
		digest := sha256.Sum256([]byte(code))
		hash := base64.RawURLEncoding.EncodeToString(digest[:])
		var row struct {
			UserID      string `db:"user_id"`
			ClientID    string `db:"client_id"`
			RedirectURI string `db:"redirect_uri"`
			Challenge   string `db:"challenge"`
			Resource    string `db:"resource"`
			Expires     int64  `db:"expires"`
		}
		err := event.App.DB().NewQuery(`SELECT user_id, client_id, redirect_uri, challenge, resource, expires FROM mcp_oauth_codes WHERE code_hash={:hash}`).Bind(dbx.Params{"hash": hash}).One(&row)
		if err != nil {
			return oauthError(event, http.StatusBadRequest, "invalid_grant")
		}
		if _, err := event.App.DB().NewQuery(`DELETE FROM mcp_oauth_codes WHERE code_hash={:hash}`).Bind(dbx.Params{"hash": hash}).Execute(); err != nil {
			return event.InternalServerError("could not consume authorization code", err)
		}
		if row.Expires < time.Now().Unix() || row.ClientID != event.Request.FormValue("client_id") || row.RedirectURI != event.Request.FormValue("redirect_uri") || row.Resource != event.Request.FormValue("resource") || !validMCPCodeChallenge(event.Request.FormValue("code_verifier"), row.Challenge) {
			return oauthError(event, http.StatusBadRequest, "invalid_grant")
		}
		user, err := event.App.FindRecordById("users", row.UserID)
		if err != nil || user.GetBool("disabled") {
			return oauthError(event, http.StatusBadRequest, "invalid_grant")
		}
		sessionID, err := randomMCPValue(24)
		if err != nil {
			return event.InternalServerError("could not create token session", err)
		}
		refresh, err := randomMCPValue(48)
		if err != nil {
			return event.InternalServerError("could not create refresh token", err)
		}
		refreshHash := hashMCPSecret(refresh)
		now := time.Now()
		if _, err := event.App.DB().NewQuery(`INSERT INTO mcp_oauth_sessions (id,user_id,client_id,resource,refresh_hash,expires,created) VALUES ({:id},{:user},{:client},{:resource},{:hash},{:expires},{:created})`).Bind(dbx.Params{"id": sessionID, "user": user.Id, "client": row.ClientID, "resource": row.Resource, "hash": refreshHash, "expires": now.Add(30 * 24 * time.Hour).Unix(), "created": now.Unix()}).Execute(); err != nil {
			return event.InternalServerError("could not create token session", err)
		}
		access, err := mintMCPAccessToken(user.Id, sessionID, config, now)
		if err != nil {
			return event.InternalServerError("could not issue access token", err)
		}
		return event.JSON(http.StatusOK, map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "refresh_token_expires_in": int((30 * 24 * time.Hour).Seconds()), "scope": "taskboard"})
	})
	e.Router.POST("/oauth/revoke", func(event *core.RequestEvent) error {
		if err := event.Request.ParseForm(); err != nil {
			return oauthError(event, http.StatusBadRequest, "invalid_request")
		}
		secret := event.Request.FormValue("token")
		if secret == "" || event.Request.FormValue("client_id") == "" {
			return oauthError(event, http.StatusBadRequest, "invalid_request")
		}
		_, err := event.App.DB().NewQuery(`DELETE FROM mcp_oauth_sessions WHERE client_id={:client} AND refresh_hash={:hash}`).Bind(dbx.Params{"client": event.Request.FormValue("client_id"), "hash": hashMCPSecret(secret)}).Execute()
		if err == nil {
			claims := &mcpOAuthClaims{}
			parsed, parseErr := jwt.ParseWithClaims(secret, claims, func(token *jwt.Token) (any, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, errInvalidMCPOAuthToken
				}
				return []byte(config.MCPOAuthSigningKey), nil
			}, jwt.WithIssuer(mcpIssuer(config)), jwt.WithAudience(mcpResourceURL(config)))
			if parseErr == nil && parsed != nil && parsed.Valid && claims.ID != "" {
				_, err = event.App.DB().NewQuery(`DELETE FROM mcp_oauth_sessions WHERE id={:id} AND client_id={:client}`).Bind(dbx.Params{"id": claims.ID, "client": event.Request.FormValue("client_id")}).Execute()
			}
		}
		if err != nil {
			return event.InternalServerError("could not revoke token", err)
		}
		return event.JSON(http.StatusOK, map[string]string{})
	})
	return nil
}

func hashMCPSecret(value string) string {
	digest := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func mintMCPAccessToken(userID, sessionID string, config Config, now time.Time) (string, error) {
	claims := mcpOAuthClaims{RegisteredClaims: jwt.RegisteredClaims{Issuer: mcpIssuer(config), Subject: userID, ID: sessionID, Audience: jwt.ClaimStrings{mcpResourceURL(config)}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.MCPOAuthSigningKey))
}

func rotateMCPRefreshToken(event *core.RequestEvent, config Config) error {
	if event.Request.FormValue("resource") != mcpResourceURL(config) {
		return oauthError(event, http.StatusBadRequest, "invalid_target")
	}
	clientID := event.Request.FormValue("client_id")
	oldToken := event.Request.FormValue("refresh_token")
	if clientID == "" || oldToken == "" {
		return oauthError(event, http.StatusBadRequest, "invalid_request")
	}
	var session struct {
		ID     string `db:"id"`
		UserID string `db:"user_id"`
	}
	err := event.App.DB().NewQuery(`SELECT id,user_id FROM mcp_oauth_sessions WHERE client_id={:client} AND resource={:resource} AND refresh_hash={:hash} AND expires>{:now}`).Bind(dbx.Params{"client": clientID, "resource": mcpResourceURL(config), "hash": hashMCPSecret(oldToken), "now": time.Now().Unix()}).One(&session)
	if err != nil {
		return oauthError(event, http.StatusBadRequest, "invalid_grant")
	}
	user, err := event.App.FindRecordById("users", session.UserID)
	if err != nil || user.GetBool("disabled") {
		return oauthError(event, http.StatusBadRequest, "invalid_grant")
	}
	refresh, err := randomMCPValue(48)
	if err != nil {
		return event.InternalServerError("could not rotate refresh token", err)
	}
	result, err := event.App.DB().NewQuery(`UPDATE mcp_oauth_sessions SET refresh_hash={:next} WHERE id={:id} AND refresh_hash={:old} AND expires>{:now}`).Bind(dbx.Params{"next": hashMCPSecret(refresh), "id": session.ID, "old": hashMCPSecret(oldToken), "now": time.Now().Unix()}).Execute()
	if err != nil {
		return event.InternalServerError("could not rotate refresh token", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return oauthError(event, http.StatusBadRequest, "invalid_grant")
	}
	access, err := mintMCPAccessToken(user.Id, session.ID, config, time.Now())
	if err != nil {
		return event.InternalServerError("could not issue access token", err)
	}
	return event.JSON(http.StatusOK, map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "refresh_token_expires_in": 30 * 24 * 3600, "scope": "taskboard"})
}

func validMCPRedirectURI(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))
}

func mcpRegisteredRedirect(app core.App, client, redirect string) bool {
	var raw string
	err := app.DB().NewQuery(`SELECT redirects FROM mcp_oauth_clients WHERE id={:id}`).Bind(dbx.Params{"id": client}).Row(&raw)
	if err != nil {
		return false
	}
	var uris []string
	if json.Unmarshal([]byte(raw), &uris) != nil {
		return false
	}
	for _, uri := range uris {
		if uri == redirect {
			return true
		}
	}
	return false
}

func randomMCPValue(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func oauthError(event *core.RequestEvent, status int, code string) error {
	return event.JSON(status, map[string]string{"error": code})
}
