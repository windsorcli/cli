package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
	sh "github.com/windsorcli/cli/pkg/shell"
	"gopkg.in/ini.v1"
)

func setupSafeAwsGeneratorMocks(injector ...di.Injector) MockComponents {
	// Use the provided injector if available, otherwise create a new one
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewInjector()
	}

	// Mock the osStat function to simulate file existence
	osStat = func(name string) (os.FileInfo, error) {
		if name == filepath.Join("/mock/config/root", ".aws", "config") {
			return nil, nil // Simulate that the file exists
		}
		return nil, os.ErrNotExist
	}

	// Mock the osMkdirAll function
	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	// Mock the iniLoad function
	iniLoad = func(_ interface{}, _ ...interface{}) (*ini.File, error) {
		file := iniEmpty()
		return file, nil
	}

	// Mock the iniSaveTo function to simulate saving the ini file
	iniSaveTo = func(cfg *ini.File, filename string) error {
		if filename == filepath.Join("/mock/config/root", ".aws", "config") {
			return nil // Simulate successful save
		}
		return nil // Simulate successful save for any file
	}

	// Mock the osWriteFile function to simulate file writing
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		if name == filepath.Join("/mock/config/root", ".aws", "config") {
			return nil // Simulate successful write
		}
		return nil // Simulate successful write for any file
	}

	// Create a new mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockInjector.Register("configHandler", mockConfigHandler)

	// Mock the configHandler to return a mock config root
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.Join("/mock/config/root"), nil
	}

	// Mock the GetString method to return default values for AWS configuration
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "aws.cli_pager":
			return ""
		case "aws.output":
			return "text"
		case "aws.region":
			return constants.DEFAULT_AWS_REGION
		case "aws.profile":
			return "default"
		case "dns.domain":
			return "test"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	// Mock the GetBool method to return false for aws.localstack.enabled
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if key == "aws.localstack.enabled" {
			return false
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}

	// Create a new mock shell
	mockShell := sh.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return filepath.Join("/mock/project/root"), nil
	}
	mockInjector.Register("shell", mockShell)

	// Create a new mock localstack service
	mockLocalstackService := services.NewMockService()
	mockLocalstackService.GetNameFunc = func() string {
		return "aws"
	}
	mockInjector.Register("localstackService", mockLocalstackService)

	return MockComponents{
		Injector:          mockInjector,
		MockConfigHandler: mockConfigHandler,
		MockShell:         mockShell,
	}
}

