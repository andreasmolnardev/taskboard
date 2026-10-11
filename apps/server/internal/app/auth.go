package app

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/andreasmolnardev/slopstack/go/auth"
)

// RegistrationAllowed centralizes the registration policy for custom endpoints
// and hooks. PocketBase owns sessions, verification, and password reset flows.
func RegistrationAllowed(config Config) bool {
	return config.RegistrationMode != RegistrationDisabled
}

func RegistrationRequiresApproval(config Config) bool {
	return config.RegistrationMode == RegistrationApproval
}

func RegistrationRequiresOTP(config Config) bool {
	return config.RegistrationMode == RegistrationOTP
}

func CurrentUser(event *core.RequestEvent) (*core.Record, error) {
	return auth.CurrentUser(event)
}
