package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// setupInitTestMocks creates mock components specifically for testing the init command
func setupInitTestMocks(t *testing.T) (*config.MockConfigHandler, *shell.MockShell, *blueprint.MockBlueprintHandler) {
	t.Helper()

	// Create mock components
	mockConfigHandler := &config.MockConfigHandler{}
	mockShell := &shell.MockShell{}
	mockBlueprintHandler := &blueprint.MockBlueprintHandler{}

	// Set up mock behaviors
	mockShell.InitializeFunc = func() error { return nil }
	mockShell.AddCurrentDirToTrustedFileFunc = func() error { return nil }
	mockShell.WriteResetTokenFunc = func() (string, error) { return "reset-token", nil }
	mockShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }

	mockConfigHandler.InitializeFunc = func() error { return nil }
	mockConfigHandler.SetContextFunc = func(string) error { return nil }
	mockConfigHandler.GetContextFunc = func() string { return "local" }
	mockConfigHandler.SetDefaultFunc = func(v1alpha1.Context) error { return nil }
	mockConfigHandler.SetContextValueFunc = func(string, any) error { return nil }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(string, ...bool) error { return nil }

	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.ProcessContextTemplatesFunc = func(contextName string, reset ...bool) error { return nil }
	mockBlueprintHandler.LoadConfigFunc = func(...bool) error { return nil }

	return mockConfigHandler, mockShell, mockBlueprintHandler
}

func TestInitCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given mock components
		mockConfigHandler, mockShell, mockBlueprintHandler := setupInitTestMocks(t)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		originalStderr := os.Stderr
		os.Stderr = w

		// Create injector and register mocks
		injector := di.NewInjector()
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("shell", mockShell)
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// Override the injector in the command context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = originalStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		expectedMessage := "Initialization successful\n"
		if buf.String() != expectedMessage {
			t.Errorf("Expected message %q, got %q", expectedMessage, buf.String())
		}
	})

	t.Run("WithContext", func(t *testing.T) {
		// Given mock components
		mockConfigHandler, mockShell, mockBlueprintHandler := setupInitTestMocks(t)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		originalStderr := os.Stderr
		os.Stderr = w

		// Create injector and register mocks
		injector := di.NewInjector()
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("shell", mockShell)
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Set up command arguments
		rootCmd.SetArgs([]string{"init", "test-context"})

		// Override the injector in the command context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = originalStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		expectedMessage := "Initialization successful\n"
		if buf.String() != expectedMessage {
			t.Errorf("Expected message %q, got %q", expectedMessage, buf.String())
		}
	})

	t.Run("WithFlags", func(t *testing.T) {
		// Given mock components
		mockConfigHandler, mockShell, mockBlueprintHandler := setupInitTestMocks(t)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		originalStderr := os.Stderr
		os.Stderr = w

		// Create injector and register mocks
		injector := di.NewInjector()
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("shell", mockShell)
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Set up command arguments with flags
		rootCmd.SetArgs([]string{"init", "--aws", "--blueprint", "custom-blueprint", "--terraform=false"})

		// Override the injector in the command context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = originalStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		expectedMessage := "Initialization successful\n"
		if buf.String() != expectedMessage {
			t.Errorf("Expected message %q, got %q", expectedMessage, buf.String())
		}
	})

	t.Run("WithSetFlag", func(t *testing.T) {
		// Given mock components
		mockConfigHandler, mockShell, mockBlueprintHandler := setupInitTestMocks(t)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		originalStderr := os.Stderr
		os.Stderr = w

		// Create injector and register mocks
		injector := di.NewInjector()
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("shell", mockShell)
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Set up command arguments with --set flag
		rootCmd.SetArgs([]string{"init", "--set", "custom.key=custom-value"})

		// Override the injector in the command context
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		// When executing the command
		err := rootCmd.Execute()

		// Close the writer and restore os.Stderr
		w.Close()
		os.Stderr = originalStderr

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		expectedMessage := "Initialization successful\n"
		if buf.String() != expectedMessage {
			t.Errorf("Expected message %q, got %q", expectedMessage, buf.String())
		}
	})
}

