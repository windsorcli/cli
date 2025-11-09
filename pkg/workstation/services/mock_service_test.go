package services

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

// The MockServiceTest provides test coverage for the MockService implementation.
// It validates the mock's function field behaviors and ensures proper operation
// of the test double, verifying nil handling and custom function field behaviors.

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with InitializeFunc set
		mock := NewMockService()
		mock.InitializeFunc = func() error {
			return nil
		}

		// When Initialize is called
		err := mock.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without InitializeFunc set
		mock := NewMockService()

		// When Initialize is called
		err := mock.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with GetComposeConfigFunc set
		mock := NewMockService()
		expectedConfig := &types.Config{}
		mock.GetComposeConfigFunc = func() (*types.Config, error) {
			return expectedConfig, nil
		}

		// When GetComposeConfig is called
		config, err := mock.GetComposeConfig()

		// Then the expected config should be returned without error
		if config != expectedConfig {
			t.Errorf("Expected config = %v, got = %v", expectedConfig, config)
		}
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without GetComposeConfigFunc set
		mock := NewMockService()

		// When GetComposeConfig is called
		config, err := mock.GetComposeConfig()

		// Then nil config and no error should be returned
		if config != nil {
			t.Errorf("Expected config = nil, got = %v", config)
		}
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockService_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with WriteConfigFunc set
		mock := NewMockService()
		mock.WriteConfigFunc = func() error {
			return nil
		}

		// When WriteConfig is called
		err := mock.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without WriteConfigFunc set
		mock := NewMockService()

		// When WriteConfig is called
		err := mock.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with SetAddressFunc set
		mock := NewMockService()
		mock.SetAddressFunc = func(address string, portAllocator *PortAllocator) error {
			return nil
		}

		// When SetAddress is called
		err := mock.SetAddress("test-address", nil)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without SetAddressFunc set
		mock := NewMockService()

		// When SetAddress is called
		err := mock.SetAddress("test-address", nil)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockService_GetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with GetAddressFunc set
		mock := NewMockService()
		expectedAddress := "test-address"
		mock.GetAddressFunc = func() string {
			return expectedAddress
		}

		// When GetAddress is called
		address := mock.GetAddress()

		// Then the expected address should be returned
		if address != expectedAddress {
			t.Errorf("Expected address = %v, got = %v", expectedAddress, address)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without GetAddressFunc set
		mock := NewMockService()

		// When GetAddress is called
		address := mock.GetAddress()

		// Then an empty string should be returned
		if address != "" {
			t.Errorf("Expected address = '', got = %v", address)
		}
	})
}

func TestMockService_GetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with GetNameFunc set
		mock := NewMockService()
		expectedName := "test-name"
		mock.GetNameFunc = func() string {
			return expectedName
		}

		// When GetName is called
		name := mock.GetName()

		// Then the expected name should be returned
		if name != expectedName {
			t.Errorf("Expected name = %v, got = %v", expectedName, name)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without GetNameFunc set
		mock := NewMockService()

		// When GetName is called
		name := mock.GetName()

		// Then an empty string should be returned
		if name != "" {
			t.Errorf("Expected name = '', got = %v", name)
		}
	})
}

func TestMockService_SetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with SetNameFunc set
		mock := NewMockService()
		expectedName := "test-name"
		mock.SetNameFunc = func(name string) {
			// No-op, just verify it's called
		}

		// When SetName is called
		mock.SetName(expectedName)

		// Then the name should be set
		if mock.GetName() != expectedName {
			t.Errorf("Expected name = %v, got = %v", expectedName, mock.GetName())
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without SetNameFunc set
		mock := NewMockService()
		expectedName := "test-name"

		// When SetName is called
		mock.SetName(expectedName)

		// Then the name should be set
		if mock.GetName() != expectedName {
			t.Errorf("Expected name = %v, got = %v", expectedName, mock.GetName())
		}
	})
}

func TestMockService_GetHostname(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with GetHostnameFunc set
		mock := NewMockService()
		expectedHostname := "test-hostname"
		mock.GetHostnameFunc = func() string {
			return expectedHostname
		}

		// When GetHostname is called
		hostname := mock.GetHostname()

		// Then the expected hostname should be returned
		if hostname != expectedHostname {
			t.Errorf("Expected hostname = %v, got = %v", expectedHostname, hostname)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without GetHostnameFunc set
		mock := NewMockService()

		// When GetHostname is called
		hostname := mock.GetHostname()

		// Then an empty string should be returned
		if hostname != "" {
			t.Errorf("Expected hostname = '', got = %v", hostname)
		}
	})
}

func TestMockService_SupportsWildcard(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockService with SupportsWildcardFunc set
		mock := NewMockService()
		mock.SupportsWildcardFunc = func() bool {
			return true
		}

		// When SupportsWildcard is called
		supports := mock.SupportsWildcard()

		// Then it should return true
		if !supports {
			t.Error("Expected SupportsWildcard to return true")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a new MockService without SupportsWildcardFunc set
		mock := NewMockService()

		// When SupportsWildcard is called
		supports := mock.SupportsWildcard()

		// Then it should return false
		if supports {
			t.Error("Expected SupportsWildcard to return false")
		}
	})
}
