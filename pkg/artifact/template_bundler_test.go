package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test TemplateBundler
// =============================================================================

func TestTemplateBundler_NewTemplateBundler(t *testing.T) {
	setup := func(t *testing.T) *TemplateBundler {
		t.Helper()
		return NewTemplateBundler()
	}

	t.Run("CreatesInstanceWithBaseBundler", func(t *testing.T) {
		// Given no preconditions
		// When creating a new template bundler
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

func TestTemplateBundler_Bundle(t *testing.T) {
	setup := func(t *testing.T) (*TemplateBundler, *BundlerMocks) {
		t.Helper()
		mocks := setupBundlerMocks(t)
		bundler := NewTemplateBundler()
		bundler.shims = mocks.Shims
		bundler.Initialize(mocks.Injector)
		return bundler, mocks
	}

	t.Run("SuccessWithValidTemplateFiles", func(t *testing.T) {
		// Given a template bundler with valid template files
		bundler, mocks := setup(t)

		// Set up mocks to simulate finding template files
		filesAdded := make(map[string][]byte)
		mocks.Artifact.AddFileFunc = func(path string, content []byte, mode os.FileMode) error {
			filesAdded[path] = content
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate finding multiple files in templates directory
			// Use filepath.Join to ensure cross-platform compatibility
			templatesDir := filepath.Join("contexts", "_template")
			fn(filepath.Join(templatesDir, "metadata.yaml"), &mockFileInfo{name: "metadata.yaml", isDir: false}, nil)
			fn(filepath.Join(templatesDir, "template.jsonnet"), &mockFileInfo{name: "template.jsonnet", isDir: false}, nil)
			fn(filepath.Join(templatesDir, "subdir"), &mockFileInfo{name: "subdir", isDir: true}, nil)
			fn(filepath.Join(templatesDir, "subdir", "nested.yaml"), &mockFileInfo{name: "nested.yaml", isDir: false}, nil)
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			templatesDir := filepath.Join("contexts", "_template")
			switch targpath {
			case filepath.Join(templatesDir, "metadata.yaml"):
				return "metadata.yaml", nil
			case filepath.Join(templatesDir, "template.jsonnet"):
				return "template.jsonnet", nil
			case filepath.Join(templatesDir, "subdir", "nested.yaml"):
				return filepath.Join("subdir", "nested.yaml"), nil
			default:
				return "", fmt.Errorf("unexpected path: %s", targpath)
			}
		}

		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			templatesDir := filepath.Join("contexts", "_template")
			switch filename {
			case filepath.Join(templatesDir, "metadata.yaml"):
				return []byte("name: test\nversion: v1.0.0"), nil
			case filepath.Join(templatesDir, "template.jsonnet"):
				return []byte("local test = 'value';"), nil
			case filepath.Join(templatesDir, "subdir", "nested.yaml"):
				return []byte("nested: content"), nil
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
			"_template/metadata.yaml":      "name: test\nversion: v1.0.0",
			"_template/template.jsonnet":   "local test = 'value';",
			"_template/subdir/nested.yaml": "nested: content",
		}

		for expectedPath, expectedContent := range expectedFiles {
			if content, exists := filesAdded[expectedPath]; !exists {
				t.Errorf("Expected file %s to be added", expectedPath)
			} else if string(content) != expectedContent {
				t.Errorf("Expected content %q for %s, got %q", expectedContent, expectedPath, string(content))
			}
		}

		// And directories should be skipped (only 3 files should be added)
		if len(filesAdded) != 3 {
			t.Errorf("Expected 3 files to be added, got %d", len(filesAdded))
		}
	})

	t.Run("ErrorWhenTemplatesDirectoryNotFound", func(t *testing.T) {
		// Given a template bundler with missing templates directory
		bundler, mocks := setup(t)
		bundler.shims.Stat = func(name string) (os.FileInfo, error) {
			templatesDir := filepath.Join("contexts", "_template")
			if name == templatesDir {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{name: name, isDir: true}, nil
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when templates directory not found")
		}
		expectedMsg := "templates directory not found: " + filepath.Join("contexts", "_template")
		if err.Error() != expectedMsg {
			t.Errorf("Expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("ErrorWhenWalkFails", func(t *testing.T) {
		// Given a template bundler with failing filesystem walk
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
		// Given a template bundler with walk callback returning error
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate walk callback being called with an error
			templatesDir := filepath.Join("contexts", "_template")
			return fn(filepath.Join(templatesDir, "test.txt"), &mockFileInfo{name: "test.txt", isDir: false}, fmt.Errorf("callback error"))
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
		// Given a template bundler with failing relative path calculation
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			templatesDir := filepath.Join("contexts", "_template")
			return fn(filepath.Join(templatesDir, "test.txt"), &mockFileInfo{name: "test.txt", isDir: false}, nil)
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
		// Given a template bundler with failing file read
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			templatesDir := filepath.Join("contexts", "_template")
			return fn(filepath.Join(templatesDir, "test.txt"), &mockFileInfo{name: "test.txt", isDir: false}, nil)
		}
		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "test.txt", nil
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
		expectedMsg := "failed to read template file " + filepath.Join("contexts", "_template", "test.txt") + ": read permission denied"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("ErrorWhenArtifactAddFileFails", func(t *testing.T) {
		// Given a template bundler with failing artifact add file
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			templatesDir := filepath.Join("contexts", "_template")
			return fn(filepath.Join(templatesDir, "test.txt"), &mockFileInfo{name: "test.txt", isDir: false}, nil)
		}
		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "test.txt", nil
		}
		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			return []byte("content"), nil
		}
		mocks.Artifact.AddFileFunc = func(path string, content []byte, mode os.FileMode) error {
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
		// Given a template bundler with mix of files and directories
		bundler, mocks := setup(t)

		filesAdded := make([]string, 0)
		mocks.Artifact.AddFileFunc = func(path string, content []byte, mode os.FileMode) error {
			filesAdded = append(filesAdded, path)
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Mix of directories and files
			templatesDir := filepath.Join("contexts", "_template")
			fn(filepath.Join(templatesDir, "dir1"), &mockFileInfo{name: "dir1", isDir: true}, nil)
			fn(filepath.Join(templatesDir, "file1.txt"), &mockFileInfo{name: "file1.txt", isDir: false}, nil)
			fn(filepath.Join(templatesDir, "dir2"), &mockFileInfo{name: "dir2", isDir: true}, nil)
			fn(filepath.Join(templatesDir, "file2.yaml"), &mockFileInfo{name: "file2.yaml", isDir: false}, nil)
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			templatesDir := filepath.Join("contexts", "_template")
			if targpath == filepath.Join(templatesDir, "file1.txt") {
				return "file1.txt", nil
			}
			if targpath == filepath.Join(templatesDir, "file2.yaml") {
				return "file2.yaml", nil
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
		expectedFiles := []string{"_template/file1.txt", "_template/file2.yaml"}
		if len(filesAdded) != len(expectedFiles) {
			t.Errorf("Expected %d files added, got %d", len(expectedFiles), len(filesAdded))
		}

		for i, expected := range expectedFiles {
			if i < len(filesAdded) && filesAdded[i] != expected {
				t.Errorf("Expected file %s at index %d, got %s", expected, i, filesAdded[i])
			}
		}
	})
}
