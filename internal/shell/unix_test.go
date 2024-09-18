//go:build darwin || linux
// +build darwin linux

package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultShell_PrintEnvVars_Unix(t *testing.T) {
	shell := NewDefaultShell()
	envVars := map[string]string{
		"VAR2": "value2",
		"VAR1": "value1",
	}
	expectedOutput := "export VAR1=\"value1\"\nexport VAR2=\"value2\"\n"

	// Capture the output
	var output bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	go func() {
		shell.PrintEnvVars(envVars)
		w.Close()
	}()

	output.ReadFrom(r)
	os.Stdout = originalStdout

	if output.String() != expectedOutput {
		t.Errorf("PrintEnvVars() output = %v, want %v", output.String(), expectedOutput)
	}
}

func TestGetProjectRoot_WindsorYaml_Unix(t *testing.T) {
	// Create a temporary directory structure
	rootDir := createTempDir(t, "project-root")
	defer os.RemoveAll(rootDir)

	subDir := filepath.Join(rootDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create windsor.yaml in the root directory
	createFile(t, rootDir, "windsor.yaml")

	// Change working directory to subDir
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	shell := NewDefaultShell()

	// Test finding the project root using windsor.yaml
	projectRoot, err := shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("GetProjectRoot returned an error: %v", err)
	}

	// Resolve symlinks to handle macOS /private prefix
	expectedRootDir, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks for rootDir: %v", err)
	}

	// Normalize paths for comparison
	expectedRootDir = normalizePath(expectedRootDir)
	projectRoot = normalizePath(projectRoot)

	if projectRoot != expectedRootDir {
		t.Errorf("Expected project root to be %s, got %s", expectedRootDir, projectRoot)
	}
}

func TestGetProjectRoot_WindsorYml_Unix(t *testing.T) {
	// Create a temporary directory structure
	rootDir := createTempDir(t, "project-root")
	defer os.RemoveAll(rootDir)

	subDir := filepath.Join(rootDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create windsor.yml in the root directory
	createFile(t, rootDir, "windsor.yml")

	// Change working directory to subDir
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	shell := NewDefaultShell()

	// Test finding the project root using windsor.yml
	projectRoot, err := shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("GetProjectRoot returned an error: %v", err)
	}

	// Resolve symlinks to handle macOS /private prefix
	expectedRootDir, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks for rootDir: %v", err)
	}

	// Normalize paths for comparison
	expectedRootDir = normalizePath(expectedRootDir)
	projectRoot = normalizePath(projectRoot)

	if projectRoot != expectedRootDir {
		t.Errorf("Expected project root to be %s, got %s", expectedRootDir, projectRoot)
	}
}
