package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/andreasmolnardev/slopstack/go/backup"
	"github.com/pocketbase/pocketbase/tests"
)

func TestTaskboardBackupCommandUsesSharedRuntimeAndKeepsAppCommands(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	command := NewBackupCommand(testApp)
	for _, name := range []string{"create", "json", "key-init", "key-import", "list", "restore"} {
		if _, _, err := command.Find([]string{name}); err != nil {
			t.Fatalf("backup command %q is missing: %v", name, err)
		}
	}

	create, _, err := command.Find([]string{"create"})
	if err != nil {
		t.Fatal(err)
	}
	if create.Flags().Lookup("encrypt") != nil {
		t.Fatal("Taskboard create must remain encrypted by default without a plaintext mode")
	}

	json, _, err := command.Find([]string{"json"})
	if err != nil || json.Use != "json" {
		t.Fatalf("Taskboard JSON export command = %v, %v", json, err)
	}
}

func TestTaskboardBackupKeyImportReadsStandardInput(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	command := NewBackupCommand(testApp)
	key, err := backup.CreateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	command.SetIn(strings.NewReader(key + "\n"))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"key-import"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Recovery key installed.") {
		t.Fatalf("key-import output = %q", output.String())
	}
	if _, err := backup.LoadKey(testApp.DataDir()); err != nil {
		t.Fatalf("imported key was not installed in app data dir: %v", err)
	}
}

func TestTaskboardBackupRestoreHonorsPrimaryAndLegacyRestartGuards(t *testing.T) {
	for _, test := range []struct {
		name       string
		primaryEnv string
		legacyEnv  string
	}{
		{name: "primary", primaryEnv: "1"},
		{name: "legacy", legacyEnv: "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(backupRestoreRestartedEnv, test.primaryEnv)
			t.Setenv("SLOPSTACK_BACKUP_RESTORE_RESTARTED", test.legacyEnv)
			testApp, err := tests.NewTestApp()
			if err != nil {
				t.Fatal(err)
			}
			defer testApp.Cleanup()

			command := NewBackupCommand(testApp)
			command.SetArgs([]string{"restore", "safe.zip.enc", "--confirm"})
			if err := command.Execute(); err != nil {
				t.Fatalf("restore command should skip after the process restart: %v", err)
			}
		})
	}
}
