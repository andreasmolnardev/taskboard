package app

import "testing"

func TestLoadConfigPrefersTaskboardEnv(t *testing.T) {
	t.Setenv("TASKBOARD_STORAGE_PATH", "taskboard-data")
	t.Setenv("SLOPSTACK_STORAGE_PATH", "legacy-data")

	if got := LoadConfig().StoragePath; got != "taskboard-data" {
		t.Fatalf("StoragePath = %q, want TASKBOARD_STORAGE_PATH value", got)
	}
}

func TestLoadConfigSupportsLegacySlopstackEnv(t *testing.T) {
	t.Setenv("TASKBOARD_STORAGE_PATH", "")
	t.Setenv("SLOPSTACK_STORAGE_PATH", "legacy-data")

	if got := LoadConfig().StoragePath; got != "legacy-data" {
		t.Fatalf("StoragePath = %q, want SLOPSTACK_STORAGE_PATH fallback", got)
	}
}

func TestPreMigrationConfigValidationChecksEnabledBackupSchedule(t *testing.T) {
	if err := validatePreMigrationConfig(Config{BackupEnabled: false, BackupCron: "invalid"}); err != nil {
		t.Fatalf("disabled backups should not validate a schedule: %v", err)
	}
	if err := validatePreMigrationConfig(Config{BackupEnabled: true, BackupCron: "invalid", BackupMaxKeep: 14}); err == nil {
		t.Fatal("enabled backups with an invalid schedule should fail before migrations")
	}
	if err := validatePreMigrationConfig(Config{BackupEnabled: true, BackupCron: "0 3 * * *", BackupMaxKeep: 14}); err != nil {
		t.Fatalf("valid backup schedule should pass pre-migration validation: %v", err)
	}
}
