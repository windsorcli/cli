package generators

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

const (
	gitGenTestMockGitignorePath = "mock/project/root/.gitignore"
	gitGenTestExistingContent   = "existing content\n"
	gitGenTestExpectedContent   = `existing content

# managed by windsor cli
.windsor/
.volumes/
terraform/**/backend_override.tf
contexts/**/.terraform/
contexts/**/.tfstate/
contexts/**/.kube/
contexts/**/.talos/
contexts/**/.omni/
contexts/**/.aws/
contexts/**/.azure/`
	gitGenTestExpectedPerm = fs.FileMode(0644)
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestGitGenerator_NewGitGenerator(t *testing.T) {
	t.Run("NewGitGenerator", func(t *testing.T) {
		// Given a set of mocks
		mocks := setupMocks(t)

		// When a new GitGenerator is created
		generator := NewGitGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// Then the GitGenerator should be created correctly
		if generator == nil {
			t.Fatalf("expected GitGenerator to be created, got nil")
		}

		// And the GitGenerator should have the correct injector
		if generator.injector != mocks.Injector {
			t.Errorf("expected GitGenerator to have the correct injector")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGitGenerator_Write(t *testing.T) {
	setup := func(t *testing.T) (*GitGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewGitGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And GetProjectRoot is mocked to return a specific path
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return filepath.Join("mock", "project", "root"), nil
		}

		// And ReadFile is mocked to return existing content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			expectedPath := filepath.Join("mock", "project", "root", ".gitignore")
			if path == expectedPath {
				return []byte(gitGenTestExistingContent), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to verify the content
		var writtenPath string
		var writtenContent []byte
		mocks.Shims.WriteFile = func(path string, content []byte, _ fs.FileMode) error {
			writtenPath = path
			writtenContent = content
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the file should be written to the correct path
		expectedPath := filepath.Join("mock", "project", "root", ".gitignore")
		if writtenPath != expectedPath {
			t.Errorf("expected filename %s, got %s", expectedPath, writtenPath)
		}

		// And the content should be correct
		expectedContent := gitGenTestExpectedContent
		actualContent := string(writtenContent)
		if actualContent != expectedContent {
			// Trim trailing whitespace and newlines for robust comparison
			trimmedExpected := expectedContent
			trimmedActual := actualContent
			for len(trimmedExpected) > 0 && (trimmedExpected[len(trimmedExpected)-1] == '\n' || trimmedExpected[len(trimmedExpected)-1] == '\r') {
				trimmedExpected = trimmedExpected[:len(trimmedExpected)-1]
			}
			for len(trimmedActual) > 0 && (trimmedActual[len(trimmedActual)-1] == '\n' || trimmedActual[len(trimmedActual)-1] == '\r') {
				trimmedActual = trimmedActual[:len(trimmedActual)-1]
			}
			if trimmedActual != trimmedExpected {
				t.Errorf("expected content %q, got %q", trimmedExpected, trimmedActual)
			}
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of mocks
		mocks := setupMocks(t)

		// And GetProjectRoot is mocked to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// And a new GitGenerator is created
		generator := NewGitGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to get project root: mock error getting project root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadingGitignore", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading .gitignore")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read .gitignore: mock error reading .gitignore"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("GitignoreDoesNotExist", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to simulate .gitignore does not exist
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And WriteFile is mocked to simulate successful file creation
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorWritingGitignore", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And WriteFile is mocked to simulate an error during file writing
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing .gitignore")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write to .gitignore: mock error writing .gitignore"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("HandlesCommentedOutLines", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And GetProjectRoot is mocked to return a specific path
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return filepath.Join("mock", "project", "root"), nil
		}

		// And ReadFile is mocked to return content with various commented out Windsor entries
		commentedContent := "existing content\n# .aws/\n # .aws/\n#    .aws/\n## .aws/\n#\t.aws/\n# .aws/   \n#contexts/**/.terraform/\n#    contexts/**/.terraform/   "
		commentedContent = strings.ReplaceAll(commentedContent, "#\t.aws/", "#\t.aws/")
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			expectedPath := filepath.Join("mock", "project", "root", ".gitignore")
			if path == expectedPath {
				return []byte(commentedContent), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to verify the content
		var writtenContent []byte
		mocks.Shims.WriteFile = func(path string, content []byte, _ fs.FileMode) error {
			writtenContent = content
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the content should preserve all commented lines and not add uncommented duplicates
		actualContent := string(writtenContent)
		commentVariants := []string{
			"# .aws/",
			" # .aws/",
			"#    .aws/",
			"## .aws/",
			"#\t.aws/",
			"# .aws/   ",
			"#contexts/**/.terraform/",
			"#    contexts/**/.terraform/   ",
		}
		commentVariants[4] = "#\t.aws/"
		for i, variant := range commentVariants {
			if i == 4 {
				variant = "#\t.aws/"
			}
			if !strings.Contains(actualContent, variant) {
				t.Errorf("expected content to preserve commented variant: %q", variant)
			}
		}

		// Check that uncommented versions are NOT added when any commented version exists
		lines := strings.Split(actualContent, "\n")
		hasUncommentedAws := false
		hasUncommentedTerraform := false
		for _, line := range lines {
			if strings.TrimSpace(line) == ".aws/" {
				hasUncommentedAws = true
			}
			if strings.TrimSpace(line) == "contexts/**/.terraform/" {
				hasUncommentedTerraform = true
			}
		}
		if hasUncommentedAws {
			t.Errorf("expected content to not add uncommented .aws/ when any commented version exists")
		}
		if hasUncommentedTerraform {
			t.Errorf("expected content to not add uncommented contexts/**/.terraform/ when any commented version exists")
		}
	})
}
