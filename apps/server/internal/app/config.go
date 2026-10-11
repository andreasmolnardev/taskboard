package app

import (
	"fmt"
	"strings"

	sharedconfig "github.com/andreasmolnardev/slopstack/go/config"
	"github.com/pocketbase/pocketbase/tools/cron"
)

type RegistrationMode string

const (
	RegistrationDisabled RegistrationMode = "disabled"
	RegistrationApproval RegistrationMode = "approval"
	RegistrationOTP      RegistrationMode = "otp"
)

type Config struct {
	HTTPAddress       string
	PublicURL         string
	SecureCookies     bool
	RegistrationMode  RegistrationMode
	LogLevel          string
	StoragePath       string
	Development       bool
	BackupEnabled     bool
	BackupJSONEnabled bool
	BackupCron        string
	BackupMaxKeep     int
	BackupS3          BackupS3Config
}

type BackupS3Config struct {
	Enabled        bool
	Bucket         string
	Region         string
	Endpoint       string
	AccessKey      string
	Secret         string
	ForcePathStyle bool
}

func LoadConfig() Config {
	return Config{
		HTTPAddress:       env("TASKBOARD_HTTP_ADDRESS", "0.0.0.0:8090"),
		PublicURL:         env("TASKBOARD_PUBLIC_URL", "http://localhost:8090"),
		SecureCookies:     envBool("TASKBOARD_SECURE_COOKIES", false),
		RegistrationMode:  registrationMode(),
		LogLevel:          env("TASKBOARD_LOG_LEVEL", "info"),
		StoragePath:       env("TASKBOARD_STORAGE_PATH", "pb_data"),
		Development:       envBool("TASKBOARD_DEVELOPMENT", false),
		BackupEnabled:     envBool("TASKBOARD_BACKUP_ENABLED", false),
		BackupJSONEnabled: envBool("TASKBOARD_BACKUP_JSON_ENABLED", false),
		BackupCron:        env("TASKBOARD_BACKUP_CRON", "0 3 * * *"),
		BackupMaxKeep:     envInt("TASKBOARD_BACKUP_MAX_KEEP", 14),
		BackupS3: BackupS3Config{
			Enabled:        envBool("TASKBOARD_BACKUP_S3_ENABLED", false),
			Bucket:         env("TASKBOARD_BACKUP_S3_BUCKET", ""),
			Region:         env("TASKBOARD_BACKUP_S3_REGION", ""),
			Endpoint:       env("TASKBOARD_BACKUP_S3_ENDPOINT", ""),
			AccessKey:      env("TASKBOARD_BACKUP_S3_ACCESS_KEY", ""),
			Secret:         env("TASKBOARD_BACKUP_S3_SECRET", ""),
			ForcePathStyle: envBool("TASKBOARD_BACKUP_S3_FORCE_PATH_STYLE", false),
		},
	}
}

func validateBackupSchedule(expression string, maxKeep int) error {
	if maxKeep < 1 {
		return fmt.Errorf("invalid backup configuration: retention must be at least 1")
	}
	if err := cron.New().Add("backup-validation", expression, func() {}); err != nil {
		return fmt.Errorf("invalid backup configuration: %w", err)
	}
	return nil
}

func registrationMode() RegistrationMode {
	if !envBool("TASKBOARD_REGISTRATION_ENABLED", true) {
		return RegistrationDisabled
	}
	switch RegistrationMode(env("TASKBOARD_REGISTRATION_MODE", string(RegistrationApproval))) {
	case RegistrationDisabled, RegistrationOTP:
		return RegistrationMode(env("TASKBOARD_REGISTRATION_MODE", string(RegistrationApproval)))
	default:
		return RegistrationApproval
	}
}

func env(key, fallback string) string {
	return sharedconfig.Lookup(key, legacyEnvKey(key), fallback)
}

func legacyEnvKey(key string) string {
	if strings.HasPrefix(key, "TASKBOARD_") {
		return strings.Replace(key, "TASKBOARD_", "SLOPSTACK_", 1)
	}
	return ""
}

func lookupEnv(key string) (string, bool) {
	return sharedconfig.LookupValue(key, legacyEnvKey(key))
}

func envBool(key string, fallback bool) bool {
	return sharedconfig.Bool(key, legacyEnvKey(key), fallback)
}

func envInt(key string, fallback int) int {
	return sharedconfig.Int(key, legacyEnvKey(key), fallback)
}
