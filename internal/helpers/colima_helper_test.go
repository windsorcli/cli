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
	shellInstance := &shell.MockShell{}
	ctx := &context.MockContext{}

	helper := NewColimaHelper(configHandler, shellInstance, ctx)

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
	shellInstance := &shell.MockShell{}
	ctx := &context.MockContext{}

	helper := NewColimaHelper(configHandler, shellInstance, ctx)

	err := helper.PostEnvExec()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestColimaHelper_SetConfig(t *testing.T) {
	t.Run("successful set config", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				if key != "contexts.test-context.vm.driver" || value != "colima" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			},
		}
		shellInstance := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shellInstance, ctx)

		err := helper.SetConfig("vm_driver", "colima")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("error retrieving context", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shellInstance := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "", errors.New("context error")
			},
		}

		helper := NewColimaHelper(configHandler, shellInstance, ctx)

		err := helper.SetConfig("vm_driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving context: context error" {
			t.Fatalf("expected context error, got %v", err)
		}
	})

	t.Run("error setting config value", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("set config error")
			},
		}
		shellInstance := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shellInstance, ctx)

		err := helper.SetConfig("vm_driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: set config error" {
			t.Fatalf("expected set config error, got %v", err)
		}
	})

	t.Run("unsupported config key", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shellInstance := &shell.MockShell{}
		ctx := &context.MockContext{}

		helper := NewColimaHelper(configHandler, shellInstance, ctx)

		err := helper.SetConfig("unsupported", "value")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
