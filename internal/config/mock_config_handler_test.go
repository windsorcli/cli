package config

import (
	"errors"
	"reflect"
	"testing"
)

// Helper function for error assertion
func assertError(t *testing.T, err error, expectedErr error) {
	if err != expectedErr {
		t.Errorf("Expected error = %v, got = %v", expectedErr, err)
	}
}

// Helper function for value assertion
func assertEqual(t *testing.T, expected, actual interface{}, name string) {
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected %s = %v, got = %v", name, expected, actual)
	}
}

// Helper function to create a new MockConfigHandler with default functions
func newMockConfigHandlerWithDefaults() *MockConfigHandler {
	return &MockConfigHandler{
		LoadConfigFunc: func(path string) error { return nil },
		GetStringFunc:  func(key string) (string, error) { return "", nil },
		GetIntFunc:     func(key string) (int, error) { return 0, nil },
		GetBoolFunc:    func(key string) (bool, error) { return false, nil },
		SetFunc:        func(key string, value interface{}) error { return nil },
		SaveConfigFunc: func(path string) error { return nil },
		SetDefaultFunc: func(context Context) {},
	}
}

func TestMockConfigHandler(t *testing.T) {
	t.Run("LoadConfig", func(t *testing.T) {
		mockLoadErr := errors.New("mock load config error")

		t.Run("LoadConfigWithPath", func(t *testing.T) {
			handler := newMockConfigHandlerWithDefaults()
			handler.LoadConfigFunc = func(path string) error { return mockLoadErr }
			err := handler.LoadConfig("some/path")
			assertError(t, err, mockLoadErr)
		})

		t.Run("LoadConfigWithNoFuncSet", func(t *testing.T) {
			handler := NewMockConfigHandler()
			err := handler.LoadConfig("some/path")
			assertError(t, err, nil)
		})
	})

	t.Run("GetString", func(t *testing.T) {
		mockGetErr := errors.New("mock get string error")

		t.Run("GetStringWithKey", func(t *testing.T) {
			handler := newMockConfigHandlerWithDefaults()
			handler.GetStringFunc = func(key string) (string, error) { return "", mockGetErr }
			_, err := handler.GetString("someKey")
			assertError(t, err, mockGetErr)
		})

		t.Run("GetStringWithNoFuncSet", func(t *testing.T) {
			handler := NewMockConfigHandler()
			value, err := handler.GetString("someKey")
			assertError(t, err, nil)
			assertEqual(t, "", value, "GetString")
		})

		t.Run("GetStringWithDefaultValue", func(t *testing.T) {
			handler := NewMockConfigHandler()
			defaultValue := "default"
			value, err := handler.GetString("someKey", defaultValue)
			assertError(t, err, nil)
			assertEqual(t, defaultValue, value, "GetString with default")
		})
	})

	t.Run("GetInt", func(t *testing.T) {
		mockGetErr := errors.New("mock get int error")

		t.Run("GetIntWithKey", func(t *testing.T) {
			handler := newMockConfigHandlerWithDefaults()
			handler.GetIntFunc = func(key string) (int, error) { return 0, mockGetErr }
			_, err := handler.GetInt("someKey")
			assertError(t, err, mockGetErr)
		})

		t.Run("GetIntWithNoFuncSet", func(t *testing.T) {
			handler := NewMockConfigHandler()
			value, err := handler.GetInt("someKey")
			assertError(t, err, nil)
			assertEqual(t, 0, value, "GetInt")
		})

		t.Run("GetIntWithDefaultValue", func(t *testing.T) {
			handler := NewMockConfigHandler()
			defaultValue := 42
			value, err := handler.GetInt("someKey", defaultValue)
			assertError(t, err, nil)
			assertEqual(t, defaultValue, value, "GetInt with default")
		})
	})

	t.Run("GetBool", func(t *testing.T) {
		mockGetErr := errors.New("mock get bool error")

		t.Run("GetBoolWithKey", func(t *testing.T) {
			handler := newMockConfigHandlerWithDefaults()
			handler.GetBoolFunc = func(key string) (bool, error) { return false, mockGetErr }
			_, err := handler.GetBool("someKey")
			assertError(t, err, mockGetErr)
		})

		t.Run("GetBoolWithNoFuncSet", func(t *testing.T) {
			handler := NewMockConfigHandler()
			value, err := handler.GetBool("someKey")
			assertError(t, err, nil)
			assertEqual(t, false, value, "GetBool")
		})

		t.Run("GetBoolWithDefaultValue", func(t *testing.T) {
			handler := NewMockConfigHandler()
			defaultValue := true
			value, err := handler.GetBool("someKey", defaultValue)
			assertError(t, err, nil)
			assertEqual(t, defaultValue, value, "GetBool with default")
		})
	})

	t.Run("Set", func(t *testing.T) {
		mockSetErr := errors.New("mock set value error")

		t.Run("SetWithKeyAndValue", func(t *testing.T) {
			handler := newMockConfigHandlerWithDefaults()
			handler.SetFunc = func(key string, value interface{}) error { return mockSetErr }
			err := handler.Set("someKey", "someValue")
			assertError(t, err, mockSetErr)
		})

		t.Run("SetWithNoFuncSet", func(t *testing.T) {
			handler := NewMockConfigHandler()
			err := handler.Set("someKey", "someValue")
			assertError(t, err, nil)
		})
	})

	t.Run("SaveConfig", func(t *testing.T) {
		mockSaveErr := errors.New("mock save config error")

		t.Run("SaveConfigWithPath", func(t *testing.T) {
			handler := newMockConfigHandlerWithDefaults()
			handler.SaveConfigFunc = func(path string) error { return mockSaveErr }
			err := handler.SaveConfig("some/path")
			assertError(t, err, mockSaveErr)
		})

		t.Run("SaveConfigWithNoFuncSet", func(t *testing.T) {
			handler := NewMockConfigHandler()
			err := handler.SaveConfig("some/path")
			assertError(t, err, nil)
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("GetWithKey", func(t *testing.T) {
			mockGetErr := errors.New("mock get error")
			handler := newMockConfigHandlerWithDefaults()
			handler.GetFunc = func(key string) (interface{}, error) { return nil, mockGetErr }
			_, err := handler.Get("someKey")
			assertError(t, err, mockGetErr)
		})

		t.Run("GetWithNoFuncSet", func(t *testing.T) {
			handler := NewMockConfigHandler()
			value, err := handler.Get("someKey")
			assertError(t, err, nil)
			assertEqual(t, nil, value, "Get")
		})
	})

	t.Run("SetDefault", func(t *testing.T) {
		t.Run("SetDefaultFuncCalled", func(t *testing.T) {
			// Arrange: Create a mock config handler and a flag to check if the function was called
			mockHandler := NewMockConfigHandler()
			called := false

			// Set the SetDefaultFunc to update the flag and check the parameters
			mockHandler.SetDefaultFunc = func(context Context) {
				called = true
				expectedValue := DefaultLocalConfig
				if !reflect.DeepEqual(context, expectedValue) {
					t.Errorf("Expected value %v, got %v", expectedValue, context)
				}
			}

			// Act: Call SetDefault
			mockHandler.SetDefault(DefaultLocalConfig)

			// Assert: Verify that the function was called
			if !called {
				t.Error("Expected SetDefaultFunc to be called")
			}
		})
	})
}
