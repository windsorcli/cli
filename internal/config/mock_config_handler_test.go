package config

import (
	"errors"
	"testing"
)

func TestMockConfigHandler_LoadConfig(t *testing.T) {
	mockErr := errors.New("mock load config error")
	handler := &MockConfigHandler{LoadConfigErr: mockErr}

	err := handler.LoadConfig("some/path")
	if err != mockErr {
		t.Errorf("LoadConfig() error = %v, wantErr %v", err, mockErr)
	}
}

func TestMockConfigHandler_GetConfigValue(t *testing.T) {
	mockErr := errors.New("mock get config value error")
	handler := &MockConfigHandler{GetConfigValueErr: mockErr}

	_, err := handler.GetConfigValue("someKey")
	if err != mockErr {
		t.Errorf("GetConfigValue() error = %v, wantErr %v", err, mockErr)
	}
}

func TestMockConfigHandler_SetConfigValue(t *testing.T) {
	mockErr := errors.New("mock set config value error")
	handler := &MockConfigHandler{SetConfigValueErr: mockErr}

	err := handler.SetConfigValue("someKey", "someValue")
	if err != mockErr {
		t.Errorf("SetConfigValue() error = %v, wantErr %v", err, mockErr)
	}
}

func TestMockConfigHandler_SaveConfig(t *testing.T) {
	mockErr := errors.New("mock save config error")
	handler := &MockConfigHandler{SaveConfigErr: mockErr}

	err := handler.SaveConfig("some/path")
	if err != mockErr {
		t.Errorf("SaveConfig() error = %v, wantErr %v", err, mockErr)
	}
}
