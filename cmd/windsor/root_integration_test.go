package windsor

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Integration tests
// TestIntegration_RunWindsorCLI tests the Windsor CLI help command output
func TestIntegration_RunWindsorCLI(t *testing.T) {
	// Get the absolute path to the main.go file
	mainGoPath, err := filepath.Abs("../../cmd/main.go")
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	cmd := exec.Command("go", "run", mainGoPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run Windsor CLI: %v\nOutput: %s", err, string(output))
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Windsor CLI is a command-line interface for managing Windsor blueprints.") {
		t.Errorf("Expected help output to contain description, got %q", outputStr)
	}
	if !strings.Contains(outputStr, "Usage:") {
		t.Errorf("Expected help output to contain usage information, got %q", outputStr)
	}
	if !strings.Contains(outputStr, "Available Commands:") {
		t.Errorf("Expected help output to contain available commands, got %q", outputStr)
	}
	if !strings.Contains(outputStr, "Flags:") {
		t.Errorf("Expected help output to contain flags, got %q", outputStr)
	}
}

// TestIntegration_RunWindsorCLIWithArgs tests the Windsor CLI version command output
func TestIntegration_RunWindsorCLIWithArgs(t *testing.T) {
	// Get the absolute path to the main.go file
	mainGoPath, err := filepath.Abs("../../cmd/main.go")
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	cmd := exec.Command("go", "run", mainGoPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run Windsor CLI with args: %v\nOutput: %s", err, string(output))
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Windsor CLI") {
		t.Errorf("Expected version output to contain 'Windsor CLI', got %q", outputStr)
	}

	// Check for semantic versioning
	semverRegex := `v\d+\.\d+\.\d+`
	matched, err := regexp.MatchString(semverRegex, outputStr)
	if err != nil {
		t.Fatalf("Failed to compile regex: %v", err)
	}
	if !matched {
		t.Errorf("Expected version output to contain a semantic version, got %q", outputStr)
	}
}
