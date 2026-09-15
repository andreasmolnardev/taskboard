package app

import (
	"os"
	"strconv"
)

type RegistrationMode string

const (
	RegistrationDisabled RegistrationMode = "disabled"
	RegistrationApproval RegistrationMode = "approval"
	RegistrationOTP      RegistrationMode = "otp"
)

type Config struct {
	HTTPAddress      string
	PublicURL        string
	SecureCookies    bool
	RegistrationMode RegistrationMode
	LogLevel         string
	StoragePath      string
	Development      bool
	BackupEnabled    bool
	BackupCron       string
	BackupMaxKeep    int
	BackupS3         BackupS3Config
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
		HTTPAddress:      env("SLOPSTACK_HTTP_ADDRESS", "0.0.0.0:8090"),
		PublicURL:        env("SLOPSTACK_PUBLIC_URL", "http://localhost:8090"),
		SecureCookies:    envBool("SLOPSTACK_SECURE_COOKIES", false),
		RegistrationMode: registrationMode(),
		LogLevel:         env("SLOPSTACK_LOG_LEVEL", "info"),
		StoragePath:      env("SLOPSTACK_STORAGE_PATH", "pb_data"),
		Development:      envBool("SLOPSTACK_DEVELOPMENT", false),
		BackupEnabled:    envBool("SLOPSTACK_BACKUP_ENABLED", false),
		BackupCron:       env("SLOPSTACK_BACKUP_CRON", "0 3 * * *"),
		BackupMaxKeep:    envInt("SLOPSTACK_BACKUP_MAX_KEEP", 14),
		BackupS3: BackupS3Config{
			Enabled:        envBool("SLOPSTACK_BACKUP_S3_ENABLED", false),
			Bucket:         env("SLOPSTACK_BACKUP_S3_BUCKET", ""),
			Region:         env("SLOPSTACK_BACKUP_S3_REGION", ""),
			Endpoint:       env("SLOPSTACK_BACKUP_S3_ENDPOINT", ""),
			AccessKey:      env("SLOPSTACK_BACKUP_S3_ACCESS_KEY", ""),
			Secret:         env("SLOPSTACK_BACKUP_S3_SECRET", ""),
			ForcePathStyle: envBool("SLOPSTACK_BACKUP_S3_FORCE_PATH_STYLE", false),
		},
	}
}

func registrationMode() RegistrationMode {
	if !envBool("SLOPSTACK_REGISTRATION_ENABLED", true) {
		return RegistrationDisabled
	}
	switch RegistrationMode(env("SLOPSTACK_REGISTRATION_MODE", string(RegistrationApproval))) {
	case RegistrationDisabled, RegistrationOTP:
		return RegistrationMode(env("SLOPSTACK_REGISTRATION_MODE", string(RegistrationApproval)))
	default:
		return RegistrationApproval
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
