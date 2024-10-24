package helpers

import (
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
)

func TestDNSHelper_NewDNSHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create DI container without registering cliConfigHandler
		diContainer := di.NewContainer()

		// Attempt to create DNSHelper
		_, err := NewDNSHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		// Create DI container and register cliConfigHandler
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Attempt to create DNSHelper
		helper, err := NewDNSHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// Ensure the helper is not nil
		if helper == nil {
			t.Fatalf("expected helper to be non-nil")
		}
	})
}

func TestDNSHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and helper
		mockConfigHandler := config.NewMockConfigHandler()
		helper := &DNSHelper{ConfigHandler: mockConfigHandler}

		// When: Initialize is called
		err := helper.Initialize()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
	})
}

func TestDNSHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and helper
		mockConfigHandler := config.NewMockConfigHandler()
		helper := &DNSHelper{ConfigHandler: mockConfigHandler}

		// When: GetEnvVars is called
		envVars, err := helper.GetEnvVars()

		// Then: no error should be returned and envVars should be nil
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}
		if envVars != nil {
			t.Errorf("expected envVars to be nil, got %v", envVars)
		}
	})
}

func TestDNSHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and helper
		mockConfigHandler := config.NewMockConfigHandler()
		helper := &DNSHelper{ConfigHandler: mockConfigHandler}

		// When: PostEnvExec is called
		err := helper.PostEnvExec()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("PostEnvExec() error = %v", err)
		}
	})
}

func TestDNSHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and helper
		mockConfigHandler := config.NewMockConfigHandler()
		helper := &DNSHelper{ConfigHandler: mockConfigHandler}

		// When: GetComposeConfig is called
		config, err := helper.GetComposeConfig()

		// Then: no error should be returned and config should be nil
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if config != nil {
			t.Errorf("expected config to be nil, got %v", config)
		}
	})
}

func TestDNSHelper_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and helper
		mockConfigHandler := config.NewMockConfigHandler()
		helper := &DNSHelper{ConfigHandler: mockConfigHandler}

		// When: WriteConfig is called
		err := helper.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})
}
