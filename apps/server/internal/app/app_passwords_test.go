package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestAuthenticateAppPasswordAcceptsValidCredential(t *testing.T) {
	testApp, user, secret := newAppPasswordTestApp(t)
	defer testApp.Cleanup()
	resetAppPasswordLimiter(t, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))

	got, err := AuthenticateAppPassword(testApp, user.GetString("email"), secret)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Id != user.Id {
		t.Fatalf("authenticated user = %#v", got)
	}
}

func TestAuthenticateAppPasswordThrottlesByUserAndIP(t *testing.T) {
	testApp, user, secret := newAppPasswordTestApp(t)
	defer testApp.Cleanup()
	clock := resetAppPasswordLimiter(t, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))

	for count := 0; count < appPasswordFailureLimit; count++ {
		if _, err := AuthenticateAppPassword(testApp, user.GetString("email"), "wrong", "192.0.2.10"); err == nil {
			t.Fatal("invalid credential was accepted")
		}
	}
	if _, err := AuthenticateAppPassword(testApp, user.GetString("email"), secret, "192.0.2.10"); err == nil {
		t.Fatal("user was not locked out after repeated failures")
	}

	*clock = clock.Add(appPasswordInitialBlock)
	if got, err := AuthenticateAppPassword(testApp, user.GetString("email"), secret, "192.0.2.10"); err != nil || got == nil {
		t.Fatalf("valid credential after lockout = %#v, %v", got, err)
	}

	unknown := "unknown@example.com"
	for count := 0; count < appPasswordFailureLimit; count++ {
		AuthenticateAppPassword(testApp, unknown, "wrong", "192.0.2.11")
	}
	if _, err := AuthenticateAppPassword(testApp, user.GetString("email"), secret, "192.0.2.11"); err == nil {
		t.Fatal("IP was not locked out after repeated failures")
	}
	if got, err := AuthenticateAppPassword(testApp, user.GetString("email"), secret, "192.0.2.12"); err != nil || got == nil {
		t.Fatalf("valid credential from another IP = %#v, %v", got, err)
	}
}

func TestAuthenticateAppPasswordSuccessResetsFailures(t *testing.T) {
	testApp, user, secret := newAppPasswordTestApp(t)
	defer testApp.Cleanup()
	resetAppPasswordLimiter(t, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))

	for count := 0; count < appPasswordFailureLimit-1; count++ {
		AuthenticateAppPassword(testApp, user.GetString("email"), "wrong", "192.0.2.20")
	}
	if _, err := AuthenticateAppPassword(testApp, user.GetString("email"), secret, "192.0.2.20"); err != nil {
		t.Fatal(err)
	}

	for count := 0; count < appPasswordFailureLimit-1; count++ {
		if _, err := AuthenticateAppPassword(testApp, user.GetString("email"), "wrong", "192.0.2.20"); err == nil {
			t.Fatal("invalid credential was accepted")
		}
	}
	if _, err := AuthenticateAppPassword(testApp, user.GetString("email"), secret, "192.0.2.20"); err != nil {
		t.Fatalf("valid credential after reset = %v", err)
	}
}

func TestAuthenticateAppPasswordDoesNotLeakSecrets(t *testing.T) {
	testApp, user, secret := newAppPasswordTestApp(t)
	defer testApp.Cleanup()
	resetAppPasswordLimiter(t, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))

	for _, attempt := range []struct {
		username string
		password string
	}{
		{user.GetString("email"), "wrong"},
		{"missing@example.com", secret},
	} {
		_, err := AuthenticateAppPassword(testApp, attempt.username, attempt.password)
		if err == nil || err.Error() != "invalid credentials" {
			t.Fatalf("error = %v", err)
		}
		if strings.Contains(err.Error(), attempt.password) || strings.Contains(err.Error(), attempt.username) {
			t.Fatalf("error leaked authentication data: %v", err)
		}
	}
	key := appPasswordThrottleKey("username", secret)
	if strings.Contains(key, secret) {
		t.Fatal("throttle key contains the secret")
	}
}

func TestAppPasswordThrottleIsBounded(t *testing.T) {
	limiter := newAppPasswordThrottle(func() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC) })
	for count := 0; count < appPasswordMaxEntries+100; count++ {
		limiter.recordFailure(appPasswordThrottleKey("username", strconv.Itoa(count)))
	}
	if got := len(limiter.entries); got != appPasswordMaxEntries {
		t.Fatalf("throttle entries = %d, want %d", got, appPasswordMaxEntries)
	}
}

func TestAppPasswordThrottleKeyIsSHA256(t *testing.T) {
	value := "user@example.com"
	digest := sha256.Sum256([]byte(value))
	want := "username:" + hex.EncodeToString(digest[:])
	if got := appPasswordThrottleKey("username", value); got != want {
		t.Fatalf("throttle key = %q, want %q", got, want)
	}
}

func newAppPasswordTestApp(t *testing.T) (*tests.TestApp, *core.Record, string) {
	t.Helper()
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureCollections(testApp); err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("app-password@example.com")
	user.SetPassword("password123456")
	if err := testApp.Save(user); err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}

	secret := "app-secret-123"
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	if err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	passwords, err := testApp.FindCollectionByNameOrId(appPasswordCollection)
	if err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	record := core.NewRecord(passwords)
	record.Set("user", user.Id)
	record.Set("name", "Test")
	record.Set("secret_hash", string(hash))
	if err := testApp.Save(record); err != nil {
		testApp.Cleanup()
		t.Fatal(err)
	}
	return testApp, user, secret
}

func resetAppPasswordLimiter(t *testing.T, start time.Time) *time.Time {
	t.Helper()
	clock := start
	previous := appPasswordLimiter
	appPasswordLimiter = newAppPasswordThrottle(func() time.Time { return clock })
	t.Cleanup(func() { appPasswordLimiter = previous })
	return &clock
}
