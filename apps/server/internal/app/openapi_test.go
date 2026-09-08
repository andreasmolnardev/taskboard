package app

import "testing"

func TestDefaultConfiguration(t *testing.T) {
	config := LoadConfig()
	if config.HTTPAddress == "" || config.PublicURL == "" {
		t.Fatal("default server configuration must be complete")
	}
}
