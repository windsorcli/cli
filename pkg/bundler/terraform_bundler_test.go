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
		filesAdded := make(map[string][]byte)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded[path] = content
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			fn("terraform", &mockFileInfo{name: "terraform", isDir: true}, nil)
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
			default:
				return "", fmt.Errorf("unexpected path (should have been filtered): %s", targpath)
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
			default:
				return nil, fmt.Errorf("unexpected file should not be read (should have been filtered): %s", filename)
			}
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And files should be added with correct paths (excluding .tfvars files for security)
		expectedFiles := map[string]string{
			"terraform/main.tf":             "resource \"aws_instance\" \"example\" {\n  ami = \"ami-12345\"\n}",
			"terraform/variables.tf":        "variable \"instance_type\" {\n  type = string\n  default = \"t2.micro\"\n}",
			"terraform/outputs.tf":          "output \"instance_id\" {\n  value = aws_instance.example.id\n}",
			"terraform/modules/vpc/main.tf": "resource \"aws_vpc\" \"main\" {\n  cidr_block = \"10.0.0.0/16\"\n}",
		}

		for expectedPath, expectedContent := range expectedFiles {
			if content, exists := filesAdded[expectedPath]; !exists {
				t.Errorf("Expected file %s to be added", expectedPath)
			} else if string(content) != expectedContent {
				t.Errorf("Expected content %q for %s, got %q", expectedContent, expectedPath, string(content))
			}
		}

		// And directories should be skipped (only 4 files should be added, .tfvars files are filtered out)
		if len(filesAdded) != 4 {
			t.Errorf("Expected 4 files to be added, got %d", len(filesAdded))
		}
	})

	t.Run("SkipsTerraformDirectoriesAndOverrideFiles", func(t *testing.T) {
		// Given a terraform bundler with .terraform directories and override files
		bundler, mocks := setup(t)
		filesAdded := make(map[string][]byte)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded[path] = content
			return nil
		}

		walkCallCount := 0
		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			walkCallCount++

			// Simulate directory traversal with .terraform directories and override files
			if err := fn("terraform", &mockFileInfo{name: "terraform", isDir: true}, nil); err != nil {
				return err
			}
			if err := fn(filepath.Join("terraform", "main.tf"), &mockFileInfo{name: "main.tf", isDir: false}, nil); err != nil {
				return err
			}
			if err := fn(filepath.Join("terraform", "backend_override.tf"), &mockFileInfo{name: "backend_override.tf", isDir: false}, nil); err != nil {
				return err
			}
			if err := fn(filepath.Join("terraform", "local_override.tf"), &mockFileInfo{name: "local_override.tf", isDir: false}, nil); err != nil {
				return err
			}
			// Test .terraform directory - this should be skipped
			if err := fn(filepath.Join("terraform", ".terraform"), &mockFileInfo{name: ".terraform", isDir: true}, nil); err != nil {
				if err == filepath.SkipDir {
					// This is expected behavior - continue with the rest of the files
				} else {
					return err
				}
			}
			if err := fn(filepath.Join("terraform", "variables.tf"), &mockFileInfo{name: "variables.tf", isDir: false}, nil); err != nil {
				return err
			}
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			switch targpath {
			case filepath.Join("terraform", "main.tf"):
				return "main.tf", nil
			case filepath.Join("terraform", "variables.tf"):
				return "variables.tf", nil
			case filepath.Join("terraform", "backend_override.tf"):
				return "backend_override.tf", nil
			case filepath.Join("terraform", "local_override.tf"):
				return "local_override.tf", nil
			default:
				return "", fmt.Errorf("unexpected path: %s", targpath)
			}
		}

		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			switch filename {
			case filepath.Join("terraform", "main.tf"):
				return []byte("resource \"aws_instance\" \"example\" {}"), nil
			case filepath.Join("terraform", "variables.tf"):
				return []byte("variable \"instance_type\" {}"), nil
			default:
				return nil, fmt.Errorf("unexpected file should not be read: %s", filename)
			}
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And only non-override files should be added (override files should be skipped)
		expectedFiles := map[string]string{
			"terraform/main.tf":      "resource \"aws_instance\" \"example\" {}",
			"terraform/variables.tf": "variable \"instance_type\" {}",
		}

		for expectedPath, expectedContent := range expectedFiles {
			if content, exists := filesAdded[expectedPath]; !exists {
				t.Errorf("Expected file %s to be added", expectedPath)
			} else if string(content) != expectedContent {
				t.Errorf("Expected content %q for %s, got %q", expectedContent, expectedPath, string(content))
			}
		}

		// And override files should not be included
		overrideFiles := []string{
			"terraform/backend_override.tf",
			"terraform/local_override.tf",
		}
		for _, overrideFile := range overrideFiles {
			if _, exists := filesAdded[overrideFile]; exists {
				t.Errorf("Override file %s should not be added", overrideFile)
			}
		}

		// And only the expected files should be added (2 files)
		if len(filesAdded) != 2 {
			t.Errorf("Expected 2 files to be added, got %d", len(filesAdded))
			for path := range filesAdded {
				t.Logf("Added file: %s", path)
			}
		}
	})

	t.Run("SkipsTerraformDirectoryCompletely", func(t *testing.T) {
		// Given a terraform bundler with .terraform directory containing files
		bundler, mocks := setup(t)
		filesAdded := make(map[string][]byte)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded[path] = content
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate directory traversal
			if err := fn("terraform", &mockFileInfo{name: "terraform", isDir: true}, nil); err != nil {
				return err
			}
			if err := fn(filepath.Join("terraform", "main.tf"), &mockFileInfo{name: "main.tf", isDir: false}, nil); err != nil {
				return err
			}
			// Test .terraform directory - should return SkipDir to skip entire directory
			if err := fn(filepath.Join("terraform", ".terraform"), &mockFileInfo{name: ".terraform", isDir: true}, nil); err != nil {
				if err == filepath.SkipDir {
					// .terraform directory should be skipped completely, don't traverse its contents
					return nil
				}
				return err
			}
			// These files should NOT be called because .terraform directory should be skipped
			// If they are called, the test should fail
			if err := fn(filepath.Join("terraform", ".terraform", "providers"), &mockFileInfo{name: "providers", isDir: true}, nil); err != nil {
				return err
			}
			if err := fn(filepath.Join("terraform", ".terraform", "terraform.tfstate"), &mockFileInfo{name: "terraform.tfstate", isDir: false}, nil); err != nil {
				return err
			}
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			switch targpath {
			case filepath.Join("terraform", "main.tf"):
				return "main.tf", nil
			default:
				return "", fmt.Errorf("unexpected path: %s", targpath)
			}
		}

		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			switch filename {
			case filepath.Join("terraform", "main.tf"):
				return []byte("resource \"aws_instance\" \"example\" {}"), nil
			default:
				return nil, fmt.Errorf("unexpected file should not be read: %s", filename)
			}
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And only main.tf should be added (.terraform directory should be completely skipped)
		expectedFiles := map[string]string{
			"terraform/main.tf": "resource \"aws_instance\" \"example\" {}",
		}

		for expectedPath, expectedContent := range expectedFiles {
			if content, exists := filesAdded[expectedPath]; !exists {
				t.Errorf("Expected file %s to be added", expectedPath)
			} else if string(content) != expectedContent {
				t.Errorf("Expected content %q for %s, got %q", expectedContent, expectedPath, string(content))
			}
		}

		// And only 1 file should be added
		if len(filesAdded) != 1 {
			t.Errorf("Expected 1 file to be added, got %d", len(filesAdded))
			for path := range filesAdded {
				t.Logf("Added file: %s", path)
			}
		}
	})

	t.Run("FiltersCommonTerraformIgnorePatterns", func(t *testing.T) {
		// Given a terraform bundler with various terraform files including ones that should be filtered
		bundler, mocks := setup(t)
		filesAdded := make(map[string][]byte)
		mocks.Artifact.AddFileFunc = func(path string, content []byte) error {
			filesAdded[path] = content
			return nil
		}

		bundler.shims.Walk = func(root string, fn filepath.WalkFunc) error {
			// Simulate walking through terraform directory with various file types
			files := []struct {
				path   string
				name   string
				isDir  bool
				should string // "include" or "exclude"
			}{
				{"terraform", "terraform", true, "skip-dir"},
				{"terraform/main.tf", "main.tf", false, "include"},
				{"terraform/variables.tf", "variables.tf", false, "include"},
				{"terraform/outputs.tf", "outputs.tf", false, "include"},
				{"terraform/.terraform.lock.hcl", ".terraform.lock.hcl", false, "include"}, // Lock files should be included!
				{"terraform/terraform.tfvars", "terraform.tfvars", false, "exclude"},
				{"terraform/prod.tfvars", "prod.tfvars", false, "exclude"},
				{"terraform/secrets.tfvars.json", "secrets.tfvars.json", false, "exclude"},
				{"terraform/terraform.tfstate", "terraform.tfstate", false, "exclude"},
				{"terraform/terraform.tfstate.backup", "terraform.tfstate.backup", false, "exclude"},
				{"terraform/prod.tfstate", "prod.tfstate", false, "exclude"},
				{"terraform/plan.tfplan", "plan.tfplan", false, "exclude"},
				{"terraform/terraform.tfplan", "terraform.tfplan", false, "exclude"},
				{"terraform/backend_override.tf", "backend_override.tf", false, "exclude"},
				{"terraform/local_override.tf", "local_override.tf", false, "exclude"},
				{"terraform/override.tf", "override.tf", false, "exclude"},
				{"terraform/override.tf.json", "override.tf.json", false, "exclude"},
				{"terraform/test_override.tf.json", "test_override.tf.json", false, "exclude"},
				{"terraform/.terraformrc", ".terraformrc", false, "exclude"},
				{"terraform/terraform.rc", "terraform.rc", false, "exclude"},
				{"terraform/crash.log", "crash.log", false, "exclude"},
				{"terraform/crash.20241205.log", "crash.20241205.log", false, "exclude"},
				{"terraform/modules", "modules", true, "skip-dir"},
				{"terraform/modules/vpc", "vpc", true, "skip-dir"},
				{"terraform/modules/vpc/main.tf", "main.tf", false, "include"},
			}

			for _, file := range files {
				err := fn(file.path, &mockFileInfo{name: file.name, isDir: file.isDir}, nil)
				if err != nil && err != filepath.SkipDir {
					return err
				}
			}
			return nil
		}

		bundler.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			// Return relative path for files that should be included
			includeFiles := map[string]string{
				filepath.Join("terraform", "main.tf"):                   "main.tf",
				filepath.Join("terraform", "variables.tf"):              "variables.tf",
				filepath.Join("terraform", "outputs.tf"):                "outputs.tf",
				filepath.Join("terraform", ".terraform.lock.hcl"):       ".terraform.lock.hcl",
				filepath.Join("terraform", "modules", "vpc", "main.tf"): "modules/vpc/main.tf",
			}
			if relPath, exists := includeFiles[targpath]; exists {
				return relPath, nil
			}
			return "", fmt.Errorf("unexpected path (should have been filtered): %s", targpath)
		}

		bundler.shims.ReadFile = func(filename string) ([]byte, error) {
			// Return content for files that should be included
			contentMap := map[string]string{
				filepath.Join("terraform", "main.tf"):                   "resource \"aws_instance\" \"example\" {}",
				filepath.Join("terraform", "variables.tf"):              "variable \"instance_type\" {}",
				filepath.Join("terraform", "outputs.tf"):                "output \"instance_id\" {}",
				filepath.Join("terraform", ".terraform.lock.hcl"):       "# Lock file content",
				filepath.Join("terraform", "modules", "vpc", "main.tf"): "module vpc content",
			}
			if content, exists := contentMap[filename]; exists {
				return []byte(content), nil
			}
			return nil, fmt.Errorf("unexpected file should not be read (should have been filtered): %s", filename)
		}

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And only files that should be included are added
		expectedFiles := map[string]string{
			"terraform/main.tf":             "resource \"aws_instance\" \"example\" {}",
			"terraform/variables.tf":        "variable \"instance_type\" {}",
			"terraform/outputs.tf":          "output \"instance_id\" {}",
			"terraform/.terraform.lock.hcl": "# Lock file content",
			"terraform/modules/vpc/main.tf": "module vpc content",
		}

		for expectedPath, expectedContent := range expectedFiles {
			if content, exists := filesAdded[expectedPath]; !exists {
				t.Errorf("Expected file %s to be added", expectedPath)
			} else if string(content) != expectedContent {
				t.Errorf("Expected content %q for %s, got %q", expectedContent, expectedPath, string(content))
			}
		}

		// And no unwanted files should be included
		if len(filesAdded) != len(expectedFiles) {
			t.Errorf("Expected %d files to be added, got %d", len(expectedFiles), len(filesAdded))
			for path := range filesAdded {
				t.Logf("Added file: %s", path)
			}
		}

		// Verify specific files are NOT included
		excludedFiles := []string{
			"terraform/terraform.tfvars",
			"terraform/prod.tfvars",
			"terraform/secrets.tfvars.json",
			"terraform/terraform.tfstate",
			"terraform/terraform.tfstate.backup",
			"terraform/plan.tfplan",
			"terraform/backend_override.tf",
			"terraform/override.tf",
			"terraform/.terraformrc",
			"terraform/crash.log",
		}

		for _, excludedFile := range excludedFiles {
			if _, exists := filesAdded[excludedFile]; exists {
				t.Errorf("File %s should have been excluded but was included", excludedFile)
			}
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
