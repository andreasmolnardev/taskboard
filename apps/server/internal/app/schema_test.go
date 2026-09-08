package app

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestEnsureCollectionsAddsAdminFlagToUsers(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	if err := EnsureCollections(testApp); err != nil {
		t.Fatal(err)
	}

	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.Fields.GetByName("isAdmin") == nil {
		t.Fatal("users collection must include isAdmin")
	}
}
