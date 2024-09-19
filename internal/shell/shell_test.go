package shell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Helper function to create a temporary directory
func createTempDir(t *testing.T, name string) string {
	dir, err := os.MkdirTemp("", name)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Failed to remove temp dir: %v", err)
		}
	})
	return dir
}

// Helper function to change the working directory
func changeDir(t *testing.T, dir string) {
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
}

// Helper function to create a file
func createFile(t *testing.T, dir, name string) {
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create file %s: %v", filePath, err)
	}
}

// Helper function to initialize a git repository
func initGitRepo(t *testing.T, dir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}
}

// Helper function to normalize a path
func normalizePath(path string) string {
	return strings.ReplaceAll(filepath.Clean(path), "\\", "/")
}

func TestGetProjectRoot(t *testing.T) {
	t.Run("GitRepo", func(t *testing.T) {
		// Given a temporary directory structure with a git repository
		rootDir := createTempDir(t, "project-root")

		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		// When initializing a git repository in the root directory
		initGitRepo(t, rootDir)

		// And changing the working directory to a subdirectory
		changeDir(t, subDir)

		shell := NewDefaultShell()

		// Then GetProjectRoot should find the project root using git
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
	})

	t.Run("Cached", func(t *testing.T) {
		// Given a temporary directory structure with a git repository
		rootDir := createTempDir(t, "project-root")

		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		// When initializing a git repository in the root directory
		initGitRepo(t, rootDir)

		// And changing the working directory to a subdirectory
		changeDir(t, subDir)

		shell := NewDefaultShell()

		// Then GetProjectRoot should find the project root using git
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

		// And when the project root is cached
		shell.projectRoot = expectedRootDir
		cachedProjectRoot, err := shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("GetProjectRoot returned an error: %v", err)
		}

		// Then GetProjectRoot should return the cached project root
		cachedProjectRoot = normalizePath(cachedProjectRoot)

		if cachedProjectRoot != expectedRootDir {
			t.Errorf("Expected cached project root to be %s, got %s", expectedRootDir, cachedProjectRoot)
		}
	})

	t.Run("MaxDepth", func(t *testing.T) {
		// Given a temporary directory structure with nested subdirectories
		rootDir := createTempDir(t, "project-root")

		currentDir := rootDir
		for i := 0; i <= maxDepth; i++ {
			subDir := filepath.Join(currentDir, "subdir")
			if err := os.Mkdir(subDir, 0755); err != nil {
				t.Fatalf("Failed to create subdir %d: %v", i, err)
			}
			currentDir = subDir
		}

		// When changing the working directory to the deepest directory
		changeDir(t, currentDir)

		shell := NewDefaultShell()

		// Then GetProjectRoot should return an empty project root
		projectRoot, err := shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("GetProjectRoot returned an error: %v", err)
		}

		if projectRoot != "" {
			t.Errorf("Expected project root to be empty, got %s", projectRoot)
		}
	})

	t.Run("NoGitNoYaml", func(t *testing.T) {
		// Given a temporary directory structure without git or windsor.yaml
		rootDir := createTempDir(t, "project-root")

		subDir := filepath.Join(rootDir, "subdir")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		// When changing the working directory to a subdirectory
		changeDir(t, subDir)

		shell := NewDefaultShell()

		// Then GetProjectRoot should return an empty project root
		projectRoot, err := shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("GetProjectRoot returned an error: %v", err)
		}

		if projectRoot != "" {
			t.Errorf("Expected project root to be empty, got %s", projectRoot)
		}
	})

	t.Run("GetwdFails", func(t *testing.T) {
		// Given that getwd is overridden to simulate an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", errors.New("simulated error")
		}
		defer func() { getwd = originalGetwd }() // Restore original getwd after test

		shell := NewDefaultShell()

		// Then GetProjectRoot should return an error
		_, err := shell.GetProjectRoot()
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}
