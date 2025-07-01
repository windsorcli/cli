package bundler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test KustomizeBundler
// =============================================================================

func TestKustomizeBundler_NewKustomizeBundler(t *testing.T) {
	setup := func(t *testing.T) *KustomizeBundler {
		t.Helper()
		return NewKustomizeBundler()
	}

	t.Run("CreatesInstanceWithBaseBundler", func(t *testing.T) {
		// Given no preconditions
		// When creating a new kustomize bundler
		bundler := setup(t)

		// Then it should not be nil
		if bundler == nil {
			t.Fatal("Expected non-nil bundler")
		}
		// And it should have inherited BaseBundler properties
		if bundler.shims == nil {
			t.Error("Expected shims to be inherited from BaseBundler")
		}
		// And other fields should be nil until Initialize
		if bundler.shell != nil {
			t.Error("Expected shell to be nil before Initialize")
		}
		if bundler.injector != nil {
			t.Error("Expected injector to be nil before Initialize")
		}
	})
}

func TestKustomizeBundler_Bundle(t *testing.T) {
	setup := func(t *testing.T) (*KustomizeBundler, *BundlerMocks) {
		t.Helper()
		mocks := setupBundlerMocks(t)
		bundler := NewKustomizeBundler()
		bundler.shims = mocks.Shims
		bundler.Initialize(mocks.Injector)
		return bundler, mocks
	}

	t.Run("SuccessWithValidKustomizeFiles", func(t *testing.T) {
		// Given a kustomize bundler with valid kustomize files
		bundler, mocks := setup(t)

		// Set up mocks to simulate finding kustomize files
		filesAdded := make(map[string][]byte)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded[path] = content
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate finding multiple files in kustomize directory
			// Use filepath.Join to ensure cross-platform compatibility
			fn(filepath.Join("kustomize", "kustomization.yaml"), &mockFileInfo{name: "kustomization.yaml", isDir: false}, nil)
			fn(filepath.Join("kustomize", "deployment.yaml"), &mockFileInfo{name: "deployment.yaml", isDir: false}, nil)
			fn(filepath.Join("kustomize", "base"), &mockFileInfo{name: "base", isDir: true}, nil)
			fn(filepath.Join("kustomize", "base", "service.yaml"), &mockFileInfo{name: "service.yaml", isDir: false}, nil)
			fn(filepath.Join("kustomize", "overlays"), &mockFileInfo{name: "overlays", isDir: true}, nil)
			fn(filepath.Join("kustomize", "overlays", "prod"), &mockFileInfo{name: "prod", isDir: true}, nil)
			fn(filepath.Join("kustomize", "overlays", "prod", "patch.yaml"), &mockFileInfo{name: "patch.yaml", isDir: false}, nil)
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			switch targpath {
			case filepath.Join("kustomize", "kustomization.yaml"):
				return "kustomization.yaml", nil
			case filepath.Join("kustomize", "deployment.yaml"):
				return "deployment.yaml", nil
			case filepath.Join("kustomize", "base", "service.yaml"):
				return filepath.Join("base", "service.yaml"), nil
			case filepath.Join("kustomize", "overlays", "prod", "patch.yaml"):
				return filepath.Join("overlays", "prod", "patch.yaml"), nil
			default:
				return "", fmt.Errorf("unexpected path: %s", targpath)
			}
		}

		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			switch filename {
			case filepath.Join("kustomize", "kustomization.yaml"):
				return []byte("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization"), nil
			case filepath.Join("kustomize", "deployment.yaml"):
				return []byte("apiVersion: apps/v1\nkind: Deployment"), nil
			case filepath.Join("kustomize", "base", "service.yaml"):
				return []byte("apiVersion: v1\nkind: Service"), nil
			case filepath.Join("kustomize", "overlays", "prod", "patch.yaml"):
				return []byte("- op: replace\n  path: /spec/replicas\n  value: 3"), nil
			default:
				return nil, fmt.Errorf("unexpected file: %s", filename)
			}
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And files should be added with correct paths
		expectedFiles := map[string]string{
			"kustomize/kustomization.yaml":       "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization",
			"kustomize/deployment.yaml":          "apiVersion: apps/v1\nkind: Deployment",
			"kustomize/base/service.yaml":        "apiVersion: v1\nkind: Service",
			"kustomize/overlays/prod/patch.yaml": "- op: replace\n  path: /spec/replicas\n  value: 3",
		}

		for expectedPath, expectedContent := range expectedFiles {
			if content, exists := filesAdded[expectedPath]; !exists {
				t.Errorf("Expected file %s to be added", expectedPath)
			} else if string(content) != expectedContent {
				t.Errorf("Expected content %q for %s, got %q", expectedContent, expectedPath, string(content))
			}
		}

		// And directories should be skipped (only 4 files should be added)
		if len(filesAdded) != 4 {
			t.Errorf("Expected 4 files to be added, got %d", len(filesAdded))
		}
	})

	t.Run("ErrorWhenKustomizeDirectoryNotFound", func(t *testing.T) {
		// Given a kustomize bundler with missing kustomize directory
		bundler, mocks := setup(t)
		bundler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "kustomize" {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{name: name, isDir: true}, nil
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when kustomize directory not found")
		}
		expectedMsg := "kustomize directory not found: kustomize"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("ErrorWhenWalkFails", func(t *testing.T) {
		// Given a kustomize bundler with failing filesystem walk
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fmt.Errorf("permission denied")
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then the walk error should be returned
		if err == nil {
			t.Error("Expected error when walk fails")
		}
		if err.Error() != "permission denied" {
			t.Errorf("Expected walk error, got: %v", err)
		}
	})

	t.Run("ErrorWhenWalkCallbackFails", func(t *testing.T) {
		// Given a kustomize bundler with walk callback returning error
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate walk callback being called with an error
			return fn(filepath.Join("kustomize", "test.yaml"), &mockFileInfo{name: "test.yaml", isDir: false}, fmt.Errorf("callback error"))
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then the callback error should be returned
		if err == nil {
			t.Error("Expected error when walk callback fails")
		}
		if err.Error() != "callback error" {
			t.Errorf("Expected callback error, got: %v", err)
		}
	})

	t.Run("ErrorWhenFilepathRelFails", func(t *testing.T) {
		// Given a kustomize bundler with failing relative path calculation
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fn(filepath.Join("kustomize", "test.yaml"), &mockFileInfo{name: "test.yaml", isDir: false}, nil)
		}
		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "", fmt.Errorf("relative path error")
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then the relative path error should be returned
		if err == nil {
			t.Error("Expected error when filepath rel fails")
		}
		expectedMsg := "failed to get relative path: relative path error"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("ErrorWhenReadFileFails", func(t *testing.T) {
		// Given a kustomize bundler with failing file read
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fn(filepath.Join("kustomize", "test.yaml"), &mockFileInfo{name: "test.yaml", isDir: false}, nil)
		}
		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "test.yaml", nil
		}
		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("read permission denied")
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then the read error should be returned
		if err == nil {
			t.Error("Expected error when read file fails")
		}
		expectedMsg := "failed to read kustomize file " + filepath.Join("kustomize", "test.yaml") + ": read permission denied"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("ErrorWhenArtifactAddFileFails", func(t *testing.T) {
		// Given a kustomize bundler with failing artifact add file
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fn(filepath.Join("kustomize", "test.yaml"), &mockFileInfo{name: "test.yaml", isDir: false}, nil)
		}
		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "test.yaml", nil
		}
		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			return []byte("content"), nil
		}
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			return fmt.Errorf("artifact storage full")
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then the add file error should be returned
		if err == nil {
			t.Error("Expected error when artifact add file fails")
		}
		if err.Error() != "artifact storage full" {
			t.Errorf("Expected add file error, got: %v", err)
		}
	})

	t.Run("SkipsDirectoriesInWalk", func(t *testing.T) {
		// Given a kustomize bundler with mix of files and directories
		bundler, mocks := setup(t)

		filesAdded := make([]string, 0)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded = append(filesAdded, path)
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Mix of directories and files
			fn(filepath.Join("kustomize", "base"), &mockFileInfo{name: "base", isDir: true}, nil)
			fn(filepath.Join("kustomize", "kustomization.yaml"), &mockFileInfo{name: "kustomization.yaml", isDir: false}, nil)
			fn(filepath.Join("kustomize", "overlays"), &mockFileInfo{name: "overlays", isDir: true}, nil)
			fn(filepath.Join("kustomize", "deployment.yaml"), &mockFileInfo{name: "deployment.yaml", isDir: false}, nil)
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			if targpath == filepath.Join("kustomize", "kustomization.yaml") {
				return "kustomization.yaml", nil
			}
			if targpath == filepath.Join("kustomize", "deployment.yaml") {
				return "deployment.yaml", nil
			}
			return "", nil
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And only files should be added (not directories)
		expectedFiles := []string{"kustomize/kustomization.yaml", "kustomize/deployment.yaml"}
		if len(filesAdded) != len(expectedFiles) {
			t.Errorf("Expected %d files added, got %d", len(expectedFiles), len(filesAdded))
		}

		for i, expected := range expectedFiles {
			if i < len(filesAdded) && filesAdded[i] != expected {
				t.Errorf("Expected file %s at index %d, got %s", expected, i, filesAdded[i])
			}
		}
	})

	t.Run("HandlesEmptyKustomizeDirectory", func(t *testing.T) {
		// Given a kustomize bundler with empty kustomize directory
		bundler, mocks := setup(t)

		filesAdded := make([]string, 0)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded = append(filesAdded, path)
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// No files found in directory
			return nil
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And no files should be added
		if len(filesAdded) != 0 {
			t.Errorf("Expected 0 files added, got %d", len(filesAdded))
		}
	})
}
