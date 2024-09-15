package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

var tempDir string

func setup() {
	var err error
	tempDir, err = os.MkdirTemp("", "testdata")
	if err != nil {
		panic(err)
	}
	os.WriteFile(tempDir+"/config.yaml", []byte("testKey: testValue\n"), 0644)
	os.WriteFile(tempDir+"/invalid.yaml", []byte("invalid content"), 0644)
}

func teardown() {
	os.RemoveAll(tempDir)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func TestViperConfigHandler_LoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVar  string
		wantErr bool
	}{
		{"ValidConfig", tempDir + "/config.yaml", false},
		{"InvalidConfig", tempDir + "/invalid.yaml", true},
		{"NonExistentConfig", tempDir + "/nonexistent.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ViperConfigHandler{}
			err := handler.LoadConfig(tt.envVar)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestViperConfigHandler_LoadConfig_CreateConfigFile(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Ensure the config file does not exist
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("Config file already exists at %s", configPath)
	}

	// Load the configuration, which should create the config file
	err := handler.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v, expected nil", err)
	}

	// Verify that the config file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}
}

func TestViperConfigHandler_LoadConfig_CreateConfigFileError(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	invalidConfigPath := filepath.Join(tempDir, "invalid", "config.yaml")

	// Mock osMkdirAll to return an error
	originalOsMkdirAll := osMkdirAll
	defer func() { osMkdirAll = originalOsMkdirAll }()
	osMkdirAll = func(path string, perm os.FileMode) error {
		fmt.Printf("Mock osMkdirAll called with path: %s\n", path)
		return nil
	}

	// Mock viper.SafeWriteConfigAs to return an error
	originalViperSafeWriteConfigAs := viperSafeWriteConfigAs
	defer func() { viperSafeWriteConfigAs = originalViperSafeWriteConfigAs }()
	viperSafeWriteConfigAs = func(filename string) error {
		fmt.Printf("Mock viperSafeWriteConfigAs called with filename: %s\n", filename)
		return fmt.Errorf("mock error creating config file")
	}

	// Load the configuration, which should attempt to create the config file and fail
	err := handler.LoadConfig(invalidConfigPath)
	if err == nil {
		t.Fatalf("LoadConfig() expected error, got nil")
	}

	expectedError := "mock error creating config file"
	if !containsErrorMessage(err.Error(), expectedError) {
		t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
	}
}

func TestViperConfigHandler_LoadConfig_EmptyPath(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Mock os.UserHomeDir to return the temporary directory
	originalUserHomeDir := osUserHomeDir
	defer func() { osUserHomeDir = originalUserHomeDir }()
	osUserHomeDir = func() (string, error) {
		return tempDir, nil
	}

	// Mock viper.GetString to return an empty string
	originalViperGetString := viperGetString
	defer func() { viperGetString = originalViperGetString }()
	viperGetString = func(key string) string {
		return ""
	}

	// Test when WINDSORCONFIG is not set and os.UserHomeDir() succeeds
	expectedPath := filepath.Join(tempDir, ".config", "windsor", "config.yaml")

	// Create the expected config file
	os.MkdirAll(filepath.Dir(expectedPath), 0755)
	os.WriteFile(expectedPath, []byte("testKey: testValue\n"), 0644)

	err := handler.LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v, expected nil", err)
	}

	if viper.ConfigFileUsed() != expectedPath {
		t.Errorf("LoadConfig() used config file = %v, expected %v", viper.ConfigFileUsed(), expectedPath)
	}
}

func TestViperConfigHandler_LoadConfig_EmptyPath_HomeDirError(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Mock os.UserHomeDir to return an error
	originalUserHomeDir := osUserHomeDir
	defer func() { osUserHomeDir = originalUserHomeDir }()
	osUserHomeDir = func() (string, error) {
		return "", fmt.Errorf("mock error")
	}

	// Mock viper.GetString to return an empty string
	originalViperGetString := viperGetString
	defer func() { viperGetString = originalViperGetString }()
	viperGetString = func(key string) string {
		return ""
	}

	err := handler.LoadConfig("")
	if err == nil || err.Error() != "error finding home directory, mock error" {
		t.Fatalf("LoadConfig() error = %v, expected 'error finding home directory, mock error'", err)
	}
}

func TestViperConfigHandler_GetConfigValue(t *testing.T) {
	v := &ViperConfigHandler{}
	viper.Set("testKey", "testValue")

	tests := []struct {
		name    string
		key     string
		want    string
		wantErr bool
	}{
		{"ExistingKey", "testKey", "testValue", false},
		{"NonExistentKey", "nonExistentKey", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v.GetConfigValue(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetConfigValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestViperConfigHandler_SetConfigValue(t *testing.T) {
	v := &ViperConfigHandler{}

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"SetNewKey", "newKey", "newValue"},
		{"OverwriteKey", "testKey", "newTestValue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := v.SetConfigValue(tt.key, tt.value); err != nil {
				t.Errorf("SetConfigValue() error = %v", err)
			}
			if got := viper.GetString(tt.key); got != tt.value {
				t.Errorf("SetConfigValue() = %v, want %v", got, tt.value)
			}
		})
	}
}

