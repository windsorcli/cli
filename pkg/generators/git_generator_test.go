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
	gitGenTestMockGitignorePath = "/mock/project/root/.gitignore"
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
contexts/**/.aws/
`
	gitGenTestExpectedPerm = fs.FileMode(0644)
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestGitGenerator_NewGitGenerator(t *testing.T) {
	t.Run("NewGitGenerator", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new GitGenerator is created
		gitGenerator := NewGitGenerator(mocks.Injector)

		// Then the GitGenerator should be created correctly
		if gitGenerator == nil {
			t.Fatalf("expected GitGenerator to be created, got nil")
		}

		// And the GitGenerator should have the correct injector
		if gitGenerator.injector != mocks.Injector {
			t.Errorf("expected GitGenerator to have the correct injector")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGitGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And osReadFile is mocked to return predefined content
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(filename string) ([]byte, error) {
			if filepath.ToSlash(filename) == gitGenTestMockGitignorePath {
				return []byte(gitGenTestExistingContent), nil
			}
			return []byte{}, nil // Return empty content instead of an error
		}

		// And osWriteFile is mocked to capture parameters
		var capturedFilename string
		var capturedContent []byte
		var capturedPerm fs.FileMode
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(filename string, content []byte, perm fs.FileMode) error {
			capturedFilename = filename
			capturedContent = content
			capturedPerm = perm
			return nil
		}

		// And a GitGenerator is created and initialized
		gitGenerator := NewGitGenerator(mocks.Injector)
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// When the Write method is called
		err := gitGenerator.Write()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And osWriteFile should be called with the correct parameters
		if filepath.ToSlash(capturedFilename) != gitGenTestMockGitignorePath {
			t.Errorf("expected filename %s, got %s", gitGenTestMockGitignorePath, capturedFilename)
		}

		// Normalize line endings for cross-platform compatibility
		normalizeLineEndings := func(content string) string {
			return strings.ReplaceAll(content, "\r\n", "\n")
		}

		if normalizeLineEndings(string(capturedContent)) != normalizeLineEndings(gitGenTestExpectedContent) {
			t.Errorf("expected content %s, got %s", gitGenTestExpectedContent, string(capturedContent))
		}

		if capturedPerm != gitGenTestExpectedPerm {
			t.Errorf("expected permission %v, got %v", gitGenTestExpectedPerm, capturedPerm)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And GetProjectRootFunc is mocked to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// And a GitGenerator is created and initialized
		gitGenerator := NewGitGenerator(mocks.Injector)
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// When the Write method is called
		err := gitGenerator.Write()

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
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And osReadFile is mocked to return an error
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading .gitignore")
		}

		// And a GitGenerator is created and initialized
		gitGenerator := NewGitGenerator(mocks.Injector)
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// When the Write method is called
		err := gitGenerator.Write()

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
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a GitGenerator is created and initialized
		gitGenerator := NewGitGenerator(mocks.Injector)
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// And osReadFile is mocked to simulate .gitignore does not exist
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And osWriteFile is mocked to simulate successful file creation
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return nil
		}

		// When the Write method is called
		err := gitGenerator.Write()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorWritingGitignore", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And osWriteFile is mocked to simulate an error during file writing
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing .gitignore")
		}

		// And a GitGenerator is created and initialized
		gitGenerator := NewGitGenerator(mocks.Injector)
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// When the Write method is called
		err := gitGenerator.Write()

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
}
