package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureProjectAnchor(t *testing.T) {
	setup := func(t *testing.T) string {
		t.Helper()
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		tmpDir := t.TempDir()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chdir(origDir)
		})
		// Isolate HOME so a real global windsor.yaml doesn't shortcut detection.
		t.Setenv("HOME", tmpDir)
		// Resolve any symlinks (macOS /var -> /private/var) so our string compare matches.
		resolved, err := filepath.EvalSymlinks(tmpDir)
		if err != nil {
			return tmpDir
		}
		return resolved
	}

	t.Run("WritesMinimalWindsorYamlWhenNoProjectExists", func(t *testing.T) {
		// Given an empty directory
		tmpDir := setup(t)

		// When EnsureProjectAnchor is called
		if err := EnsureProjectAnchor(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then a minimal windsor.yaml should be created in cwd
		data, err := os.ReadFile(filepath.Join(tmpDir, "windsor.yaml"))
		if err != nil {
			t.Fatalf("Expected windsor.yaml to exist, got %v", err)
		}
		if !strings.Contains(string(data), "version: v1alpha1") {
			t.Errorf("Expected minimal v1alpha1 root config, got: %s", string(data))
		}
	})

	t.Run("NoOpWhenProjectAlreadyExistsInCwd", func(t *testing.T) {
		// Given a directory that already has windsor.yaml
		tmpDir := setup(t)
		existing := []byte("version: v1alpha1\ncustom: preserved\n")
		if err := os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), existing, 0644); err != nil {
			t.Fatalf("Failed to seed windsor.yaml: %v", err)
		}

		// When EnsureProjectAnchor is called
		if err := EnsureProjectAnchor(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the existing file should not be overwritten
		data, err := os.ReadFile(filepath.Join(tmpDir, "windsor.yaml"))
		if err != nil {
			t.Fatalf("Expected windsor.yaml to exist, got %v", err)
		}
		if string(data) != string(existing) {
			t.Errorf("Expected existing windsor.yaml preserved, got: %s", string(data))
		}
	})

	t.Run("NoOpWhenProjectExistsInParentDirectory", func(t *testing.T) {
		// Given a parent directory containing windsor.yaml and a cwd below it
		tmpDir := setup(t)
		subDir := filepath.Join(tmpDir, "sub")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to seed windsor.yaml: %v", err)
		}
		if err := os.Chdir(subDir); err != nil {
			t.Fatalf("Failed to chdir to subdir: %v", err)
		}

		// When EnsureProjectAnchor is called
		if err := EnsureProjectAnchor(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then no windsor.yaml should be created in the subdir
		if _, err := os.Stat(filepath.Join(subDir, "windsor.yaml")); err == nil {
			t.Error("Expected no windsor.yaml in subdir (parent already has one)")
		}
	})
}

func TestEnsureGitRepository(t *testing.T) {
	setup := func(t *testing.T) string {
		t.Helper()
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		tmpDir := t.TempDir()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chdir(origDir)
		})
		// Isolate HOME so a real ~/.git doesn't shortcut detection.
		t.Setenv("HOME", tmpDir)
		resolved, err := filepath.EvalSymlinks(tmpDir)
		if err != nil {
			return tmpDir
		}
		return resolved
	}

	t.Run("ReturnsNilWhenGitDirInCwd", func(t *testing.T) {
		// Given a directory with .git/
		tmpDir := setup(t)
		if err := os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755); err != nil {
			t.Fatalf("Failed to create .git: %v", err)
		}

		if err := EnsureGitRepository(); err != nil {
			t.Errorf("Expected no error with .git present, got %v", err)
		}
	})

	t.Run("ReturnsNilWhenGitFileInCwd", func(t *testing.T) {
		// Given a directory with .git as a file (worktree / submodule layout)
		tmpDir := setup(t)
		if err := os.WriteFile(filepath.Join(tmpDir, ".git"), []byte("gitdir: ../parent/.git/worktrees/x\n"), 0644); err != nil {
			t.Fatalf("Failed to create .git file: %v", err)
		}

		if err := EnsureGitRepository(); err != nil {
			t.Errorf("Expected no error with .git file present, got %v", err)
		}
	})

	t.Run("ReturnsNilWhenGitDirInParent", func(t *testing.T) {
		// Given a parent directory with .git/ and a cwd below it
		tmpDir := setup(t)
		if err := os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755); err != nil {
			t.Fatalf("Failed to create .git: %v", err)
		}
		subDir := filepath.Join(tmpDir, "sub")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}
		if err := os.Chdir(subDir); err != nil {
			t.Fatalf("Failed to chdir to subdir: %v", err)
		}

		if err := EnsureGitRepository(); err != nil {
			t.Errorf("Expected no error walking up to parent .git, got %v", err)
		}
	})

	t.Run("ReturnsActionableErrorWhenNoGit", func(t *testing.T) {
		// Given a directory with no .git anywhere up the walk
		_ = setup(t)

		err := EnsureGitRepository()
		if err == nil {
			t.Fatal("Expected error when no .git present, got nil")
		}
		if !strings.Contains(err.Error(), "must run inside a git repository") {
			t.Errorf("Expected hint about git repository, got: %v", err)
		}
		if !strings.Contains(err.Error(), "git init") {
			t.Errorf("Expected suggestion to run 'git init', got: %v", err)
		}
	})
}
