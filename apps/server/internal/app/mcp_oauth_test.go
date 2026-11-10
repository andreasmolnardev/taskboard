package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestValidMCPCodeChallenge(t *testing.T) {
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_."
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if !validMCPCodeChallenge(verifier, challenge) {
		t.Fatal("valid S256 verifier was rejected")
	}
	if validMCPCodeChallenge(verifier, "wrong") || validMCPCodeChallenge("short", challenge) {
		t.Fatal("invalid PKCE binding was accepted")
	}
}

func TestMCPOAuthRegisterAndExchangeAuthorizationCode(t *testing.T) {
	config := Config{PublicURL: "https://tasks.example", MCPResourceURL: "https://tasks.example/mcp", MCPOAuthSigningKey: "0123456789abcdef0123456789abcdef", MCPPath: "/mcp"}
	server := newHTTPIntegrationServer(t, config)
	user := integrationUser(t, server.app, "oauth@example.com")
	metadataResponse, err := server.client.Get(server.server.URL + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.NewDecoder(metadataResponse.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	metadataResponse.Body.Close()
	if metadata["authorization_endpoint"] != "https://tasks.example/oauth/authorize" {
		t.Fatalf("metadata = %#v", metadata)
	}
	resourceResponse, err := server.client.Get(server.server.URL + "/.well-known/oauth-protected-resource/mcp")
	if err != nil {
		t.Fatal(err)
	}
	var resourceMetadata map[string]any
	if err := json.NewDecoder(resourceResponse.Body).Decode(&resourceMetadata); err != nil {
		t.Fatal(err)
	}
	resourceResponse.Body.Close()
	if resourceMetadata["resource"] != config.MCPResourceURL {
		t.Fatalf("resource metadata = %#v", resourceMetadata)
	}

	registerBody := `{"redirect_uris":["http://localhost:4321/callback"]}`
	registerResponse, err := server.client.Post(server.server.URL+"/oauth/register", "application/json", strings.NewReader(registerBody))
	if err != nil {
		t.Fatal(err)
	}
	var client struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(registerResponse.Body).Decode(&client); err != nil {
		t.Fatal(err)
	}
	registerResponse.Body.Close()
	if registerResponse.StatusCode != 201 || client.ClientID == "" {
		t.Fatalf("client registration status=%d id=%q", registerResponse.StatusCode, client.ClientID)
	}

	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	params := url.Values{"response_type": {"code"}, "client_id": {client.ClientID}, "redirect_uri": {"http://localhost:4321/callback"}, "state": {"state-value"}, "code_challenge_method": {"S256"}, "code_challenge": {challenge}, "resource": {config.MCPResourceURL}}
	authorizeResponse, err := server.client.Get(server.server.URL + "/oauth/authorize?" + params.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if authorizeResponse.StatusCode != 200 {
		body, _ := io.ReadAll(authorizeResponse.Body)
		t.Fatalf("authorize status=%d body=%s", authorizeResponse.StatusCode, body)
	}
	authorizeResponse.Body.Close()
	cookie := authorizeResponse.Cookies()[0]
	form := url.Values{"csrf": {cookie.Value}, "email": {user.GetString("email")}, "password": {"password123456"}, "approve": {"yes"}}
	for key, values := range params {
		for _, value := range values {
			form.Add(key, value)
		}
	}
	request, err := http.NewRequest(http.MethodPost, server.server.URL+"/oauth/authorize", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(cookie)
	authorizeResponse, err = server.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if authorizeResponse.StatusCode != 302 {
		body, _ := io.ReadAll(authorizeResponse.Body)
		t.Fatalf("authorize POST status=%d body=%s", authorizeResponse.StatusCode, body)
	}
	callback, _ := url.Parse(authorizeResponse.Header.Get("Location"))
	authorizeResponse.Body.Close()
	code := callback.Query().Get("code")
	if callback.Query().Get("state") != "state-value" || code == "" {
		t.Fatalf("callback = %v", callback)
	}
	tokenForm := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {client.ClientID}, "redirect_uri": {"http://localhost:4321/callback"}, "code_verifier": {verifier}, "resource": {config.MCPResourceURL}}
	tokenResponse, err := server.client.Post(server.server.URL+"/oauth/token", "application/x-www-form-urlencoded", strings.NewReader(tokenForm.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	var tokenResult map[string]any
	if err := json.NewDecoder(tokenResponse.Body).Decode(&tokenResult); err != nil {
		t.Fatal(err)
	}
	tokenResponse.Body.Close()
	if tokenResponse.StatusCode != 200 || tokenResult["token_type"] != "Bearer" || tokenResult["access_token"] == nil {
		t.Fatalf("token status=%d body=%#v", tokenResponse.StatusCode, tokenResult)
	}
}

func TestMCPUserFromRequestRequiresResourceBoundSignedToken(t *testing.T) {
	testApp, user, _ := newNotificationTestApp(t)
	defer testApp.Cleanup()
	config := Config{PublicURL: "https://tasks.example", MCPResourceURL: "https://tasks.example/mcp", MCPOAuthSigningKey: "0123456789abcdef0123456789abcdef"}
	makeToken := func(audience string) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, mcpOAuthClaims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer: config.PublicURL, Subject: user.Id, Audience: jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		}})
		value, err := token.SignedString([]byte(config.MCPOAuthSigningKey))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}

	request := httptest.NewRequest("GET", "/mcp", nil)
	request.Header.Set("Authorization", "Bearer "+makeToken(config.MCPResourceURL))
	got, err := mcpUserFromRequest(testApp, request, config)
	if err != nil || got.Id != user.Id {
		t.Fatalf("valid token user = %v, err = %v", got, err)
	}

	request.Header.Set("Authorization", "Bearer "+makeToken("https://other.example/mcp"))
	if _, err := mcpUserFromRequest(testApp, request, config); err == nil {
		t.Fatal("token for another resource was accepted")
	}
}

func TestMCPProtectedResourceMetadata(t *testing.T) {
	metadata := mcpProtectedResourceMetadata(Config{PublicURL: "https://tasks.example"})
	if metadata["resource"] != "https://tasks.example/mcp" || metadata["authorization_servers"].([]string)[0] != "https://tasks.example" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
