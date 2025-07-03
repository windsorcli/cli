package bundler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test TerraformBundler
// =============================================================================

func TestTerraformBundler_NewTerraformBundler(t *testing.T) {
	setup := func(t *testing.T) *TerraformBundler {
		t.Helper()
		return NewTerraformBundler()
	}

	t.Run("CreatesInstanceWithBaseBundler", func(t *testing.T) {
		// Given no preconditions
		// When creating a new terraform bundler
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

func TestTerraformBundler_Bundle(t *testing.T) {
	setup := func(t *testing.T) (*TerraformBundler, *BundlerMocks) {
		t.Helper()
		mocks := setupBundlerMocks(t)
		bundler := NewTerraformBundler()
		bundler.shims = mocks.Shims
		bundler.Initialize(mocks.Injector)
		return bundler, mocks
	}

	t.Run("SuccessWithValidTerraformFiles", func(t *testing.T) {
		// Given a terraform bundler with valid terraform files
		bundler, mocks := setup(t)

		// Set up mocks to simulate finding terraform files
		filesAdded := make(map[string][]byte)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded[path] = content
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate finding multiple files in terraform directory
			// Use filepath.Join to ensure cross-platform compatibility
			fn(filepath.Join("terraform", "main.tf"), &mockFileInfo{name: "main.tf", isDir: false}, nil)
			fn(filepath.Join("terraform", "variables.tf"), &mockFileInfo{name: "variables.tf", isDir: false}, nil)
			fn(filepath.Join("terraform", "outputs.tf"), &mockFileInfo{name: "outputs.tf", isDir: false}, nil)
			fn(filepath.Join("terraform", "modules"), &mockFileInfo{name: "modules", isDir: true}, nil)
			fn(filepath.Join("terraform", "modules", "vpc"), &mockFileInfo{name: "vpc", isDir: true}, nil)
			fn(filepath.Join("terraform", "modules", "vpc", "main.tf"), &mockFileInfo{name: "main.tf", isDir: false}, nil)
			fn(filepath.Join("terraform", "environments"), &mockFileInfo{name: "environments", isDir: true}, nil)
			fn(filepath.Join("terraform", "environments", "prod"), &mockFileInfo{name: "prod", isDir: true}, nil)
			fn(filepath.Join("terraform", "environments", "prod", "terraform.tfvars"), &mockFileInfo{name: "terraform.tfvars", isDir: false}, nil)
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			switch targpath {
			case filepath.Join("terraform", "main.tf"):
				return "main.tf", nil
			case filepath.Join("terraform", "variables.tf"):
				return "variables.tf", nil
			case filepath.Join("terraform", "outputs.tf"):
				return "outputs.tf", nil
			case filepath.Join("terraform", "modules", "vpc", "main.tf"):
				return filepath.Join("modules", "vpc", "main.tf"), nil
			case filepath.Join("terraform", "environments", "prod", "terraform.tfvars"):
				return filepath.Join("environments", "prod", "terraform.tfvars"), nil
			default:
				return "", fmt.Errorf("unexpected path: %s", targpath)
			}
		}

		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			switch filename {
			case filepath.Join("terraform", "main.tf"):
				return []byte("resource \"aws_instance\" \"example\" {\n  ami = \"ami-12345\"\n}"), nil
			case filepath.Join("terraform", "variables.tf"):
				return []byte("variable \"instance_type\" {\n  type = string\n  default = \"t2.micro\"\n}"), nil
			case filepath.Join("terraform", "outputs.tf"):
				return []byte("output \"instance_id\" {\n  value = aws_instance.example.id\n}"), nil
			case filepath.Join("terraform", "modules", "vpc", "main.tf"):
				return []byte("resource \"aws_vpc\" \"main\" {\n  cidr_block = \"10.0.0.0/16\"\n}"), nil
			case filepath.Join("terraform", "environments", "prod", "terraform.tfvars"):
				return []byte("instance_type = \"t3.medium\"\nregion = \"us-west-2\""), nil
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
			"terraform/main.tf":                            "resource \"aws_instance\" \"example\" {\n  ami = \"ami-12345\"\n}",
			"terraform/variables.tf":                       "variable \"instance_type\" {\n  type = string\n  default = \"t2.micro\"\n}",
			"terraform/outputs.tf":                         "output \"instance_id\" {\n  value = aws_instance.example.id\n}",
			"terraform/modules/vpc/main.tf":                "resource \"aws_vpc\" \"main\" {\n  cidr_block = \"10.0.0.0/16\"\n}",
			"terraform/environments/prod/terraform.tfvars": "instance_type = \"t3.medium\"\nregion = \"us-west-2\"",
		}

		for expectedPath, expectedContent := range expectedFiles {
			if content, exists := filesAdded[expectedPath]; !exists {
				t.Errorf("Expected file %s to be added", expectedPath)
			} else if string(content) != expectedContent {
				t.Errorf("Expected content %q for %s, got %q", expectedContent, expectedPath, string(content))
			}
		}

		// And directories should be skipped (only 5 files should be added)
		if len(filesAdded) != 5 {
			t.Errorf("Expected 5 files to be added, got %d", len(filesAdded))
		}
	})

	t.Run("HandlesWhenTerraformDirectoryNotFound", func(t *testing.T) {
		// Given a terraform bundler with missing terraform directory
		bundler, mocks := setup(t)
		bundler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "terraform" {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{name: name, isDir: true}, nil
		}

		filesAdded := make([]string, 0)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded = append(filesAdded, path)
			return nil
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned (graceful handling)
		if err != nil {
			t.Errorf("Expected nil error when terraform directory not found, got %v", err)
		}

		// And no files should be added
		if len(filesAdded) != 0 {
			t.Errorf("Expected 0 files added when directory not found, got %d", len(filesAdded))
		}
	})

	t.Run("ErrorWhenWalkFails", func(t *testing.T) {
		// Given a terraform bundler with failing filesystem walk
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
		// Given a terraform bundler with walk callback returning error
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate walk callback being called with an error
			return fn(filepath.Join("terraform", "test.tf"), &mockFileInfo{name: "test.tf", isDir: false}, fmt.Errorf("callback error"))
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
		// Given a terraform bundler with failing relative path calculation
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fn(filepath.Join("terraform", "test.tf"), &mockFileInfo{name: "test.tf", isDir: false}, nil)
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
		// Given a terraform bundler with failing file read
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fn(filepath.Join("terraform", "test.tf"), &mockFileInfo{name: "test.tf", isDir: false}, nil)
		}
		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "test.tf", nil
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
		expectedMsg := "failed to read terraform file " + filepath.Join("terraform", "test.tf") + ": read permission denied"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("ErrorWhenArtifactAddFileFails", func(t *testing.T) {
		// Given a terraform bundler with failing artifact add file
		bundler, mocks := setup(t)
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fn(filepath.Join("terraform", "test.tf"), &mockFileInfo{name: "test.tf", isDir: false}, nil)
		}
		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "test.tf", nil
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
		// Given a terraform bundler with mix of files and directories
		bundler, mocks := setup(t)

		filesAdded := make([]string, 0)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded = append(filesAdded, path)
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Mix of directories and files
			fn(filepath.Join("terraform", "modules"), &mockFileInfo{name: "modules", isDir: true}, nil)
			fn(filepath.Join("terraform", "main.tf"), &mockFileInfo{name: "main.tf", isDir: false}, nil)
			fn(filepath.Join("terraform", "environments"), &mockFileInfo{name: "environments", isDir: true}, nil)
			fn(filepath.Join("terraform", "variables.tf"), &mockFileInfo{name: "variables.tf", isDir: false}, nil)
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			if targpath == filepath.Join("terraform", "main.tf") {
				return "main.tf", nil
			}
			if targpath == filepath.Join("terraform", "variables.tf") {
				return "variables.tf", nil
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
		expectedFiles := []string{"terraform/main.tf", "terraform/variables.tf"}
		if len(filesAdded) != len(expectedFiles) {
			t.Errorf("Expected %d files added, got %d", len(expectedFiles), len(filesAdded))
		}

		for i, expected := range expectedFiles {
			if i < len(filesAdded) && filesAdded[i] != expected {
				t.Errorf("Expected file %s at index %d, got %s", expected, i, filesAdded[i])
			}
		}
	})

	t.Run("HandlesEmptyTerraformDirectory", func(t *testing.T) {
		// Given a terraform bundler with empty terraform directory
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
