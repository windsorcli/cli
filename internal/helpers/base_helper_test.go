package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
)

// mockExecCmd is a custom exec.Cmd that returns predefined output
type mockExecCmd struct {
	output string
}

func (c *mockExecCmd) Run() error {
	_, _ = fmt.Fprint(os.Stdout, c.output)
	return nil
}

func (c *mockExecCmd) Output() ([]byte, error) {
	return []byte(c.output), nil
}

// setMockExecCommand sets the mock output for exec.Command
func setMockExecCommand(mockOutput string) {
	execCommand = func(command string, args ...string) *exec.Cmd {
		cmd := &exec.Cmd{
			Path: "/bin/sh",
			Args: []string{"-c", fmt.Sprintf("echo '%s'", mockOutput)},
		}
		cmd.Stdout = &bytes.Buffer{}
		cmd.Stderr = &bytes.Buffer{}
		return cmd
	}
}

func TestGetParentProcessName(t *testing.T) {
	// Mock exec.Command
	setMockExecCommand("ProcessId,ExecutablePath\n1234,C:\\Windows\\System32\\cmd.exe\n")
	defer func() { execCommand = exec.Command }()

	expected := "C:\\Windows\\System32\\cmd.exe"
	result, err := getParentProcessName()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != expected {
		t.Fatalf("expected %v, got %v", expected, result)
	}
}

func TestGetShellType(t *testing.T) {
	tests := []struct {
		goos       string
		expected   string
		mockOutput string
	}{
		{"windows", "cmd", "ProcessId,ExecutablePath\n1234,C:\\Windows\\System32\\cmd.exe\n"},
		{"windows", "powershell", "ProcessId,ExecutablePath\n1234,C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe\n"},
		{"windows", "unknown", "ProcessId,ExecutablePath\n"},
		{"linux", "unix", ""},
	}

	for _, test := range tests {
		goos = test.goos
		setMockExecCommand(test.mockOutput)
		defer func() { execCommand = exec.Command }()

		result := getShellType()
		if result != test.expected {
			t.Fatalf("expected %v, got %v", test.expected, result)
		}
	}
}

func TestPrintEnvVars(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			if key == "context" {
				return "testContext", nil
			}
			return "", fmt.Errorf("key not found")
		},
		GetNestedMapFunc: func(key string) (map[string]interface{}, error) {
			if key == "contexts.testContext.environment" {
				return map[string]interface{}{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, fmt.Errorf("key not found")
		},
	}
	baseHelper := &BaseHelper{ConfigHandler: mockConfigHandler}

	// Mock getShellType
	originalGetShellType := getShellType
	defer func() { getShellType = originalGetShellType }()

	tests := []struct {
		shellType string
		expected  []string
	}{
		{"powershell", []string{"$env:VAR1='value1'", "$env:VAR2='value2'", "$env:WINDSORCONTEXT='testContext'"}},
		{"cmd", []string{"set VAR1=value1", "set VAR2=value2", "set WINDSORCONTEXT=testContext"}},
		{"unix", []string{"export VAR1='value1'", "export VAR2='value2'", "export WINDSORCONTEXT='testContext'"}},
	}

	for _, test := range tests {
		test := test // capture range variable
		getShellType = func() string { return test.shellType }

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
	mockConfigHandler = &config.MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			if key == "context" {
				return "testContext", nil
			}
			return "", fmt.Errorf("key not found")
		},
		GetNestedMapFunc: func(key string) (map[string]interface{}, error) {
			return nil, fmt.Errorf("key not found")
		},
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
	mockConfigHandler = &config.MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			if key == "context" {
				return "testContext", nil
			}
			return "", fmt.Errorf("key not found")
		},
		GetNestedMapFunc: func(key string) (map[string]interface{}, error) {
			if key == "contexts.testContext.environment" {
				return map[string]interface{}{
					"VAR1": "value1",
					"VAR2": 123, // Non-string value
				}, nil
			}
			return nil, fmt.Errorf("key not found")
		},
	}
	baseHelper = &BaseHelper{ConfigHandler: mockConfigHandler}

	err = baseHelper.PrintEnvVars()
	if err == nil || !strings.Contains(err.Error(), "non-string value found in environment variables for context testContext") {
		t.Fatalf("expected error for non-string value, got %v", err)
	}
}
