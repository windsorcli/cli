// The MockSecretsProviderTest is a test suite for the MockSecretsProvider
// It provides comprehensive testing of the mock implementation
// It serves as a validation mechanism for the mock's behavior
// It ensures the mock correctly implements the SecretsProvider interface

package secrets

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Methods
// =============================================================================

func TestMockSecretsProvider_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		// Given a mock secrets provider with InitializeFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())
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

	t.Run("NoInitializeFunc", func(t *testing.T) {
		// Given a mock secrets provider with no InitializeFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())

		// When Initialize is called
		err := mock.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockSecretsProvider_LoadSecrets(t *testing.T) {
	mockLoadSecretsErr := fmt.Errorf("mock load secrets error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with LoadSecretsFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())
		mock.LoadSecretsFunc = func() error {
			return mockLoadSecretsErr
		}

		// When LoadSecrets is called
		err := mock.LoadSecrets()

		// Then the expected error should be returned
		if err != mockLoadSecretsErr {
			t.Errorf("Expected error = %v, got = %v", mockLoadSecretsErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with no LoadSecretsFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())

		// When LoadSecrets is called
		err := mock.LoadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockSecretsProvider_GetSecret(t *testing.T) {
	mockGetSecretErr := fmt.Errorf("mock get secret error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with GetSecretFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())
		mock.GetSecretFunc = func(key string) (string, error) {
			return "", mockGetSecretErr
		}

		// When GetSecret is called
		_, err := mock.GetSecret("test_key")

		// Then the expected error should be returned
		if err != mockGetSecretErr {
			t.Errorf("Expected error = %v, got = %v", mockGetSecretErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with no GetSecretFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())

		// When GetSecret is called
		_, err := mock.GetSecret("test_key")

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})
}

func TestMockSecretsProvider_ParseSecrets(t *testing.T) {
	mockParseSecretsErr := fmt.Errorf("mock parse secrets error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with ParseSecretsFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())
		mock.ParseSecretsFunc = func(input string) (string, error) {
			return "", mockParseSecretsErr
		}

		// When ParseSecrets is called
		_, err := mock.ParseSecrets("input")

		// Then the expected error should be returned
		if err != mockParseSecretsErr {
			t.Errorf("Expected error = %v, got = %v", mockParseSecretsErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with no ParseSecretsFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())

		// When ParseSecrets is called
		output, err := mock.ParseSecrets("input")

		// Then no error should be returned and the input should be returned unchanged
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
		if output != "input" {
			t.Errorf("Expected output = %v, got = %v", "input", output)
		}
	})
}

func TestMockSecretsProvider_Unlock(t *testing.T) {
	mockUnlockErr := fmt.Errorf("mock unlock error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with UnlockFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())
		mock.UnlockFunc = func() error {
			return mockUnlockErr
		}

		// When Unlock is called
		err := mock.Unlock()

		// Then the expected error should be returned
		if err != mockUnlockErr {
			t.Errorf("Expected error = %v, got = %v", mockUnlockErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock secrets provider with no UnlockFunc set
		mock := NewMockSecretsProvider(di.NewMockInjector())

		// When Unlock is called
		err := mock.Unlock()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