func TestViperConfigHandler_LoadConfig_CreateParentDirs(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nested", "config.yaml")

	// Ensure the parent directories do not exist
	if _, err := os.Stat(filepath.Dir(configPath)); !os.IsNotExist(err) {
		t.Fatalf("Parent directories already exist at %s", filepath.Dir(configPath))
	}

	// Load the configuration, which should create the parent directories and the config file
	err := handler.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v, expected nil", err)
	}

	// Verify that the parent directories and config file were created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}
}

func TestViperConfigHandler_SaveConfig_CreateParentDirs(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("saveKey", "saveValue")

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nested", "save_config.yaml")

	// Ensure the parent directories do not exist
	if _, err := os.Stat(filepath.Dir(configPath)); !os.IsNotExist(err) {
		t.Fatalf("Parent directories already exist at %s", filepath.Dir(configPath))
	}

	// Save the configuration, which should create the parent directories and the config file
	err := handler.SaveConfig(configPath)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v, expected nil", err)
	}

	// Verify that the parent directories and config file were created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}
}

func TestViperConfigHandler_LoadConfig_CreateParentDirsError(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Mock osMkdirAll to return an error
	originalOsMkdirAll := osMkdirAll
	defer func() { osMkdirAll = originalOsMkdirAll }()
	osMkdirAll = func(path string, perm os.FileMode) error {
		return fmt.Errorf("mock error creating directories")
	}

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nested", "config.yaml")

	// Load the configuration, which should attempt to create the parent directories and fail
	err := handler.LoadConfig(configPath)
	if err == nil {
		t.Fatalf("LoadConfig() expected error, got nil")
	}

	expectedError := "error creating directories"
	if !containsErrorMessage(err.Error(), expectedError) {
		t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
	}
}

// containsErrorMessage checks if the actual error message contains the expected error message
func containsErrorMessage(actual, expected string) bool {
	return strings.Contains(actual, expected)
}

func TestViperConfigHandler_SaveConfig_InvalidPath(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("saveKey", "saveValue")

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	invalidConfigPath := filepath.Join(tempDir, "invalid", "save_config.yaml")

	// Mock osMkdirAll to return an error
	originalOsMkdirAll := osMkdirAll
	defer func() { osMkdirAll = originalOsMkdirAll }()
	osMkdirAll = func(path string, perm os.FileMode) error {
		fmt.Printf("Mock osMkdirAll called with path: %s\n", path)
		return fmt.Errorf("mock error creating directories")
	}

	// Save the configuration, which should attempt to create the config file and fail
	fmt.Println("Calling SaveConfig")
	err := handler.SaveConfig(invalidConfigPath)
	if err == nil {
		t.Fatalf("SaveConfig() expected error, got nil")
	}

	expectedError := "mock error creating directories"
	if !containsErrorMessage(err.Error(), expectedError) {
		t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
	}
	fmt.Println("SaveConfig test completed")
}

func TestViperConfigHandler_SaveConfig_WriteConfigError(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("saveKey", "saveValue")

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	validConfigPath := filepath.Join(tempDir, "save_config.yaml")

	// Mock viper.WriteConfigAs to return an error
	originalViperWriteConfigAs := viperWriteConfigAs
	defer func() { viperWriteConfigAs = originalViperWriteConfigAs }()
	viperWriteConfigAs = func(filename string) error {
		fmt.Printf("Mock viperWriteConfigAs called with filename: %s\n", filename)
		return fmt.Errorf("mock error writing config")
	}

	// Save the configuration, which should attempt to write the config file and fail
	err := handler.SaveConfig(validConfigPath)
	if err == nil {
		t.Fatalf("SaveConfig() expected error, got nil")
	}

	expectedError := "mock error writing config"
	if !containsErrorMessage(err.Error(), expectedError) {
		t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
	}
}

func TestViperConfigHandler_SaveConfig_EmptyPath_ValidConfigFileUsed(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Mock viper.ConfigFileUsed to return a valid path
	originalViperConfigFileUsed := viperConfigFileUsed
	defer func() { viperConfigFileUsed = originalViperConfigFileUsed }()
	viperConfigFileUsed = func() string {
		return configPath
	}

	handler := &ViperConfigHandler{}

	// Ensure the config file does not exist
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("Config file already exists at %s", configPath)
	}

	// Save the configuration, which should use the mocked path
	fmt.Printf("Calling SaveConfig with empty path\n")
	err := handler.SaveConfig("")
	if err != nil {
		t.Fatalf("SaveConfig() error = %v, expected nil", err)
	}

	// Verify that the config file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}
}

func TestViperConfigHandler_SaveConfig_EmptyPath_InvalidConfigFileUsed(t *testing.T) {
	// Mock viper.ConfigFileUsed to return an empty path
	originalViperConfigFileUsed := viperConfigFileUsed
	defer func() { viperConfigFileUsed = originalViperConfigFileUsed }()
	viperConfigFileUsed = func() string {
		return ""
	}

	handler := &ViperConfigHandler{}

	// Save the configuration, which should return an error
	fmt.Printf("Calling SaveConfig with empty path\n")
	err := handler.SaveConfig("")
	if err == nil {
		t.Fatalf("SaveConfig() expected error, got nil")
	}

	expectedError := "path cannot be empty"
	if err.Error() != expectedError {
		t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
	}
}
