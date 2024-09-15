package config

import (
	"errors"
	"testing"
)

func TestMockConfigHandler(t *testing.T) {
	mockLoadErr := errors.New("mock load config error")
	mockGetErr := errors.New("mock get config value error")
	mockSetErr := errors.New("mock set config value error")
	mockSaveErr := errors.New("mock save config error")

	handler := &MockConfigHandler{
		LoadConfigFunc: func(path string) error {
			return mockLoadErr
		},
		GetConfigValueFunc: func(key string) (string, error) {
			return "", mockGetErr
		},
		SetConfigValueFunc: func(key, value string) error {
			return mockSetErr
		},
		SaveConfigFunc: func(path string) error {
			return mockSaveErr
		},
	}

	tests := []struct {
		name     string
		testFunc func() error
		wantErr  error
	}{
		{
			name: "LoadConfig",
			testFunc: func() error {
				return handler.LoadConfig("some/path")
			},
			wantErr: mockLoadErr,
		},
		{
			name: "GetConfigValue",
			testFunc: func() error {
				_, err := handler.GetConfigValue("someKey")
				return err
			},
			wantErr: mockGetErr,
		},
		{
			name: "SetConfigValue",
			testFunc: func() error {
				return handler.SetConfigValue("someKey", "someValue")
			},
			wantErr: mockSetErr,
		},
		{
			name: "SaveConfig",
			testFunc: func() error {
				return handler.SaveConfig("some/path")
			},
			wantErr: mockSaveErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.testFunc(); err != tt.wantErr {
				t.Errorf("%s() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}

	defaultHandler := &MockConfigHandler{}

	defaultTests := []struct {
		name      string
		testFunc  func() (string, error)
		wantValue string
		wantErr   error
	}{
		{
			name: "LoadConfig_Default",
			testFunc: func() (string, error) {
				return "", defaultHandler.LoadConfig("some/path")
			},
			wantValue: "",
			wantErr:   nil,
		},
		{
			name: "GetConfigValue_Default",
			testFunc: func() (string, error) {
				return defaultHandler.GetConfigValue("someKey")
			},
			wantValue: "",
			wantErr:   nil,
		},
		{
			name: "SetConfigValue_Default",
			testFunc: func() (string, error) {
				return "", defaultHandler.SetConfigValue("someKey", "someValue")
			},
			wantValue: "",
			wantErr:   nil,
		},
		{
			name: "SaveConfig_Default",
			testFunc: func() (string, error) {
				return "", defaultHandler.SaveConfig("some/path")
			},
			wantValue: "",
			wantErr:   nil,
		},
	}

	for _, tt := range defaultTests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := tt.testFunc()
			if err != tt.wantErr {
				t.Errorf("%s() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if value != tt.wantValue {
				t.Errorf("%s() value = %v, wantValue %v", tt.name, value, tt.wantValue)
			}
		})
	}
}
