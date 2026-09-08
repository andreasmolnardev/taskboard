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
