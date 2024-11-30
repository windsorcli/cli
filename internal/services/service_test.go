package services

import (
	"fmt"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type MockBaseServiceComponents struct {
	Injector          di.Injector
	MockContext       *context.MockContext
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
}

func setupSafeBaseServiceMocks(optionalInjector ...di.Injector) *MockBaseServiceComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mock instances in the injector
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)

	return &MockBaseServiceComponents{
		Injector:          injector,
		MockContext:       mockContext,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
	}
}

func TestBaseService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeBaseServiceMocks()

		// When: a new BaseService is created and initialized
		service := &BaseService{injector: mocks.Injector}
		err := service.Initialize()

		// Then: the initialization should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And: the resolved dependencies should be set correctly
		if service.configHandler == nil {
			t.Fatalf("expected configHandler to be set, got nil")
		}
		if service.shell == nil {
			t.Fatalf("expected shell to be set, got nil")
		}
		if service.contextHandler == nil {
			t.Fatalf("expected contextHandler to be set, got nil")
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given: a set of mock components with a faulty injector
		mocks := setupSafeBaseServiceMocks(mockInjector)

		mockInjector.SetResolveError("configHandler", fmt.Errorf("error resolving configHandler"))

		// When: a new BaseService is created and initialized
		service := &BaseService{injector: mocks.Injector}
		err := service.Initialize()

		// Then: the initialization should fail with an error
		if err == nil {
			t.Fatalf("expected an error during initialization, got nil")
		}
		expectedErrorMessage := "error resolving configHandler: error resolving configHandler"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given: a set of mock components with a faulty injector
		mocks := setupSafeBaseServiceMocks(mockInjector)
		mockInjector.SetResolveError("shell", fmt.Errorf("error resolving shell"))

		// When: a new BaseService is created and initialized
		service := &BaseService{injector: mocks.Injector}
		err := service.Initialize()

		// Then: the initialization should fail with an error
		if err == nil {
			t.Fatalf("expected an error during initialization, got nil")
		}
		expectedErrorMessage := "error resolving shell: error resolving shell"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given: a set of mock components with a faulty injector
		mocks := setupSafeBaseServiceMocks(mockInjector)
		mockInjector.SetResolveError("contextHandler", fmt.Errorf("error resolving context"))

		// When: a new BaseService is created and initialized
		service := &BaseService{injector: mocks.Injector}
		err := service.Initialize()

		// Then: the initialization should fail with an error
		if err == nil {
			t.Fatalf("expected an error during initialization, got nil")
		}
		expectedErrorMessage := "error resolving context: error resolving context"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})
}

func TestBaseService_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: the WriteConfig should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during WriteConfig, got %v", err)
		}
	})
}

func TestBaseService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}

		// When: SetAddress is called with a valid IPv4 address
		err := service.SetAddress("192.168.1.1")

		// Then: the SetAddress should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during SetAddress, got %v", err)
		}

		// And: the address should be set correctly
		expectedAddress := "192.168.1.1"
		if service.GetAddress() != expectedAddress {
			t.Fatalf("expected address '%s', got %v", expectedAddress, service.GetAddress())
		}
	})

	t.Run("InvalidAddress", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}

		// When: SetAddress is called with an invalid IPv4 address
		err := service.SetAddress("invalid_address")

		// Then: the SetAddress should fail with an error
		if err == nil {
			t.Fatalf("expected an error during SetAddress, got nil")
		}

		// And: the error message should be as expected
		expectedErrorMessage := "invalid IPv4 address"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})
}

func TestBaseService_GetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}
		service.SetAddress("192.168.1.1")

		// When: GetAddress is called
		address := service.GetAddress()

		// Then: the address should be as expected
		expectedAddress := "192.168.1.1"
		if address != expectedAddress {
			t.Fatalf("expected address '%s', got %v", expectedAddress, address)
		}
	})
}

func TestBaseService_GetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}
		service.SetName("TestService")

		// When: GetName is called
		name := service.GetName()

		// Then: the name should be as expected
		expectedName := "TestService"
		if name != expectedName {
			t.Fatalf("expected name '%s', got %v", expectedName, name)
		}
	})
}
