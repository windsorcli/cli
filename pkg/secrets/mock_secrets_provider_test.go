package secrets

import (
	"fmt"
	"testing"
)

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
