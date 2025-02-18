package secrets

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockSafeComponents struct {
	Injector di.Injector
	Shell    *shell.MockShell
}

// setupSafeMocks creates mock components for testing the secrets provider
func setupSafeMocks(injector ...di.Injector) MockSafeComponents {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a mock shell
	mockShell := shell.NewMockShell()
	mockInjector.Register("shell", mockShell)

	return MockSafeComponents{
		Injector: mockInjector,
		Shell:    mockShell,
	}
}

func TestMockSecretsProvider_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		mock.InitializeFunc = func() error {
			return nil
		}
		err := mock.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		err := mock.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockSecretsProvider_LoadSecrets(t *testing.T) {
	mockLoadSecretsErr := fmt.Errorf("mock load secrets error")

	t.Run("WithFuncSet", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		mock.LoadSecretsFunc = func() error {
			return mockLoadSecretsErr
		}
		err := mock.LoadSecrets()
		if err != mockLoadSecretsErr {
			t.Errorf("Expected error = %v, got = %v", mockLoadSecretsErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		err := mock.LoadSecrets()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockSecretsProvider_GetSecret(t *testing.T) {
	mockGetSecretErr := fmt.Errorf("mock get secret error")

	t.Run("WithFuncSet", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		mock.GetSecretFunc = func(key string) (string, error) {
			return "", mockGetSecretErr
		}
		_, err := mock.GetSecret("test_key")
		if err != mockGetSecretErr {
			t.Errorf("Expected error = %v, got = %v", mockGetSecretErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		_, err := mock.GetSecret("test_key")
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})
}

func TestMockSecretsProvider_ParseSecrets(t *testing.T) {
	mockParseSecretsErr := fmt.Errorf("mock parse secrets error")

	t.Run("WithFuncSet", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		mock.ParseSecretsFunc = func(input string) (string, error) {
			return "", mockParseSecretsErr
		}
		_, err := mock.ParseSecrets("input")
		if err != mockParseSecretsErr {
			t.Errorf("Expected error = %v, got = %v", mockParseSecretsErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		output, err := mock.ParseSecrets("input")
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
		mock := NewMockSecretsProvider()
		mock.UnlockFunc = func() error {
			return mockUnlockErr
		}
		err := mock.Unlock()
		if err != mockUnlockErr {
			t.Errorf("Expected error = %v, got = %v", mockUnlockErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		mock := NewMockSecretsProvider()
		err := mock.Unlock()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