type platformTest struct {
	name           string
	flags          []string
	enabledKey     string
	enabledValue   bool
	driverKey      string
	driverExpected string
}

func TestInitCmd_PlatformFlags(t *testing.T) {
	platforms := []platformTest{
		{
			name:           "aws",
			flags:          []string{"--aws"},
			enabledKey:     "aws.enabled",
			enabledValue:   true,
			driverKey:      "cluster.driver",
			driverExpected: "eks",
		},
		{
			name:           "azure",
			flags:          []string{"--azure"},
			enabledKey:     "azure.enabled",
			enabledValue:   true,
			driverKey:      "cluster.driver",
			driverExpected: "aks",
		},
		{
			name:           "talos",
			flags:          []string{"--talos"},
			enabledKey:     "",
			enabledValue:   false,
			driverKey:      "cluster.driver",
			driverExpected: "talos",
		},
		{
			name:           "aws_with_talos_override",
			flags:          []string{"--aws", "--talos"},
			enabledKey:     "aws.enabled",
			enabledValue:   true,
			driverKey:      "cluster.driver",
			driverExpected: "talos", // talos overrides eks
		},
	}

	for _, tc := range platforms {
		t.Run(tc.name, func(t *testing.T) {
			// Given mock components with a store to track values
			store := make(map[string]any)
			mockConfigHandler, mockShell, mockBlueprintHandler := setupInitTestMocks(t)

			// Override SetContextValueFunc to store values
			mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
				store[key] = value
				return nil
			}

			// Create injector and register mocks
			injector := di.NewInjector()
			injector.Register("configHandler", mockConfigHandler)
			injector.Register("shell", mockShell)
			injector.Register("blueprintHandler", mockBlueprintHandler)

			// Reset the root command and init command flags
			rootCmd.ResetFlags()
			initCmd.ResetFlags()

			// Re-add the init command flags
			initCmd.Flags().StringVar(&initBlueprint, "blueprint", "windsorcli/core", "Specify the blueprint to use")
			initCmd.Flags().BoolVar(&initTerraform, "terraform", true, "Enable Terraform")
			initCmd.Flags().BoolVar(&initK8s, "k8s", true, "Enable Kubernetes")
			initCmd.Flags().BoolVar(&initColima, "colima", false, "Use Colima as VM driver")
			initCmd.Flags().BoolVar(&initAws, "aws", false, "Enable AWS platform")
			initCmd.Flags().BoolVar(&initAzure, "azure", false, "Enable Azure platform")
			initCmd.Flags().BoolVar(&initDockerCompose, "docker-compose", true, "Enable Docker Compose")
			initCmd.Flags().BoolVar(&initTalos, "talos", false, "Enable Talos")
			initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override configuration values")

			// Set up command arguments
			args := append([]string{"init"}, tc.flags...)
			rootCmd.SetArgs(args)

			// Override the injector in the command context
			ctx := context.WithValue(context.Background(), injectorKey, injector)
			rootCmd.SetContext(ctx)

			// When executing the command
			err := rootCmd.Execute()

			// Then no error should occur
			if err != nil {
				t.Fatalf("Expected success, got error: %v", err)
			}

			// And the expected configuration should be set
			if tc.enabledKey != "" {
				if value, exists := store[tc.enabledKey]; !exists || value != tc.enabledValue {
					t.Errorf("Expected %s to be %v, got %v", tc.enabledKey, tc.enabledValue, value)
				}
			}

			if value, exists := store[tc.driverKey]; !exists || value != tc.driverExpected {
				t.Errorf("Expected %s to be %q, got %v", tc.driverKey, tc.driverExpected, value)
			}
		})
	}
}
