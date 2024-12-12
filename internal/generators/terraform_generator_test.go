package generators

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/blueprint"
)

func TestNewTerraformGenerator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)

		// Then the generator should be non-nil
		if generator == nil {
			t.Errorf("Expected NewTerraformGenerator to return a non-nil value")
		}
	})
}

func TestTerraformGenerator_Write(t *testing.T) {
	// Common components setup
	remoteComponent := blueprint.TerraformComponentV1Alpha1{
		Name:   "remote_component",
		Source: "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git@v1.0.0",
		Values: map[string]interface{}{
			"remote_variable1": "default_value",
		},
	}

	localComponent := blueprint.TerraformComponentV1Alpha1{
		Name:   "local_component",
		Source: "local/path",
		Values: map[string]interface{}{
			"local_variable1": "default_value",
		},
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		if err := generator.Write(); err != nil {
			// Then it should succeed without errors
			t.Errorf("Expected TerraformGenerator.Write to return a nil value")
		}
	})

	t.Run("NotRemoteSource", func(t *testing.T) {
		// Given a set of safe mocks with a local source component
		mocks := setupSafeMocks()

		// Mock the blueprint handler methods to return the local component
		mocks.MockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprint.TerraformComponentV1Alpha1 {
			return []blueprint.TerraformComponentV1Alpha1{localComponent}
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		if err := generator.Write(); err != nil {
			// Then it should succeed without errors
			t.Errorf("Expected TerraformGenerator.Write to return a nil value")
		}
	})

	t.Run("RemoteSource", func(t *testing.T) {
		// Given a set of safe mocks with a remote source component
		mocks := setupSafeMocks()

		// Mock the blueprint handler methods to return the remote component
		mocks.MockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprint.TerraformComponentV1Alpha1 {
			return []blueprint.TerraformComponentV1Alpha1{remoteComponent}
		}

		// Capture the first and second written content
		var firstWrittenContent, secondWrittenContent []byte
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
			if firstWrittenContent == nil {
				firstWrittenContent = data
			} else if secondWrittenContent == nil {
				secondWrittenContent = data
			}
			return nil
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		if err := generator.Write(); err != nil {
			// Then it should succeed without errors
			t.Errorf("Expected TerraformGenerator.Write to return a nil value")
		}

		// Validate that the expected module content was written the first time
		expectedModuleContent := `module "remote_component" {
  source           = "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git@v1.0.0"
  remote_variable1 = var.remote_variable1
}
`
		if string(firstWrittenContent) != expectedModuleContent {
			t.Errorf("Expected first written content to be:\n%s\nBut got:\n%s", expectedModuleContent, string(firstWrittenContent))
		}

		// Validate that the expected variables content was written the second time
		expectedVariablesContent := `variable "remote_variable1" {
}
`
		if string(secondWrittenContent) != expectedVariablesContent {
			t.Errorf("Expected second written content to be:\n%s\nBut got:\n%s", expectedVariablesContent, string(secondWrittenContent))
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Mock the shell methods to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})
	t.Run("ErrorMakingDirectory", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Use the mock components we created above
		mocks.MockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprint.TerraformComponentV1Alpha1 {
			return []blueprint.TerraformComponentV1Alpha1{remoteComponent}
		}

		// Mock the osMkdirAll function to return an error
		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("error making directory")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWritingModuleFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Use the mock components we created above
		mocks.MockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprint.TerraformComponentV1Alpha1 {
			return []blueprint.TerraformComponentV1Alpha1{remoteComponent}
		}

		// Mock the osWriteFile function to return an error specifically for the main.tf file
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "main.tf") {
				return fmt.Errorf("error writing module file")
			}
			return originalOsWriteFile(name, data, perm)
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWritingVariableFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Use the mock components we created above
		mocks.MockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprint.TerraformComponentV1Alpha1 {
			return []blueprint.TerraformComponentV1Alpha1{remoteComponent}
		}

		// Mock the osWriteFile function to return an error specifically for the variables file
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.Contains(name, "variables.tf") {
				return fmt.Errorf("error writing variable file")
			}
			return originalOsWriteFile(name, data, perm)
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})
}

func TestTerraformGenerator_isValidTerraformRemoteSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "ValidLocalPath",
			source: "/absolute/path/to/module",
			want:   false,
		},
		{
			name:   "ValidRelativePath",
			source: "./relative/path/to/module",
			want:   false,
		},
		{
			name:   "InvalidLocalPath",
			source: "/invalid/path/to/module",
			want:   false,
		},
		{
			name:   "ValidGitURL",
			source: "git::https://github.com/user/repo.git",
			want:   true,
		},
		{
			name:   "ValidSSHGitURL",
			source: "git@github.com:user/repo.git",
			want:   true,
		},
		{
			name:   "ValidHTTPURL",
			source: "https://github.com/user/repo.git",
			want:   true,
		},
		{
			name:   "ValidHTTPZipURL",
			source: "https://example.com/archive.zip",
			want:   true,
		},
		{
			name:   "InvalidHTTPURL",
			source: "https://example.com/not-a-zip",
			want:   false,
		},
		{
			name:   "ValidTerraformRegistry",
			source: "registry.terraform.io/hashicorp/consul/aws",
			want:   true,
		},
		{
			name:   "ValidGitHubReference",
			source: "github.com/hashicorp/terraform-aws-consul",
			want:   true,
		},
		{
			name:   "InvalidSource",
			source: "invalid-source",
			want:   false,
		},
		{
			name:   "VersionFileGitAtURL",
			source: "git@github.com:user/version.git",
			want:   true,
		},
		{
			name:   "VersionFileGitAtURLWithPath",
			source: "git@github.com:user/version.git@v1.0.0",
			want:   true,
		},
		{
			name:   "ValidGitLabURL",
			source: "git::https://gitlab.com/user/repo.git",
			want:   true,
		},
		{
			name:   "ValidSSHGitLabURL",
			source: "git@gitlab.com:user/repo.git",
			want:   true,
		},
		{
			name:   "ErrorCausingPattern",
			source: "[invalid-regex",
			want:   false,
		},
	}

	t.Run("ValidSources", func(t *testing.T) {
		for _, tt := range tests {
			if tt.name == "RegexpMatchStringError" {
				continue
			}
			t.Run(tt.name, func(t *testing.T) {
				if got := isValidTerraformRemoteSource(tt.source); got != tt.want {
					t.Errorf("isValidTerraformRemoteSource(%s) = %v, want %v", tt.source, got, tt.want)
				}
			})
		}
	})

	t.Run("RegexpMatchStringError", func(t *testing.T) {
		// Mock the regexpMatchString function to simulate an error for the specific test case
		originalRegexpMatchString := regexpMatchString
		defer func() { regexpMatchString = originalRegexpMatchString }()
		regexpMatchString = func(pattern, s string) (bool, error) {
			return false, fmt.Errorf("mocked error in regexpMatchString")
		}

		if got := isValidTerraformRemoteSource("[invalid-regex"); got != false {
			t.Errorf("isValidTerraformRemoteSource([invalid-regex) = %v, want %v", got, false)
		}
	})
}
