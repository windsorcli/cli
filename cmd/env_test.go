package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/controller"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupEnvMocks(t *testing.T) (*Mocks, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	injector := di.NewInjector()
	mocks := &Mocks{
		Injector:        injector,
		ConfigHandler:   config.NewMockConfigHandler(),
		Controller:      controller.NewMockController(),
		Shell:           shell.NewMockShell(),
		SecretsProvider: secrets.NewMockSecretsProvider(injector),
		EnvPrinter:      env.NewMockEnvPrinter(),
		Shims:           &Shims{},
	}

	// Set up mock shell functions
	mockShell := mocks.Shell
	mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
	mockShell.CheckResetFlagsFunc = func() (bool, error) { return false, nil }
	mockShell.ResetFunc = func() {}
	mockShell.GetProjectRootFunc = func() (string, error) { return "/test/dir", nil }

	// Create and register mock env printers
	mockEnvPrinter := env.NewMockEnvPrinter()
	mockEnvPrinter.PrintFunc = func() error {
		return nil
	}
	mockEnvPrinter.PostEnvHookFunc = func() error {
		return nil
	}
	injector.Register("envPrinter", mockEnvPrinter)

	mockWindsorEnvPrinter := env.NewMockEnvPrinter()
	mockWindsorEnvPrinter.PrintFunc = func() error {
		return nil
	}
	mockWindsorEnvPrinter.PostEnvHookFunc = func() error {
		return nil
	}
	injector.Register("windsorEnvPrinter", mockWindsorEnvPrinter)

	mockDockerEnvPrinter := env.NewMockEnvPrinter()
	mockDockerEnvPrinter.PrintFunc = func() error {
		return nil
	}
	mockDockerEnvPrinter.PostEnvHookFunc = func() error {
		return nil
	}
	injector.Register("dockerEnvPrinter", mockDockerEnvPrinter)

	// Set up mock controller to return all env printers
	mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
		return []env.EnvPrinter{mockEnvPrinter, mockWindsorEnvPrinter, mockDockerEnvPrinter}
	}

	// Set up output buffers
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	return mocks, stdout, stderr
}

func TestEnvCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, stderr := setupEnvMocks(t)

		rootCmd.SetArgs([]string{"env"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("NotTrustedDirectory", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, stderr := setupEnvMocks(t)

		// Override shell to return error for CheckTrustedDirectory
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mocks.Shell.ResetFunc = func() {}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		// Set up env printer
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		// Set up command output
		rootCmd.SetArgs([]string{"env"})
		rootCmd.SetErr(stderr)

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain warning
		expectedWarning := "\033[33mWarning: You are not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve.\033[0m\n"
		if stderr.String() != expectedWarning {
			t.Errorf("Expected warning %q, got %q", expectedWarning, stderr.String())
		}
	})

	t.Run("NotTrustedDirectoryWithHook", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, stderr := setupEnvMocks(t)

		// Override shell to return not trusted error
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}
		mocks.Shell.ResetFunc = func() {}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		rootCmd.SetArgs([]string{"env", "--hook"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ResetRequired", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to require reset
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}
		mocks.Shell.ResetFunc = func() {}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		// Set up env printer
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("DecryptSecrets", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to be trusted
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mocks.Shell.ResetFunc = func() {}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		// Set up env printer
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env", "--decrypt"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("DecryptSecretsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to be trusted
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mocks.Shell.ResetFunc = func() {}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		// Override secrets provider to return error
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets error")
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		// Set up env printer
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env", "--decrypt"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur (errors are suppressed in non-verbose mode)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("DecryptSecretsErrorVerbose", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mocks.Shell.ResetFunc = func() {}

		// Override secrets provider to return error
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets error")
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		// Set up env printer
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env", "--decrypt", "--verbose"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur in verbose mode
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain secrets error message
		expectedError := "Error loading secrets provider: secrets error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PrintError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to be trusted
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mockShell.ResetFunc = func() {}
		mocks.Shell = mockShell
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// Override env printer to return error
		mocks.EnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env", "--verbose"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur in verbose mode
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain print error message
		expectedError := "Error executing Print: print error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to be trusted
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mockShell.ResetFunc = func() {}
		mocks.Shell = mockShell
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// Override env printer to return error
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post hook error")
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env", "--verbose"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur in verbose mode
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain post hook error message
		expectedError := "Error executing PostEnvHook: post hook error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoEnvPrinters", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to be trusted
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mockShell.ResetFunc = func() {}
		mocks.Shell = mockShell
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// Override controller to return no env printers
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return nil
		}

		rootCmd.SetArgs([]string{"env", "--verbose"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur in verbose mode
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain no printers message
		expectedError := "Error resolving environment printers: no printers returned"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("InitializationError", func(t *testing.T) {
		// Given a set of mocks with initialization error
		mocks, _, _ := setupEnvMocks(t)

		// Mock controller to return initialization error
		mocks.Controller.InitializeWithRequirementsFunc = func(req ctrl.Requirements) error {
			return fmt.Errorf("initialization failed")
		}

		rootCmd.SetArgs([]string{"env"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain initialization message
		expectedError := "Error initializing: initialization failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("CheckResetFlagsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to return error checking reset flags
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("reset flags error")
		}
		mockShell.ResetFunc = func() {}
		mocks.Shell = mockShell
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// Set up mock controller to handle initialization
		mocks.Controller.InitializeWithRequirementsFunc = func(req ctrl.Requirements) error {
			return nil
		}

		// Set up mock env printer
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env", "--verbose"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur in verbose mode
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain reset flags error message
		expectedError := "Error checking reset signal: reset flags error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetenvError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setupEnvMocks(t)

		// Override shell to require reset
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil // This will trigger reset
		}
		mocks.Shell.ResetFunc = func() {}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		// Set up shims to return error for Setenv and value for Getenv
		mocks.Shims.Setenv = func(key, value string) error {
			if key == "NO_CACHE" {
				return fmt.Errorf("setenv error")
			}
			return nil
		}
		mocks.Shims.Getenv = func(key string) string {
			return "" // No session token, will also trigger reset
		}

		// Set up mock controller initialization
		mocks.Controller.InitializeWithRequirementsFunc = func(req ctrl.Requirements) error {
			if req.Flags["verbose"] {
				return fmt.Errorf("Error setting NO_CACHE: setenv error")
			}
			return nil
		}

		// Set up mock env printer
		mocks.EnvPrinter.PrintFunc = func() error {
			return nil
		}
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter}
		}

		rootCmd.SetArgs([]string{"env", "--verbose"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "Error setting NO_CACHE: setenv error") {
			t.Errorf("Expected error to contain 'Error setting NO_CACHE: setenv error', got %v", err)
		}
	})
}
