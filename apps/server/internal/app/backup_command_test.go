package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidateBackupName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"manual_20260913_120000.zip", true},
		{"pb_backup_taskboard_ab12cd.zip", true},
		{"@auto_pb_backup_taskboard_ab12cd.zip", true},
		{"../data.db", false},
		{"folder/backup.zip", false},
		{`folder\\backup.zip`, false},
		{"backup.ZIP", false},
		{"backup zip.zip", false},
		{".zip", false},
		{"", false},
		{strings.Repeat("a", 147) + ".zip", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateBackupName(test.name)
			if test.valid && err != nil {
				t.Fatalf("expected valid name: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected invalid name")
			}
		})
	}
}

func TestBackupRestoreRequiresConfirmation(t *testing.T) {
	restored := false
	command := (&BackupCommand{
		Restore: func(context.Context, string) error {
			restored = true
			return nil
		},
	}).restoreCommand()
	command.SetArgs([]string{"safe.zip"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if restored {
		t.Fatal("restore ran without confirmation")
	}
}

func TestBackupRestoreSkipsCommandAfterRestart(t *testing.T) {
	t.Setenv(backupRestoreRestartedEnv, "1")
	restored := false
	command := (&BackupCommand{
		Restore: func(context.Context, string) error {
			restored = true
			return nil
		},
	}).restoreCommand()
	command.SetArgs([]string{"safe.zip", "--confirm"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if restored {
		t.Fatal("restore ran again after restart")
	}
}

func TestBackupRestoreRejectsUnsafeName(t *testing.T) {
	restored := false
	command := (&BackupCommand{
		Restore: func(context.Context, string) error {
			restored = true
			return nil
		},
	}).restoreCommand()
	command.SetArgs([]string{"../backup.zip", "--confirm"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "unsafe backup name") {
		t.Fatalf("expected unsafe name error, got %v", err)
	}
	if restored {
		t.Fatal("restore ran with an unsafe name")
	}
}

func TestBackupRestoreConfirmed(t *testing.T) {
	var restoredName string
	command := (&BackupCommand{
		Restore: func(_ context.Context, name string) error {
			restoredName = name
			return nil
		},
	}).restoreCommand()
	command.SetArgs([]string{"safe.zip", "--confirm"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if restoredName != "safe.zip" {
		t.Fatalf("restored %q", restoredName)
	}
}

func TestBackupCreatePrintsCreatedName(t *testing.T) {
	command := (&BackupCommand{
		Create: func(context.Context) (string, error) { return "manual_test.zip", nil },
	}).createCommand()
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "manual_test.zip\n" {
		t.Fatalf("unexpected output %q", output.String())
	}
}

func TestBackupListPrintsFiles(t *testing.T) {
	command := (&BackupCommand{
		List: func(context.Context) ([]BackupFile, error) {
			return []BackupFile{{Name: "safe.zip", Size: 42, Modified: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)}}, nil
		},
	}).listCommand()
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"NAME", "safe.zip", "42", "2026-09-13T12:00:00Z"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output %q does not contain %q", output.String(), expected)
		}
	}
}
