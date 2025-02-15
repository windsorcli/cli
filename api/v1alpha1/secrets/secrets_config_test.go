package secrets

import (
	"testing"
)

func TestSecretsConfig_Merge_NonNilOverlay(t *testing.T) {
	base := &SecretsConfig{
		Enabled:  ptrBool(false),
		Provider: "base_provider",
	}
	overlay := &SecretsConfig{
		Enabled:  ptrBool(true),
		Provider: "overlay_provider",
	}

	base.Merge(overlay)

	if *base.Enabled != true {
		t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
	}
	if base.Provider != "overlay_provider" {
		t.Errorf("Expected Provider to be 'overlay_provider', got %v", base.Provider)
	}
}

func TestSecretsConfig_Merge_NilOverlay(t *testing.T) {
	base := &SecretsConfig{
		Enabled:  ptrBool(false),
		Provider: "base_provider",
	}
	overlay := &SecretsConfig{}

	base.Merge(overlay)

	if *base.Enabled != false {
		t.Errorf("Expected Enabled to be false, got %v", *base.Enabled)
	}
	if base.Provider != "base_provider" {
		t.Errorf("Expected Provider to be 'base_provider', got %v", base.Provider)
	}
}

func TestSecretsConfig_Copy_NonNil(t *testing.T) {
	original := &SecretsConfig{
		Enabled:  ptrBool(true),
		Provider: "provider",
	}

	copy := original.Copy()

	if *copy.Enabled != true {
		t.Errorf("Expected Enabled to be true, got %v", *copy.Enabled)
	}
	if copy.Provider != "provider" {
		t.Errorf("Expected Provider to be 'provider', got %v", copy.Provider)
	}
}

func TestSecretsConfig_Copy_Nil(t *testing.T) {
	var original *SecretsConfig

	copy := original.Copy()

	if copy != nil {
		t.Errorf("Expected copy to be nil, got %v", copy)
	}
}
