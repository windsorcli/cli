package env

import (
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// TestEnv_Initialize tests the Initialize method of the Env struct
func TestEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Create and register mock versions of shell and configHandler
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and check for errors
		err := env.Initialize()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register mock version of configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)

		// Register an invalid shell that cannot be cast to shell.Shell
		mockInjector.Register("shell", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting shell to shell.Shell" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingCliConfigHandler", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register mock version of shell
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)

		// Register an invalid configHandler that cannot be cast to config.ConfigHandler
		mockInjector.Register("configHandler", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting configHandler to config.ConfigHandler" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestEnv_GetEnvVars tests the GetEnvVars method of the Env struct
func TestEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()
		env := NewBaseEnvPrinter(mockInjector)
		env.Initialize()

		// Call GetEnvVars and check for errors
		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that the returned envVars is an empty map
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})
}

// TestEnv_Print tests the Print method of the Env struct
func TestEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)
		env := NewBaseEnvPrinter(mockInjector)
		env.Initialize()

		// Mock the PrintEnvVarsFunc to verify it is called
		var capturedEnvVars map[string]string
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := env.Print(map[string]string{"TEST_VAR": "test_value"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{"TEST_VAR": "test_value"}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("NoCustomVars", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)
		env := NewBaseEnvPrinter(mockInjector)
		env.Initialize()

		// Mock the PrintEnvVarsFunc to verify it is called
		var capturedEnvVars map[string]string
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print without custom vars and check for errors
		err := env.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with an empty map
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})
}

type MockUser struct {
	CurrentDir string
	HomeDir    string
}

// NewMockUser creates a new instance of MockUser.
func NewMockUser() *MockUser {
	return &MockUser{}
}

// func TestEnv_CheckTrustedDirectory(t *testing.T) {
// 	t.Run("DirectoryTrusted", func(t *testing.T) {
// 		// Mock the current directory and trusted file
// 		currentDir := "/mock/current/dir"
// 		trustedDir := "/mock/current"
// 		mockUser := NewMockUser()
// 		mockUser.CurrentDir = currentDir
// 		mockUser.HomeDir = "/mock/home"

// 		// Initialize the env variable
// 		// mockInjector := di.NewMockInjector()
// 		// env := NewBaseEnvPrinter(mockInjector) // Adjust this line based on your actual Env initialization

// 		// Create a mock trusted file with the trusted directory
// 		trustedFilePath := path.Join(mockUser.HomeDir, ".config", "windsor", ".trusted")
// 		os.MkdirAll(path.Dir(trustedFilePath), os.ModePerm)
// 		os.WriteFile(trustedFilePath, []byte(trustedDir+"\n"), 0644)

// 		// Call CheckTrustedDirectory and check for errors
// 		err := CheckTrustedDirectory()
// 		if err != nil {
// 			t.Errorf("unexpected error: %v", err)
// 		}
// 	})

// 	t.Run("DirectoryNotTrusted", func(t *testing.T) {
// 		// Mock the current directory and trusted file
// 		currentDir := "/mock/current/dir"
// 		mockUser := NewMockUser()
// 		mockUser.CurrentDir = currentDir
// 		mockUser.HomeDir = "/mock/home"
// 		// user.SetMockUser(mockUser)

// 		// Ensure the trusted file is wiped out or deleted if it exists
// 		trustedFilePath := path.Join(mockUser.HomeDir, ".config", "windsor", ".trusted")
// 		os.Remove(trustedFilePath)
// 		os.MkdirAll(path.Dir(trustedFilePath), os.ModePerm)
// 		os.WriteFile(trustedFilePath, []byte("/some/other/dir\n"), 0644)

// 		// Call CheckTrustedDirectory and expect an error
// 		err := CheckTrustedDirectory()
// 		if err != nil {
// 			t.Errorf("unexpected error: %v", err)
// 		}
// 	})
// }

// func TestEnv_AddCurrentDirToTrustedFile(t *testing.T) {
// 	t.Run("AddDirectory", func(t *testing.T) {

// 		currentDir, err := os.Getwd()
// 		if err != nil {
// 			t.Fatalf("failed to get current directory: %v", err)
// 		}

// 		homeDir, err := os.UserHomeDir()
// 		if err != nil {
// 			t.Fatalf("Error retrieving user home directory: %v\n", err)
// 		}

// 		// Ensure the trusted file does not exist
// 		trustedFilePath := path.Join(homeDir, ".config", "windsor", ".trusted")
// 		os.Remove(trustedFilePath)

// 		// Call AddCurrentDirToTrustedFile and check for errors
// 		err = AddCurrentDirToTrustedFile()
// 		if err != nil {
// 			t.Errorf("unexpected error: %v", err)
// 		}

// 		// Verify that the current directory was added to the trusted file
// 		content, _ := os.ReadFile(trustedFilePath)
// 		expectedContent := currentDir + "\n"
// 		if string(content) != expectedContent {
// 			t.Errorf("trusted file content = %v, want %v", string(content), expectedContent)
// 		}
// 	})

// 	t.Run("DirectoryAlreadyTrusted", func(t *testing.T) {
// 		currentDir, err := os.Getwd()
// 		if err != nil {
// 			t.Fatalf("failed to get current directory: %v", err)
// 		}

// 		homeDir, err := os.UserHomeDir()
// 		if err != nil {
// 			t.Fatalf("Error retrieving user home directory: %v\n", err)
// 		}

// 		// Ensure the trusted file does not exist
// 		trustedFilePath := path.Join(homeDir, ".config", "windsor", ".trusted")
// 		os.Remove(trustedFilePath)

// 		// Create the .trusted file and write the current directory to it
// 		err = os.WriteFile(trustedFilePath, []byte(currentDir+"\n"), 0644)
// 		if err != nil {
// 			t.Fatalf("failed to initialize .trusted file: %v", err)
// 		}

// 		// Call AddCurrentDirToTrustedFile and check for errors
// 		err = AddCurrentDirToTrustedFile()
// 		if err != nil {
// 			t.Errorf("unexpected error: %v", err)
// 		}

// 		// Verify that the current directory was added to the trusted file
// 		content, _ := os.ReadFile(trustedFilePath)
// 		expectedContent := currentDir + "\n"
// 		if string(content) != expectedContent {
// 			t.Errorf("trusted file content = %v, want %v", string(content), expectedContent)
// 		}

// 		contentLines := len(strings.Split(string(content), "\n")) - 1
// 		if contentLines > 1 {
// 			t.Errorf("trusted file contains more than one entry: %d entries", contentLines)
// 		}

// 	})
// }
