package config

import (
	"fmt"
	"os"
	"path/filepath"
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

	// Ensure the config file does not exist
	if _, err := os.Stat(invalidConfigPath); !os.IsNotExist(err) {
		t.Fatalf("Config file already exists at %s", invalidConfigPath)
	}

	// Load the configuration, which should attempt to create the config file and fail
	err := handler.LoadConfig(invalidConfigPath)
	if err == nil {
		t.Fatalf("LoadConfig() expected error, got nil")
	}

	expectedError := "error creating config file"
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

func TestViperConfigHandler_SaveConfig(t *testing.T) {
	v := &ViperConfigHandler{}
	viper.Set("saveKey", "saveValue")

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"ValidPath", tempDir + "/save_config.yaml", false},
		{"InvalidPath", "/invalid/path/save_config.yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := v.SaveConfig(tt.path); (err != nil) != tt.wantErr {
				t.Errorf("SaveConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if _, err := os.Stat(tt.path); os.IsNotExist(err) {
					t.Errorf("SaveConfig() file not created at %v", tt.path)
				} else {
					os.Remove(tt.path) // Clean up after test
				}
			}
		})
	}
}

// containsErrorMessage checks if the actual error message contains the expected error message
func containsErrorMessage(actual, expected string) bool {
	return len(actual) >= len(expected) && actual[:len(expected)] == expected
}
