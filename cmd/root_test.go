package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Mock function to simulate an error when getting the home directory
var mockUserHomeDir = func() (string, error) {
	return "", fmt.Errorf("mock error: unable to find home directory")
}

func resetViper() {
	viper.Reset()
}

func TestExecute_NoError(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to track if it's called
	exitCalled := false
	exitFunc = func(code int) {
		exitCalled = true
	}

	// Mock PersistentPreRun to avoid actual configuration loading
	originalPersistentPreRun := rootCmd.PersistentPreRun
	defer func() { rootCmd.PersistentPreRun = originalPersistentPreRun }()
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Do nothing
	}

	// Set rootCmd's RunE to a function that returns nil (no error)
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	// Execute the command
	Execute()

	// Verify that exitFunc was not called
	if exitCalled {
		t.Errorf("exitFunc was called when it should not have been")
	}
}

func TestExecute_WithError(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Set rootCmd's RunE to a function that returns an error
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return errors.New("test error")
	}

	// Execute the command
	Execute()

	// Verify that exitFunc was called with code 1
	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}
}

func TestRootCmd_DefaultConfig(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to track if it's called
	exitCalled := false
	exitFunc = func(code int) {
		exitCalled = true
	}

	// Reset Viper state
	resetViper()

	// Create a temporary directory to act as the home directory
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	os.Mkdir(homeDir, 0755)

	// Set the HOME environment variable to the temporary directory
	os.Setenv("HOME", homeDir)
	defer os.Unsetenv("HOME")

	// Create a default config file
	configDir := filepath.Join(homeDir, ".config", "windsor")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.yaml")
	err := os.WriteFile(configFile, []byte("key: value"), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Debug: Verify the contents of the config file
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	t.Logf("Config file contents: %s", string(content))

	// Directly call the PersistentPreRun function
	rootCmd.PersistentPreRun(rootCmd, []string{})

	// Verify that the configuration was loaded correctly
	if viper.GetString("key") != "value" {
		t.Errorf("Expected config key 'key' to be 'value', got '%s'", viper.GetString("key"))
	}

	// Verify that exitFunc was not called
	if exitCalled {
		t.Errorf("exitFunc was called when it should not have been")
	}
}

func TestRootCmd_EnvConfig(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to track if it's called
	exitCalled := false
	exitFunc = func(code int) {
		exitCalled = true
	}

	// Reset Viper state
	resetViper()

	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	os.WriteFile(configFile, []byte("key: value"), 0644)

	// Set the WINDSORCONFIG environment variable to the temporary config file
	os.Setenv("WINDSORCONFIG", configFile)
	defer os.Unsetenv("WINDSORCONFIG")

	// Execute the root command
	rootCmd.PersistentPreRun(rootCmd, []string{})

	// Verify that the configuration was loaded correctly
	if viper.GetString("key") != "value" {
		t.Errorf("Expected config key 'key' to be 'value', got '%s'", viper.GetString("key"))
	}

	// Verify that exitFunc was not called
	if exitCalled {
		t.Errorf("exitFunc was called when it should not have been")
	}
}

func TestRootCmd_HomeDirError(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Save the original userHomeDir and restore it after the test
	originalUserHomeDir := userHomeDir
	defer func() { userHomeDir = originalUserHomeDir }()

	// Replace userHomeDir with the mock function
	userHomeDir = mockUserHomeDir

	// Reset Viper state
	resetViper()

	// Execute the root command
	rootCmd.PersistentPreRun(rootCmd, []string{})

	// Verify that exitFunc was called with code 1
	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}
}

func TestRootCmd_ConfigReadError(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Reset Viper state
	resetViper()

	// Create a temporary directory to act as the home directory
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	os.Mkdir(homeDir, 0755)

	// Set the HOME environment variable to the temporary directory
	os.Setenv("HOME", homeDir)
	defer os.Unsetenv("HOME")

	// Create a default config file with invalid content
	configDir := filepath.Join(homeDir, ".config", "windsor")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configFile, []byte("invalid content"), 0644)

	// Execute the root command
	rootCmd.PersistentPreRun(rootCmd, []string{})

	// Verify that exitFunc was called with code 1
	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}
}
