package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestStandardModuleResolver_processLocalComponent(t *testing.T) {
	setup := func(t *testing.T) (*StandardModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver, mocks
	}

	t.Run("ProcessesNamedLocalComponent", func(t *testing.T) {
		// Given a resolver with a named local component (no source, but has name)
		resolver, mocks := setup(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		actualModulePath := filepath.Join(tmpDir, "terraform", "cluster")
		shimPath := filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "cluster")

		if err := os.MkdirAll(actualModulePath, 0755); err != nil {
			t.Fatalf("Failed to create actual module directory: %v", err)
		}

		variablesTfPath := filepath.Join(actualModulePath, "variables.tf")
		if err := os.WriteFile(variablesTfPath, []byte(`variable "node_count" { type = number }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		outputsTfPath := filepath.Join(actualModulePath, "outputs.tf")
		if err := os.WriteFile(outputsTfPath, []byte(`output "cluster_id" { value = "test" }`), 0644); err != nil {
			t.Fatalf("Failed to write outputs.tf: %v", err)
		}

		component := blueprintv1alpha1.TerraformComponent{
			Name:     "cluster",
			Path:     "cluster",
			Source:   "",
			FullPath: shimPath,
		}

		var writtenFiles []string
		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenFiles = append(writtenFiles, path)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			return os.WriteFile(path, data, perm)
		}

		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return os.MkdirAll(path, perm)
		}

		resolver.BaseModuleResolver.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return filepath.Rel(basepath, targpath)
		}

		// When processLocalComponent is called
		err := resolver.processLocalComponent(component)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And shim files should be created
		expectedMainTf := filepath.Join(shimPath, "main.tf")
		expectedVariablesTf := filepath.Join(shimPath, "variables.tf")
		expectedOutputsTf := filepath.Join(shimPath, "outputs.tf")

		foundMainTf := false
		foundVariablesTf := false
		foundOutputsTf := false
		for _, file := range writtenFiles {
			if file == expectedMainTf {
				foundMainTf = true
			}
			if file == expectedVariablesTf {
				foundVariablesTf = true
			}
			if file == expectedOutputsTf {
				foundOutputsTf = true
			}
		}

		if !foundMainTf {
			t.Errorf("Expected main.tf to be written at %s", expectedMainTf)
		}
		if !foundVariablesTf {
			t.Errorf("Expected variables.tf to be written at %s", expectedVariablesTf)
		}
		if !foundOutputsTf {
			t.Errorf("Expected outputs.tf to be written at %s", expectedOutputsTf)
		}

		// And main.tf should reference the relative path to the actual module
		if foundMainTf {
			mainTfContent, err := os.ReadFile(expectedMainTf)
			if err != nil {
				t.Fatalf("Failed to read main.tf: %v", err)
			}
			if !strings.Contains(string(mainTfContent), "source") {
				t.Error("Expected main.tf to contain source reference")
			}
		}
	})

	t.Run("SkipsLocalComponentWithoutName", func(t *testing.T) {
		// Given a resolver with a local component without name
		resolver, _ := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			Source:   "",
			FullPath: "/mock/project/terraform/test-module",
		}

		// When processLocalComponent is called
		err := resolver.processLocalComponent(component)

		// Then no error is returned (component is skipped, no shims created)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesMkdirError", func(t *testing.T) {
		// Given a resolver with a named local component that fails to create directory
		resolver, mocks := setup(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		shimPath := filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "cluster")

		component := blueprintv1alpha1.TerraformComponent{
			Name:     "cluster",
			Path:     "cluster",
			Source:   "",
			FullPath: shimPath,
		}

		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		}

		// When processLocalComponent is called
		err := resolver.processLocalComponent(component)

		// Then an error is returned
		if err == nil {
			t.Error("Expected error when mkdir fails, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to create module directory") {
			t.Errorf("Expected error about creating module directory, got: %v", err)
		}
	})

	t.Run("HandlesFilepathRelError", func(t *testing.T) {
		// Given a resolver with a named local component that fails to calculate relative path
		resolver, mocks := setup(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		shimPath := filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "cluster")

		component := blueprintv1alpha1.TerraformComponent{
			Name:     "cluster",
			Path:     "cluster",
			Source:   "",
			FullPath: shimPath,
		}

		resolver.BaseModuleResolver.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "", errors.New("filepath rel error")
		}

		// When processLocalComponent is called
		err := resolver.processLocalComponent(component)

		// Then an error is returned
		if err == nil {
			t.Error("Expected error when filepath rel fails, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to calculate relative path") {
			t.Errorf("Expected error about calculating relative path, got: %v", err)
		}
	})

	t.Run("HandlesWriteMainTfError", func(t *testing.T) {
		// Given a resolver with a named local component that fails to write main.tf
		resolver, mocks := setup(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		shimPath := filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "cluster")

		component := blueprintv1alpha1.TerraformComponent{
			Name:     "cluster",
			Path:     "cluster",
			Source:   "",
			FullPath: shimPath,
		}

		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return os.MkdirAll(path, perm)
		}

		resolver.BaseModuleResolver.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return filepath.Rel(basepath, targpath)
		}

		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "main.tf") {
				return errors.New("write main.tf error")
			}
			return nil
		}

		// When processLocalComponent is called
		err := resolver.processLocalComponent(component)

		// Then an error is returned
		if err == nil {
			t.Error("Expected error when writing main.tf fails, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to write main.tf") {
			t.Errorf("Expected error about writing main.tf, got: %v", err)
		}
	})
}

func TestStandardModuleResolver_shouldHandle(t *testing.T) {
	setup := func(t *testing.T) *StandardModuleResolver {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.shims = mocks.Shims
		return resolver
	}

	t.Run("HandlesLocalPaths", func(t *testing.T) {
		// Given a standard module resolver
		resolver := setup(t)

		// When checking if it should handle local paths
		// Then it should handle relative local module paths
		if !resolver.shouldHandle("./local/module") {
			t.Error("Expected ./local/module to be handled")
		}
		if !resolver.shouldHandle("../parent/module") {
			t.Error("Expected ../parent/module to be handled")
		}
	})

	t.Run("HandlesTerraformRegistryModules", func(t *testing.T) {
		// Given a standard module resolver
		resolver := setup(t)

		// When checking if it should handle terraform registry modules
		// Then it should handle registry module sources
		if !resolver.shouldHandle("terraform-aws-modules/vpc/aws") {
			t.Error("Expected terraform-aws-modules/vpc/aws to be handled")
		}
		if !resolver.shouldHandle("hashicorp/consul/aws") {
			t.Error("Expected hashicorp/consul/aws to be handled")
		}
	})

	t.Run("HandlesGitSources", func(t *testing.T) {
		// Given a standard module resolver
		resolver := setup(t)

		// When checking if it should handle git sources
		// Then it should handle various git source formats
		if !resolver.shouldHandle("github.com/terraform-aws-modules/terraform-aws-vpc") {
			t.Error("Expected github.com/terraform-aws-modules/terraform-aws-vpc to be handled")
		}
		if !resolver.shouldHandle("git@github.com:terraform-aws-modules/terraform-aws-vpc.git") {
			t.Error("Expected git@github.com:terraform-aws-modules/terraform-aws-vpc.git to be handled")
		}
		if !resolver.shouldHandle("git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git") {
			t.Error("Expected git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git to be handled")
		}
	})

	t.Run("HandlesHTTPSources", func(t *testing.T) {
		// Given a standard module resolver
		resolver := setup(t)

		// When checking if it should handle HTTP(S) sources
		// Then it should handle HTTP and HTTPS module sources
		if !resolver.shouldHandle("https://github.com/terraform-aws-modules/terraform-aws-vpc") {
			t.Error("Expected https://github.com/terraform-aws-modules/terraform-aws-vpc to be handled")
		}
		if !resolver.shouldHandle("http://example.com/module.zip") {
			t.Error("Expected http://example.com/module.zip to be handled")
		}
	})

	t.Run("HandlesCloudStorageSources", func(t *testing.T) {
		// Given a standard module resolver
		resolver := setup(t)

		// When checking if it should handle cloud storage sources
		// Then it should handle S3 and GCS module sources
		if !resolver.shouldHandle("s3::https://s3.amazonaws.com/bucket/module.zip") {
			t.Error("Expected s3::https://s3.amazonaws.com/bucket/module.zip to be handled")
		}
		if !resolver.shouldHandle("gcs::https://storage.googleapis.com/bucket/module.zip") {
			t.Error("Expected gcs::https://storage.googleapis.com/bucket/module.zip to be handled")
		}
	})

	t.Run("RejectsEmptySource", func(t *testing.T) {
		// Given a standard module resolver
		resolver := setup(t)

		// When checking if it should handle an empty source
		// Then it should not handle empty sources
		if resolver.shouldHandle("") {
			t.Error("Expected empty source to not be handled")
		}
	})

	t.Run("RejectsOCISources", func(t *testing.T) {
		// Given a standard module resolver
		resolver := setup(t)

		// When checking if it should handle OCI sources
		// Then it should not handle OCI sources
		if resolver.shouldHandle("oci://registry.example.com/module:latest") {
			t.Error("Expected oci://registry.example.com/module:latest to not be handled")
		}
	})

	t.Run("HandlesMercurialSources", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking if it should handle mercurial sources
		testCases := []string{
			"hg::https://bitbucket.org/user/repo",
			"hg::ssh://hg@bitbucket.org/user/repo",
		}

		for _, source := range testCases {
			// Then it should handle mercurial sources
			if !resolver.shouldHandle(source) {
				t.Errorf("Expected %s to be handled", source)
			}
		}
	})

	t.Run("HandlesAdditionalGitSources", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking if it should handle additional git sources
		testCases := []string{
			"git@gitlab.com:user/repo.git",
			"git@bitbucket.org:user/repo.git",
			"git@custom-server.com:user/repo.git",
		}

		for _, source := range testCases {
			// Then it should handle git sources
			if !resolver.shouldHandle(source) {
				t.Errorf("Expected %s to be handled", source)
			}
		}
	})

	t.Run("HandlesBitbucketSources", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking if it should handle bitbucket sources
		testCases := []string{
			"bitbucket.org/user/repo",
			"bitbucket.org/user/repo.git",
		}

		for _, source := range testCases {
			// Then it should handle bitbucket sources
			if !resolver.shouldHandle(source) {
				t.Errorf("Expected %s to be handled", source)
			}
		}
	})

	t.Run("RejectsUnsupportedSources", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking if it should handle unsupported sources
		testCases := []string{
			"ftp://example.com/module.zip",
			"file:///local/path",
			"registry.terraform.io/hashicorp/aws", // 4 parts
			"invalid-source",
		}

		for _, source := range testCases {
			// Then it should not handle unsupported sources
			if resolver.shouldHandle(source) {
				t.Errorf("Expected %s to not be handled", source)
			}
		}
	})

	t.Run("HandlesGitSSHWithoutColon", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking git@ sources without colon (should not be handled)
		testCases := []string{
			"git@github.com",
			"git@gitlab.com",
		}

		for _, source := range testCases {
			// Then it should not handle git sources without colon
			if resolver.shouldHandle(source) {
				t.Errorf("Expected %s to not be handled", source)
			}
		}
	})
}

func TestStandardModuleResolver_isTerraformRegistryModule(t *testing.T) {
	setup := func(t *testing.T) *StandardModuleResolver {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.shims = mocks.Shims
		return resolver
	}

	t.Run("ValidRegistryModules", func(t *testing.T) {
		resolver := setup(t)
		if !resolver.isTerraformRegistryModule("terraform-aws-modules/vpc/aws") {
			t.Error("Expected terraform-aws-modules/vpc/aws to be valid")
		}
		if !resolver.isTerraformRegistryModule("hashicorp/consul/aws") {
			t.Error("Expected hashicorp/consul/aws to be valid")
		}
		if !resolver.isTerraformRegistryModule("my-org/my-module/my-provider") {
			t.Error("Expected my-org/my-module/my-provider to be valid")
		}
	})

	t.Run("InvalidRegistryModules", func(t *testing.T) {
		resolver := setup(t)
		if resolver.isTerraformRegistryModule("invalid") {
			t.Error("Expected invalid to not be valid")
		}
		if resolver.isTerraformRegistryModule("too/many/parts/here") {
			t.Error("Expected too/many/parts/here to not be valid")
		}
		if resolver.isTerraformRegistryModule("only/two") {
			t.Error("Expected only/two to not be valid")
		}
		if resolver.isTerraformRegistryModule("empty//provider") {
			t.Error("Expected empty//provider to not be valid")
		}
		if resolver.isTerraformRegistryModule("invalid@chars/provider") {
			t.Error("Expected invalid@chars/provider to not be valid")
		}
	})

	t.Run("HandlesSpecialCharacters", func(t *testing.T) {
		resolver := setup(t)
		if !resolver.isTerraformRegistryModule("my-org/my_module/my-provider") {
			t.Error("Expected my-org/my_module/my-provider to be valid")
		}
		if !resolver.isTerraformRegistryModule("org123/module456/provider789") {
			t.Error("Expected org123/module456/provider789 to be valid")
		}
	})

	t.Run("HandlesEdgeCases", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking edge cases for registry modules
		validCases := []string{
			"a/b/c",                   // minimal valid case
			"A/B/C",                   // uppercase
			"a-b/c_d/e-f",             // mixed separators
			"123/456/789",             // all numbers
			"_test/_module/_provider", // starting with underscore
			"test-/module_/provider-", // ending with separator
		}

		for _, testCase := range validCases {
			if !resolver.isTerraformRegistryModule(testCase) {
				t.Errorf("Expected %s to be valid registry module", testCase)
			}
		}
	})

	t.Run("RejectsInvalidCharacters", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking invalid characters for registry modules
		invalidCases := []string{
			"org/module/provider@version", // @ character
			"org/module/provider.git",     // . character
			"org/module/provider+extra",   // + character
			"org/module/provider space",   // space character
			"org/module/provider!",        // ! character
			"org/module/provider#hash",    // # character
			"org/module/provider%",        // % character
			"org/module/provider&",        // & character
			"org/module/provider*",        // * character
			"org/module/provider(",        // ( character
			"org/module/provider)",        // ) character
			"org/module/provider=",        // = character
			"org/module/provider[",        // [ character
			"org/module/provider]",        // ] character
			"org/module/provider{",        // { character
			"org/module/provider}",        // } character
			"org/module/provider|",        // | character
			"org/module/provider\\",       // \ character
			"org/module/provider:",        // : character
			"org/module/provider;",        // ; character
			"org/module/provider\"",       // " character
			"org/module/provider'",        // ' character
			"org/module/provider<",        // < character
			"org/module/provider>",        // > character
			"org/module/provider,",        // , character
			"org/module/provider?",        // ? character
			"org/module/provider/",        // trailing slash
		}

		for _, testCase := range invalidCases {
			if resolver.isTerraformRegistryModule(testCase) {
				t.Errorf("Expected %s to not be valid registry module", testCase)
			}
		}
	})

	t.Run("RejectsUnicodeCharacters", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking unicode characters for registry modules
		unicodeCases := []string{
			"org/module/provider™",        // trademark symbol
			"org/module/provider©",        // copyright symbol
			"org/module/provider®",        // registered trademark
			"org/module/provider€",        // euro symbol
			"org/module/provider中文",       // Chinese characters
			"org/module/provider日本語",      // Japanese characters
			"org/module/provider한국어",      // Korean characters
			"org/module/provider العربية", // Arabic characters
			"org/module/provider русский", // Russian characters
			"org/module/provider ñ",       // accented character
			"org/module/provider ç",       // cedilla
			"org/module/provider ü",       // umlaut
		}

		for _, testCase := range unicodeCases {
			if resolver.isTerraformRegistryModule(testCase) {
				t.Errorf("Expected %s to not be valid registry module", testCase)
			}
		}
	})

	t.Run("HandlesBoundaryConditions", func(t *testing.T) {
		// Given a resolver
		resolver := setup(t)

		// When checking boundary conditions
		boundaryCases := []string{
			"",          // empty string
			"/",         // single slash
			"//",        // double slash
			"a//c",      // empty middle part
			"/b/c",      // empty first part
			"a/b/",      // empty last part
			"a/b/c/d",   // too many parts
			"a",         // single part
			"a/b",       // two parts
			"a/b/c/d/e", // five parts
		}

		for _, testCase := range boundaryCases {
			if resolver.isTerraformRegistryModule(testCase) {
				t.Errorf("Expected %s to not be valid registry module", testCase)
			}
		}
	})
}
