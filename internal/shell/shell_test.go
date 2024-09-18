package shell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func createTempDir(t *testing.T, name string) string {
	dir, err := os.MkdirTemp("", name)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

func createFile(t *testing.T, dir, name string) {
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create file %s: %v", filePath, err)
	}
}

func initGitRepo(t *testing.T, dir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}
}

func normalizePath(path string) string {
	return strings.ReplaceAll(filepath.Clean(path), "\\", "/")
}

func TestGetProjectRoot_MaxDepth(t *testing.T) {
	// Create a temporary directory structure
	rootDir := createTempDir(t, "project-root")
	defer os.RemoveAll(rootDir)

	currentDir := rootDir
	for i := 0; i <= maxDepth; i++ {
		subDir := filepath.Join(currentDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}
		currentDir = subDir
	}

	// Change working directory to the deepest directory
	if err := os.Chdir(currentDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	shell := NewDefaultShell()

	// Test exceeding max depth
	projectRoot, err := shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("GetProjectRoot returned an error: %v", err)
	}

	if projectRoot != "" {
		t.Errorf("Expected project root to be empty, got %s", projectRoot)
	}
}

func TestGetProjectRoot_GitRepo(t *testing.T) {
	// Create a temporary directory structure
	rootDir := createTempDir(t, "project-root")
	defer os.RemoveAll(rootDir)

	subDir := filepath.Join(rootDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Initialize a git repository in the root directory
	initGitRepo(t, rootDir)

	// Change working directory to subDir
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	shell := NewDefaultShell()

	// Test finding the project root using git
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

func TestGetProjectRoot_Cached(t *testing.T) {
	// Create a temporary directory structure
	rootDir := createTempDir(t, "project-root")
	defer os.RemoveAll(rootDir)

	subDir := filepath.Join(rootDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Initialize a git repository in the root directory
	initGitRepo(t, rootDir)

	// Change working directory to subDir
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	shell := NewDefaultShell()

	// Test finding the project root using git
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

	// Test returning cached project root
	shell.projectRoot = expectedRootDir
	cachedProjectRoot, err := shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("GetProjectRoot returned an error: %v", err)
	}

	// Normalize paths for comparison
	cachedProjectRoot = normalizePath(cachedProjectRoot)

	if cachedProjectRoot != expectedRootDir {
		t.Errorf("Expected cached project root to be %s, got %s", expectedRootDir, cachedProjectRoot)
	}
}

func TestGetProjectRoot_NoGitNoYaml(t *testing.T) {
	// Create a temporary directory structure
	rootDir := createTempDir(t, "project-root")
	defer os.RemoveAll(rootDir)

	subDir := filepath.Join(rootDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Change working directory to subDir
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	shell := NewDefaultShell()

	// Test finding the project root without git or windsor.yaml
	projectRoot, err := shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("GetProjectRoot returned an error: %v", err)
	}

	if projectRoot != "" {
		t.Errorf("Expected project root to be empty, got %s", projectRoot)
	}
}

func TestGetProjectRoot_GetwdFails(t *testing.T) {
	// Override getwd to simulate an error
	originalGetwd := getwd
	getwd = func() (string, error) {
		return "", errors.New("simulated error")
	}
	defer func() { getwd = originalGetwd }() // Restore original getwd after test

	shell := NewDefaultShell()

	// Test GetProjectRoot when getwd fails
	_, err := shell.GetProjectRoot()
	if err == nil {
		t.Fatalf("Expected an error, got nil")
	}
}
