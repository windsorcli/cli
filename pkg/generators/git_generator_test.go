package generators

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestGitGenerator_NewGitGenerator(t *testing.T) {
	t.Run("NewGitGenerator", func(t *testing.T) {
		// Use setupSafeMocks to create mock components
		mocks := setupSafeMocks()

		// Create a new GitGenerator using the mock injector
		gitGenerator := NewGitGenerator(mocks.Injector)

		// Check if the GitGenerator is not nil
		if gitGenerator == nil {
			t.Fatalf("expected GitGenerator to be created, got nil")
		}

		// Check if the GitGenerator has the correct injector
		if gitGenerator.injector != mocks.Injector {
			t.Errorf("expected GitGenerator to have the correct injector")
		}
	})
}

func TestGitGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Mock osReadFile to return predefined content or an empty file if not exists
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(filename string) ([]byte, error) {
			if filepath.ToSlash(filename) == gitGenTestMockGitignorePath {
				return []byte(gitGenTestExistingContent), nil
			}
			return []byte{}, nil // Return empty content instead of an error
		}

		// Capture the call to osWriteFile
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

		gitGenerator := NewGitGenerator(mocks.Injector)

		// Initialize the GitGenerator
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// Call the Write method
		err := gitGenerator.Write()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Normalize line endings for cross-platform compatibility
		normalizeLineEndings := func(content string) string {
			return strings.ReplaceAll(content, "\r\n", "\n")
		}

		// Check if osWriteFile was called with the correct parameters
		if filepath.ToSlash(capturedFilename) != gitGenTestMockGitignorePath {
			t.Errorf("expected filename %s, got %s", gitGenTestMockGitignorePath, capturedFilename)
		}

		if normalizeLineEndings(string(capturedContent)) != normalizeLineEndings(gitGenTestExpectedContent) {
			t.Errorf("expected content %s, got %s", gitGenTestExpectedContent, string(capturedContent))
		}

		if capturedPerm != gitGenTestExpectedPerm {
			t.Errorf("expected permission %v, got %v", gitGenTestExpectedPerm, capturedPerm)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Mock the GetProjectRootFunc to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		gitGenerator := NewGitGenerator(mocks.Injector)

		// Initialize the GitGenerator
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// Call the Write method and expect an error
		err := gitGenerator.Write()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedError := "failed to get project root: mock error getting project root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadingGitignore", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Mock the osReadFile function to return an error
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading .gitignore")
		}

		gitGenerator := NewGitGenerator(mocks.Injector)

		// Initialize the GitGenerator
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// Call the Write method and expect an error
		err := gitGenerator.Write()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedError := "failed to read .gitignore: mock error reading .gitignore"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("GitignoreDoesNotExist", func(t *testing.T) {
		mocks := setupSafeMocks()

		gitGenerator := NewGitGenerator(mocks.Injector)

		// Initialize the GitGenerator
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// Mock the osReadFile function to simulate .gitignore does not exist
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// Mock the osWriteFile function to simulate successful file creation
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return nil
		}

		// Call the Write method and expect no error
		err := gitGenerator.Write()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorWritingGitignore", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Mock the osWriteFile function to simulate an error during file writing
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing .gitignore")
		}

		gitGenerator := NewGitGenerator(mocks.Injector)

		// Initialize the GitGenerator
		if err := gitGenerator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// Call the Write method and expect an error
		err := gitGenerator.Write()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedError := "failed to write to .gitignore: mock error writing .gitignore"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}
