package helpers

import (
	"errors"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestColimaHelper_GetEnvVars(t *testing.T) {
	configHandler := &config.MockConfigHandler{}
	shell := &shell.MockShell{}
	ctx := &context.MockContext{}

	helper := NewColimaHelper(configHandler, shell, ctx)

	envVars, err := helper.GetEnvVars()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(envVars) != 0 {
		t.Fatalf("expected empty envVars, got %v", envVars)
	}
}

func TestColimaHelper_PostEnvExec(t *testing.T) {
	configHandler := &config.MockConfigHandler{}
	shell := &shell.MockShell{}
	ctx := &context.MockContext{}

	helper := NewColimaHelper(configHandler, shell, ctx)

	err := helper.PostEnvExec()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestColimaHelper_SetConfig(t *testing.T) {
	t.Run("SuccessfulSetConfig", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				if key != "contexts.test-context.vm.driver" || value != "colima" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("driver", "colima")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "", errors.New("context error")
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving context: context error" {
			t.Fatalf("expected context error, got %v", err)
		}
	})

	t.Run("ErrorSettingConfigValue", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("set config error")
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: set config error" {
			t.Fatalf("expected set config error, got %v", err)
		}
	})

	t.Run("UnsupportedConfigKey", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("unsupported", "value")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "unsupported config key: unsupported" {
			t.Fatalf("expected unsupported config key error, got %v", err)
		}
	})

	t.Run("ArchConversion", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		NewColimaHelper(configHandler, shell, ctx)

		// Override the getArchWrapper function for testing
		originalGetArchWrapper := getArchWrapper
		defer func() { getArchWrapper = originalGetArchWrapper }()

		tests := []struct {
			mockArch string
			expected string
		}{
			{"amd64", "x86_64"},
			{"arm64", "aarch64"},
			{"unknown", "unknown"}, // Default case
		}

		for _, tt := range tests {
			getArchWrapper = func() string { return tt.mockArch }
			_, _, _, _, arch := getDefaultValues("test-context")
			if arch != tt.expected {
				t.Fatalf("expected arch to be %v, got %v", tt.expected, arch)
			}
		}
	})
}
