package helpers

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
)

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	config.ConfigHandler
	configValues map[string]string
	nestedMaps   map[string]map[string]interface{}
}

func (m *MockConfigHandler) GetConfigValue(key string) (string, error) {
	if value, ok := m.configValues[key]; ok {
		return value, nil
	}
	return "", fmt.Errorf("key not found")
}

func (m *MockConfigHandler) GetNestedMap(key string) (map[string]interface{}, error) {
	if value, ok := m.nestedMaps[key]; ok {
		return value, nil
	}
	return nil, fmt.Errorf("key not found")
}

func TestGetEnvVars(t *testing.T) {
	mockConfigHandler := &MockConfigHandler{
		configValues: map[string]string{
			"context": "testContext",
		},
		nestedMaps: map[string]map[string]interface{}{
			"contexts.testContext.environment": {
				"VAR1": "value1",
				"VAR2": "value2",
			},
		},
	}
	baseHelper := &BaseHelper{ConfigHandler: mockConfigHandler}

	expected := map[string]string{"VAR1": "value1", "VAR2": "value2"}
	result, err := baseHelper.GetEnvVars()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
}

func TestGetEnvVarsError(t *testing.T) {
	mockConfigHandler := &MockConfigHandler{
		configValues: map[string]string{},
		nestedMaps:   map[string]map[string]interface{}{},
	}
	baseHelper := &BaseHelper{ConfigHandler: mockConfigHandler}

	_, err := baseHelper.GetEnvVars()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestPrintEnvVars(t *testing.T) {
	mockConfigHandler := &MockConfigHandler{
		configValues: map[string]string{
			"context": "testContext",
		},
		nestedMaps: map[string]map[string]interface{}{
			"contexts.testContext.environment": {
				"VAR1": "value1",
				"VAR2": "value2",
			},
		},
	}
	baseHelper := &BaseHelper{ConfigHandler: mockConfigHandler}

	// Mock goos and getEnv
	originalGoos := goos
	originalGetEnv := getEnv
	defer func() {
		goos = originalGoos
		getEnv = originalGetEnv
	}()

	tests := []struct {
		goos     string
		shell    string
		comSpec  string
		expected []string
	}{
		{"windows", "powershell", "", []string{"$env:VAR1='value1'", "$env:VAR2='value2'"}},
		{"windows", "cmd", "", []string{"set VAR1=value1", "set VAR2=value2"}},
		{"linux", "bash", "", []string{"export VAR1='value1'", "export VAR2='value2'"}},
		{"windows", "", "powershell.exe", []string{"$env:VAR1='value1'", "$env:VAR2='value2'"}},
	}

	for _, test := range tests {
		goos = test.goos
		getEnv = func(key string) string {
			if key == "SHELL" {
				return test.shell
			}
			if key == "ComSpec" {
				return test.comSpec
			}
			return ""
		}

		// Capture the output
		var output bytes.Buffer
		stdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := baseHelper.PrintEnvVars()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		w.Close()
		os.Stdout = stdout
		output.ReadFrom(r)

		// Sort the output lines
		outputLines := strings.Split(strings.TrimSpace(output.String()), "\n")
		sort.Strings(outputLines)
		sortedOutput := strings.Join(outputLines, "\n") + "\n"

		// Sort the expected lines
		sort.Strings(test.expected)
		expectedOutput := strings.Join(test.expected, "\n") + "\n"

		if sortedOutput != expectedOutput {
			t.Fatalf("expected %v, got %v", expectedOutput, sortedOutput)
		}
	}

	// Test case where nested map is not found
	mockConfigHandler = &MockConfigHandler{
		configValues: map[string]string{
			"context": "testContext",
		},
		nestedMaps: map[string]map[string]interface{}{},
	}
	baseHelper = &BaseHelper{ConfigHandler: mockConfigHandler}

	// Capture the output
	var output bytes.Buffer
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := baseHelper.PrintEnvVars()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	w.Close()
	os.Stdout = stdout
	output.ReadFrom(r)

	expectedOutput := ""
	if output.String() != expectedOutput {
		t.Fatalf("expected %v, got %v", expectedOutput, output.String())
	}

	// Test case where a non-string value is found in environment variables
	mockConfigHandler = &MockConfigHandler{
		configValues: map[string]string{
			"context": "testContext",
		},
		nestedMaps: map[string]map[string]interface{}{
			"contexts.testContext.environment": {
				"VAR1": "value1",
				"VAR2": 123, // Non-string value
			},
		},
	}
	baseHelper = &BaseHelper{ConfigHandler: mockConfigHandler}

	err = baseHelper.PrintEnvVars()
	if err == nil || !strings.Contains(err.Error(), "non-string value found in environment variables for context testContext") {
		t.Fatalf("expected error for non-string value, got %v", err)
	}
}