func TestAWSGenerator_Write(t *testing.T) {
	t.Run("SuccessCreatingAwsConfig", func(t *testing.T) {
		// Use setupSafeAwsGeneratorMocks to create mock components
		mocks := setupSafeAwsGeneratorMocks()

		// Save the original osStat and osWriteFile functions
		originalStat := osStat
		originalWriteFile := osWriteFile
		defer func() {
			osStat = originalStat
			osWriteFile = originalWriteFile
		}()

		// Mock the osStat function to simulate os.IsNotExist for awsConfigFilePath
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/mock/config/root", ".aws", "config") {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		// Mock the osWriteFile function to validate that it is called with the expected parameters
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			expectedFilePath := filepath.Join("/mock/config/root", ".aws", "config")
			if filename != expectedFilePath {
				t.Errorf("Unexpected filename for osWriteFile: %s", filename)
			}
			// Additional checks on data can be added here if needed
			return nil
		}

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mocks.Injector)

		generator.Initialize()

		// Execute the Write method
		err := generator.Write()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessLocalstackEnabled", func(t *testing.T) {
		mocks := setupSafeAwsGeneratorMocks()

		// Mock the GetBool method to return true for aws.localstack.enabled
		mocks.MockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "aws.localstack.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}

		// Save the original iniSaveTo function
		originalIniSaveTo := iniSaveTo
		defer func() {
			iniSaveTo = originalIniSaveTo
		}()

		// Mock the iniSaveTo function to validate that it is called with the expected parameters
		iniSaveTo = func(cfg *ini.File, filename string) error {
			expectedFilePath := filepath.Join("/mock/config/root", ".aws", "config")
			if filename != expectedFilePath {
				t.Errorf("Unexpected filename for iniSaveTo: %s", filename)
			}
			// Additional checks on cfg can be added here if needed
			return nil
		}

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mocks.Injector)

		generator.Initialize()

		// Execute the Write method
		err := generator.Write()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		mocks := setupSafeAwsGeneratorMocks()

		// Mock the GetConfigRoot method to return an error
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error in GetConfigRoot")
		}

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mocks.Injector)

		generator.Initialize()

		// Execute the Write method and expect an error
		err := generator.Write()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedErrorMessage := "mocked error in GetConfigRoot"
		if err.Error() != expectedErrorMessage {
			t.Errorf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		mocks := setupSafeAwsGeneratorMocks()

		// Mock the GetConfigRoot method to return a valid path
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return filepath.Join("/mock/config/root"), nil
		}

		// Mock the osStat function to simulate the file does not exist
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		defer func() { osStat = os.Stat }() // Restore original function after test

		// Mock the osMkdirAll function to return an error
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mocked error in osMkdirAll")
		}
		defer func() { osMkdirAll = os.MkdirAll }() // Restore original function after test

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mocks.Injector)

		generator.Initialize()

		// Execute the Write method and expect an error
		err := generator.Write()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedErrorMessage := "mocked error in osMkdirAll"
		if err.Error() != expectedErrorMessage {
			t.Errorf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}
	})

	t.Run("NoIniFile", func(t *testing.T) {
		mocks := setupSafeAwsGeneratorMocks()

		// Mock the GetConfigRoot method to return a valid path
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return filepath.Join("/mock/config/root"), nil
		}

		// Mock the osStat function to simulate the file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		defer func() { osStat = os.Stat }() // Restore original function after test

		// Flag to check if iniLoad was called
		iniLoadCalled := false

		// Mock the iniLoad function to set the flag when called and return an error
		originalIniLoad := iniLoad
		iniLoad = func(_ interface{}, _ ...interface{}) (*ini.File, error) {
			iniLoadCalled = true
			return nil, fmt.Errorf("mocked error in iniLoad")
		}
		defer func() { iniLoad = originalIniLoad }() // Restore original shim after test

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mocks.Injector)

		generator.Initialize()

		// Execute the Write method
		err := generator.Write()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Validate that iniLoad was called
		if !iniLoadCalled {
			t.Errorf("expected iniLoad to be called, but it was not")
		}
	})

	t.Run("SuccessWithNonDefaultProfile", func(t *testing.T) {
		mocks := setupSafeAwsGeneratorMocks()

		// Mock the GetConfigRoot method to return a valid path
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return filepath.Join("/mock/config/root"), nil
		}

		// Mock the osStat function to simulate the file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		defer func() { osStat = os.Stat }() // Restore original function after test

		// Mock the iniLoad function to return an empty ini file
		originalIniLoad := iniLoad
		iniLoad = func(_ interface{}, _ ...interface{}) (*ini.File, error) {
			return iniEmpty(), nil
		}
		defer func() { iniLoad = originalIniLoad }() // Restore original shim after test

		// Mock the iniSaveTo function to validate the region key is set correctly
		originalIniSaveTo := iniSaveTo
		iniSaveTo = func(cfg *ini.File, filename string) error {
			expectedRegion := mocks.MockConfigHandler.GetString("aws.region", constants.DEFAULT_AWS_REGION)
			sectionName := "profile non-default"
			if cfg.Section(sectionName).Key("region").String() != expectedRegion {
				t.Errorf("expected region %q, got %q", expectedRegion, cfg.Section(sectionName).Key("region").String())
			}
			return nil
		}
		defer func() { iniSaveTo = originalIniSaveTo }() // Restore original shim after test

		// Mock the GetString method to return a non-default profile and a specific region
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "aws.profile" {
				return "non-default"
			}
			if key == "aws.region" {
				return "us-east-1"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mocks.Injector)

		generator.Initialize()

		// Execute the Write method
		err := generator.Write()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("FailedResolvingLocalstackService", func(t *testing.T) {
		// Create a new mock injector
		mockInjector := di.NewMockInjector()

		// Use setupSafeAwsGeneratorMocks to create mock components with the mock injector
		mocks := setupSafeAwsGeneratorMocks(mockInjector)

		// Mock the GetBool method to simulate Localstack being enabled
		mocks.MockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "aws.localstack.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}

		// Intentionally do not register the localstackService to simulate a resolution failure
		mockInjector.SetResolveError("localstackService", fmt.Errorf("mocked error in Resolve"))

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mockInjector)

		generator.Initialize()

		// Execute the Write method and expect an error
		err := generator.Write()
		if err == nil {
			t.Fatalf("expected error due to failed resolving of localstackService, got nil")
		}
		expectedError := "localstackService not found"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSavingIniFile", func(t *testing.T) {
		mocks := setupSafeAwsGeneratorMocks()

		// Mock the iniSaveTo function to return an error
		originalIniSaveTo := iniSaveTo
		defer func() { iniSaveTo = originalIniSaveTo }() // Ensure the original function is restored after the test

		iniSaveTo = func(cfg *ini.File, filename string) error {
			return fmt.Errorf("mocked error in iniSaveTo")
		}

		// Create a new AWSGenerator using the mock injector
		generator := NewAWSGenerator(mocks.Injector)

		generator.Initialize()

		// Execute the Write method and expect an error
		err := generator.Write()
		if err == nil {
			t.Fatalf("expected error due to iniSaveTo failure, got nil")
		}
		expectedError := "mocked error in iniSaveTo"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
