package services

import (
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
)

func setupSafeBaseServiceMocks(optionalInjector ...di.Injector) *BaseService {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	return &BaseService{injector: injector}
}

func TestBaseService_WriteConfig(t *testing.T) {
	t.Run("NoOp", func(t *testing.T) {
		service := setupSafeBaseServiceMocks()
		err := service.WriteConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestBaseService_SetAddress(t *testing.T) {
	t.Run("ValidAddress", func(t *testing.T) {
		service := setupSafeBaseServiceMocks()
		err := service.SetAddress("192.168.1.1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if service.address != "192.168.1.1" {
			t.Fatalf("expected address to be '192.168.1.1', got %s", service.address)
		}
	})

	t.Run("InvalidAddress", func(t *testing.T) {
		service := setupSafeBaseServiceMocks()
		err := service.SetAddress("invalid-address")
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestBaseService_GetAddress(t *testing.T) {
	t.Run("GetAddress", func(t *testing.T) {
		service := setupSafeBaseServiceMocks()
		service.address = "192.168.1.1"
		address := service.GetAddress()
		if address != "192.168.1.1" {
			t.Fatalf("expected address to be '192.168.1.1', got %s", address)
		}
	})
}

func TestBaseService_SetName(t *testing.T) {
	t.Run("SetName", func(t *testing.T) {
		service := setupSafeBaseServiceMocks()
		service.SetName("test-service")
		if service.name != "test-service" {
			t.Fatalf("expected name to be 'test-service', got %s", service.name)
		}
	})
}

func TestBaseService_GetName(t *testing.T) {
	t.Run("GetName", func(t *testing.T) {
		service := setupSafeBaseServiceMocks()
		service.name = "test-service"
		name := service.GetName()
		if name != "test-service" {
			t.Fatalf("expected name to be 'test-service', got %s", name)
		}
	})
}

func TestBaseService_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		service := setupSafeBaseServiceMocks()
		err := service.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
