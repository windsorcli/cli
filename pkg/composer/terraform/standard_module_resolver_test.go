package terraform

import (
	"errors"
	"os"
	"strings"
	"testing"

	"encoding/json"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// The StandardModuleResolverTest is a test suite for the StandardModuleResolver implementation
// It provides comprehensive coverage for standard terraform module source processing and validation
// The StandardModuleResolverTest ensures proper handling of git repositories, local paths, and registry modules
// enabling reliable terraform module resolution and shim generation for standard sources

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStandardModuleResolver_NewStandardModuleResolver(t *testing.T) {
	t.Run("CreatesStandardModuleResolver", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{})
		resolver := NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		if resolver == nil {
			t.Fatal("Expected non-nil standard module resolver")
		}
		if resolver.BaseModuleResolver == nil {
			t.Error("Expected BaseModuleResolver to be set")
		}
		if resolver.reset {
			t.Error("Expected reset to be false by default")
		}
	})
}

func TestStandardModuleResolver_NewStandardModuleResolverWithDependencies(t *testing.T) {
	setup := func(t *testing.T) (*StandardModuleResolver, *Mocks) {
		t.Helper()
		mocks := setupMocks(t, &SetupOptions{})
		resolver := NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a standard module resolver with valid dependencies
		resolver, _ := setup(t)

		// Then all handlers are set
		if resolver.BaseModuleResolver.blueprintHandler == nil {
			t.Error("Expected blueprintHandler to be set")
		}
		if resolver.BaseModuleResolver.runtime.Shell == nil {
			t.Error("Expected shell to be set")
		}
		if resolver.BaseModuleResolver.runtime.ConfigHandler == nil {
			t.Error("Expected configHandler to be set")
		}
	})
}

func TestStandardModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T) (*StandardModuleResolver, *Mocks) {
		t.Helper()
		mocks := setupMocks(t, &SetupOptions{})
		resolver := NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.shims = mocks.Shims
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with proper JSON unmarshaling for complete path coverage
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.runtime.ConfigRoot = "/test/config"

		// Use real JSON unmarshaling to exercise the parsing logic
		resolver.BaseModuleResolver.shims.JsonUnmarshal = func(data []byte, v any) error {
			return json.Unmarshal(data, v)
		}

		mocks.Shell.ExecProgressFunc = func(msg, cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				// Return terraform init output with detected path different from standard
				return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-01-09T16:25:03Z","type":"log","message":"- main in /detected/module/path"}`, nil
			}
			return "", nil
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesNilBlueprintHandler", func(t *testing.T) {
		// Given a resolver with nil blueprint handler
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.blueprintHandler = nil

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating blueprint handler not initialized
		if err == nil || !strings.Contains(err.Error(), "blueprint handler not initialized") {
			t.Errorf("Expected blueprint handler error, got: %v", err)
		}
	})

	t.Run("HandlesMkdirAllError", func(t *testing.T) {
		// Given a resolver with MkdirAll shim returning error
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to create module directory
		if err == nil || !strings.Contains(err.Error(), "failed to create module directory") {
			t.Errorf("Expected mkdir error, got: %v", err)
		}
	})

	t.Run("HandlesChdirError", func(t *testing.T) {
		// Given a resolver with Chdir shim returning error
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.shims.Chdir = func(path string) error {
			return errors.New("chdir error")
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to change to module directory
		if err == nil || !strings.Contains(err.Error(), "failed to change to module directory") {
			t.Errorf("Expected chdir error, got: %v", err)
		}
	})

	t.Run("HandlesGetConfigRootError", func(t *testing.T) {
		// Given a resolver with config handler GetConfigRoot returning error
		resolver, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mocks.Injector.Register("configHandler", mockConfigHandler)
		resolver.BaseModuleResolver.runtime.ConfigHandler = mockConfigHandler
		resolver.BaseModuleResolver.runtime.ConfigRoot = ""

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to get config root
		if err == nil || !strings.Contains(err.Error(), "config root is empty") {
			t.Errorf("Expected config root error, got: %v", err)
		}
	})

	t.Run("HandlesSetenvError", func(t *testing.T) {
		// Given a resolver with Setenv shim returning error for TF_DATA_DIR
		resolver, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mocks.Injector.Register("configHandler", mockConfigHandler)
		resolver.BaseModuleResolver.runtime.ConfigHandler = mockConfigHandler
		resolver.BaseModuleResolver.runtime.ConfigRoot = "/mock/config/root"
		resolver.BaseModuleResolver.shims.Setenv = func(key, value string) error {
			if key == "TF_DATA_DIR" {
				return errors.New("setenv error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to set TF_DATA_DIR
		if err == nil || !strings.Contains(err.Error(), "failed to set TF_DATA_DIR") {
			t.Errorf("Expected setenv error, got: %v", err)
		}
	})

	t.Run("HandlesTerraformInitError", func(t *testing.T) {
		// Given a resolver with Shell.ExecProgressFunc returning error for terraform init
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.runtime.ConfigRoot = "/test/config"
		mocks.Shell.ExecProgressFunc = func(msg, cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", errors.New("terraform init error")
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to initialize terraform
		if err == nil || !strings.Contains(err.Error(), "failed to initialize terraform") {
			t.Errorf("Expected terraform init error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimMainTfError", func(t *testing.T) {
		// Given a resolver with WriteFile shim returning error for main.tf
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "main.tf") {
				return errors.New("write main.tf error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to write main.tf
		if err == nil || !strings.Contains(err.Error(), "failed to write main.tf") {
			t.Errorf("Expected write main.tf error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimVariablesTfError", func(t *testing.T) {
		// Given a resolver with WriteFile shim returning error for variables.tf
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.runtime.ConfigRoot = "/test/config"
		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "variables.tf") {
				return errors.New("write variables.tf error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to write variables.tf
		if err == nil || !strings.Contains(err.Error(), "failed to write variables.tf") {
			t.Errorf("Expected write variables.tf error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimOutputsTfError", func(t *testing.T) {
		// Given a resolver with WriteFile shim returning error for outputs.tf
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.runtime.ConfigRoot = "/test/config"
		resolver.BaseModuleResolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "outputs.tf") {
				return errors.New("write outputs.tf error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to write outputs.tf
		if err == nil || !strings.Contains(err.Error(), "failed to write outputs.tf") {
			t.Errorf("Expected write outputs.tf error, got: %v", err)
		}
	})

	// Edge cases for component filtering
	t.Run("HandlesEmptySourceComponents", func(t *testing.T) {
		// Given a resolver with a component having empty source
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{{
				Path:     "test-module",
				Source:   "",
				FullPath: "/mock/project/terraform/test-module",
			}}
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned (component is skipped)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesNonStandardSources", func(t *testing.T) {
		// Given a resolver with a component having non-standard source
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{{
				Path:     "test-module",
				Source:   "oci://registry.example.com/module:latest",
				FullPath: "/mock/project/terraform/test-module",
			}}
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned (component is skipped)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	// Terraform init output parsing edge cases
	t.Run("HandlesTerraformInitOutputParsing", func(t *testing.T) {
		// Given a resolver with custom JsonUnmarshal and Stat shims for output parsing edge cases
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.runtime.ConfigRoot = "/test/config"
		resolver.BaseModuleResolver.shims.JsonUnmarshal = func(data []byte, v any) error {
			if initOutput, ok := v.(*TerraformInitOutput); ok {
				jsonStr := string(data)
				if strings.Contains(jsonStr, `"empty_line"`) {
					return nil
				}
				if strings.Contains(jsonStr, `"invalid_json"`) {
					return errors.New("invalid JSON")
				}
				if strings.Contains(jsonStr, `"non_log_type"`) {
					initOutput.Type = "info"
					return nil
				}
				if strings.Contains(jsonStr, `"no_main_in"`) {
					initOutput.Type = "log"
					initOutput.Message = "some other message"
					return nil
				}
				if strings.Contains(jsonStr, `"main_in_at_end"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in"
					return nil
				}
				if strings.Contains(jsonStr, `"empty_path"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in   "
					return nil
				}
				if strings.Contains(jsonStr, `"nonexistent_path"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in /nonexistent/path"
					return nil
				}
				if strings.Contains(jsonStr, `"valid_path"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in /valid/path"
					return nil
				}
			}
			return nil
		}

		// And Stat shim only succeeds for /valid/path
		resolver.BaseModuleResolver.shims.Stat = func(path string) (os.FileInfo, error) {
			if path == "/valid/path" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And Shell.ExecProgressFunc returns all edge case lines
		mocks.Shell.ExecProgressFunc = func(msg, cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `
{"empty_line":""}
invalid json line
{"non_log_type":"info"}
{"no_main_in":"log"}
{"main_in_at_end":"log"}
{"empty_path":"log"}
{"nonexistent_path":"log"}
{"valid_path":"log"}`, nil
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned (all output parsing edge cases handled)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

func TestStandardModuleResolver_shouldHandle(t *testing.T) {
	setup := func(t *testing.T) *StandardModuleResolver {
		t.Helper()
		mocks := setupMocks(t, &SetupOptions{})
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
		mocks := setupMocks(t, &SetupOptions{})
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
