package helpers

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
)

// EncryptFile encrypts the specified file using SOPS.
func EncryptFile(t *testing.T, filePath string, dstPath string) error {
	t.Logf("sopsConfigPath: %v", filePath)
	t.Logf("sopsEncConfigPath: %v", dstPath)

	// Print the contents of the sops config file
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read sops config file: %v", err)
	}
	t.Logf("Contents of sops config file: %s", content)

	// Print the version of SOPS being used
	cmdVersion := exec.Command("sops", "--version")
	versionOutput, err := cmdVersion.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get SOPS version: %v", err)
	}
	t.Logf("SOPS version: %s", versionOutput)

	cmdEncrypt := exec.Command("sops", "-e", filePath, "--output", dstPath)
	cmdEncryptOutput, err := cmdEncrypt.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute sops -e %v: %v", filePath, err)
	}
	t.Logf("SOPS encrypt output: %s", cmdEncryptOutput)

	// // Create the command to encrypt the file
	// cmd := exec.Command("sops", "-e", filePath)

	// // Create a pipe to capture the output
	// outputPipe, err := cmd.StdoutPipe()
	// if err != nil {
	// 	return err
	// }

	// t.Logf("OUTPUTPIPE : %v", outputPipe)

	// // Start the command
	// if err := cmd.Start(); err != nil {
	// 	return err
	// }

	// // Create the output file
	// outputFile, err := os.Create(dstPath)
	// if err != nil {
	// 	return err
	// }
	// defer outputFile.Close() // Ensure the file is closed after writing

	// // Copy the output from the command to the output file
	// if _, err := io.Copy(outputFile, outputPipe); err != nil {
	// 	return err
	// }

	// // // Wait for the command to finish
	// // if err := cmd.Wait(); err != nil {
	// // 	t.Logf("SOPS COMMAND err : %v", err)
	// // 	return err
	// // }

	// // Print the contents of the sops config file
	// content, err = os.ReadFile(dstPath)
	// if err != nil {
	// 	t.Fatalf("Failed to read sops config file: %v", err)
	// }
	// t.Logf("Contents of sops encrypted file: %s", content)

	return nil
}

// // EncryptFile encrypts the specified file using SOPS.
// func EncryptFile(t *testing.T, filePath string, dstPath string) error {

// 	t.Logf("sopsConfigPath: %v", filePath)
// 	t.Logf("sopsEncConfigPath: %v", dstPath)

// 	// Print the contents of the sops config file
// 	content, err := os.ReadFile(filePath)
// 	if err != nil {
// 		t.Fatalf("Failed to read sops config file: %v", err)
// 	}
// 	t.Logf("Contents of sops config file: %s", content)

// 	// Print the version of SOPS being used
// 	cmdVersion := exec.Command("sops", "--version")
// 	versionOutput, err := cmdVersion.CombinedOutput()
// 	if err != nil {
// 		t.Fatalf("Failed to get SOPS version: %v", err)
// 	}
// 	t.Logf("SOPS version: %s", versionOutput)

// 	// Create the command to encrypt the file
// 	cmd := exec.Command("sops", "-e", filePath)

// 	// Create the output file
// 	outputFile, err := os.Create(dstPath)
// 	if err != nil {
// 		return err
// 	}
// 	defer outputFile.Close() // Ensure the file is closed after writing

// 	// Set the command's output to the output file
// 	cmd.Stdout = outputFile

// 	// Run the command
// 	_, err = cmd.CombinedOutput()
// 	if err != nil {
// 		t.Logf("SOPS COMMAND err : %v", err)
// 		return err
// 	}

// 	return nil
// }

func TestSopsHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		sopsConfigPath := filepath.Join(contextPath, ".sops/secrets.yaml")
		sopsEncConfigPath := filepath.Join(contextPath, ".sops/secrets.enc.yaml")

		// Ensure the sops config file exists
		err := os.MkdirAll(filepath.Dir(sopsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create sops config directory: %v", err)
		}
		_, err = os.Create(sopsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create sops config file: %v", err)
		}

		// Create and initialize the sops config file
		os.WriteFile(sopsConfigPath, []byte("\"SOPSCONFIG\": "+sopsEncConfigPath), 0644)

		// Encrypt the sops config file using SOPS
		err = EncryptFile(t, sopsConfigPath, sopsEncConfigPath)
		if err != nil {
			t.Fatalf("Failed to encrypt sops config file: %v", err)
		}

		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

		defer func() {
			if err := os.Remove(sopsEncConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := sopsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"SOPSCONFIG": sopsEncConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		_, err := sopsHelper.GetEnvVars()

		if err != nil {
			expectedError := "file does not exist"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given a mock shell and context that returns an error for config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving config root")
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		_, err := sopsHelper.GetEnvVars()

		if err != nil {

			expectedError := "error retrieving config root"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
		}
	})

	t.Run("SopsMetaDataNotFound", func(t *testing.T) {

		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		sopsConfigPath := filepath.Join(contextPath, ".sops/secrets.yaml")

		// Ensure the sops config file exists
		err := os.MkdirAll(filepath.Dir(sopsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create sops config directory: %v", err)
		}
		_, err = os.Create(sopsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create sops config file: %v", err)
		}

		// Create and initialize the sops config file
		os.WriteFile(sopsConfigPath, []byte("\"SOPSCONFIG\": "+sopsConfigPath), 0644)

		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		_, err = sopsHelper.GetEnvVars()

		if err != nil {

			expectedError := "sops metadata not found"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
		}
	})

	t.Run("ErrorUnmarshallingYaml", func(t *testing.T) {

		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		sopsConfigPath := filepath.Join(contextPath, ".sops/secrets.yaml")
		sopsEncConfigPath := filepath.Join(contextPath, ".sops/secrets.enc.yaml")

		// Ensure the sops config file exists
		err := os.MkdirAll(filepath.Dir(sopsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create sops config directory: %v", err)
		}
		_, err = os.Create(sopsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create sops config file: %v", err)
		}

		// Create and initialize the sops config file
		os.WriteFile(sopsConfigPath, []byte("\"SOPS-CONFIG\": "+sopsEncConfigPath), 0644)

		// Encrypt the sops config file using SOPS
		err = EncryptFile(t, sopsConfigPath, sopsEncConfigPath)
		if err != nil {
			t.Fatalf("Failed to encrypt sops config file: %v", err)
		}

		// Append "breaking-code" to the sops config file
		err = os.WriteFile(sopsEncConfigPath, []byte("breaking-code\n"), 0644) // Overwrites the file
		if err != nil {
			t.Fatalf("Failed to write to sops config file: %v", err)
		}

		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()
		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsEncConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		_, err = sopsHelper.GetEnvVars()

		if err != nil {

			expectedError := "Error unmarshalling input yaml"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
		}

	})
}
