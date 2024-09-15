package config

import (
	"errors"
	"testing"
)

func TestMockConfigHandler_LoadConfig(t *testing.T) {
	mockErr := errors.New("mock load config error")
	handler := &MockConfigHandler{
		LoadConfigFunc: func(path string) error {
			return mockErr
		},
	}

	err := handler.LoadConfig("some/path")
	if err != mockErr {
		t.Errorf("LoadConfig() error = %v, wantErr %v", err, mockErr)
	}
}

func TestMockConfigHandler_GetConfigValue(t *testing.T) {
	mockErr := errors.New("mock get config value error")
	handler := &MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			return "", mockErr
		},
	}

	_, err := handler.GetConfigValue("someKey")
	if err != mockErr {
		t.Errorf("GetConfigValue() error = %v, wantErr %v", err, mockErr)
	}
}

func TestMockConfigHandler_SetConfigValue(t *testing.T) {
	mockErr := errors.New("mock set config value error")
	handler := &MockConfigHandler{
		SetConfigValueFunc: func(key, value string) error {
			return mockErr
		},
	}

	err := handler.SetConfigValue("someKey", "someValue")
	if err != mockErr {
		t.Errorf("SetConfigValue() error = %v, wantErr %v", err, mockErr)
	}
}

func TestMockConfigHandler_SaveConfig(t *testing.T) {
	mockErr := errors.New("mock save config error")
	handler := &MockConfigHandler{
		SaveConfigFunc: func(path string) error {
			return mockErr
		},
	}

	err := handler.SaveConfig("some/path")
	if err != mockErr {
		t.Errorf("SaveConfig() error = %v, wantErr %v", err, mockErr)
	}
}

func TestMockConfigHandler_LoadConfig_Default(t *testing.T) {
	handler := &MockConfigHandler{}
	err := handler.LoadConfig("some/path")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}

func TestMockConfigHandler_GetConfigValue_Default(t *testing.T) {
	handler := &MockConfigHandler{}
	value, err := handler.GetConfigValue("someKey")
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	if value != "" {
		t.Errorf("Expected empty string, got %v", value)
	}
}

func TestMockConfigHandler_SetConfigValue_Default(t *testing.T) {
	handler := &MockConfigHandler{}
	err := handler.SetConfigValue("someKey", "someValue")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}

func TestMockConfigHandler_SaveConfig_Default(t *testing.T) {
	handler := &MockConfigHandler{}
	err := handler.SaveConfig("some/path")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}
