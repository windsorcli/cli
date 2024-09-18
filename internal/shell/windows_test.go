//go:build windows
// +build windows

package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func getLongPathName(shortPath string) (string, error) {
	p, err := windows.UTF16PtrFromString(shortPath)
	if err != nil {
		return "", err
	}
	b := make([]uint16, windows.MAX_LONG_PATH)
	r, err := windows.GetLongPathName(p, &b[0], uint32(len(b)))
	if r == 0 {
		return "", err
	}
	return windows.UTF16ToString(b), nil
}

func normalizeWindowsPath(path string) string {
	longPath, err := getLongPathName(path)
	if err != nil {
		return normalizePath(path)
	}
	return normalizePath(longPath)
}

func TestDefaultShell_PrintEnvVars_Windows(t *testing.T) {
	shell := NewDefaultShell()
	envVars := map[string]string{
		"VAR2": "value2",
		"VAR1": "value1",
		"VAR3": "",
	}

	// Expected output for PowerShell
	expectedOutputPowerShell := "$env:VAR1=\"value1\"\n$env:VAR2=\"value2\"\nRemove-Item Env:VAR3\n"

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

	// Check if the output matches PowerShell format
	if output.String() != expectedOutputPowerShell {
		t.Errorf("PrintEnvVars() output = %v, want %v", output.String(), expectedOutputPowerShell)
	}
}

func TestGetProjectRoot_WindsorYaml(t *testing.T) {
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
	expectedRootDir = normalizeWindowsPath(expectedRootDir)
	projectRoot = normalizeWindowsPath(projectRoot)

	if projectRoot != expectedRootDir {
		t.Errorf("Expected project root to be %s, got %s", expectedRootDir, projectRoot)
	}
}

func TestGetProjectRoot_WindsorYml(t *testing.T) {
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
	expectedRootDir = normalizeWindowsPath(expectedRootDir)
	projectRoot = normalizeWindowsPath(projectRoot)

	if projectRoot != expectedRootDir {
		t.Errorf("Expected project root to be %s, got %s", expectedRootDir, projectRoot)
	}
}
