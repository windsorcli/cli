package virt

import (
	"bytes"
	"runtime"
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	// Given NewShims is called
	shims := NewShims()

	// Then all fields should be set
	if shims.Setenv == nil {
		t.Error("Setenv should be set")
	}
	if shims.JsonUnmarshal == nil {
		t.Error("JsonUnmarshal should be set")
	}
	if shims.UserHomeDir == nil {
		t.Error("UserHomeDir should be set")
	}
	if shims.MkdirAll == nil {
		t.Error("MkdirAll should be set")
	}
	if shims.WriteFile == nil {
		t.Error("WriteFile should be set")
	}
	if shims.Rename == nil {
		t.Error("Rename should be set")
	}
	if shims.Stat == nil {
		t.Error("Stat should be set")
	}
	if shims.GOARCH == nil {
		t.Error("GOARCH should be set")
	}
	if shims.NumCPU == nil {
		t.Error("NumCPU should be set")
	}
	if shims.VirtualMemory == nil {
		t.Error("VirtualMemory should be set")
	}
	if shims.YamlMarshal == nil {
		t.Error("YamlMarshal should be set")
	}
	if shims.NewYAMLEncoder == nil {
		t.Error("NewYAMLEncoder should be set")
	}

	// And GOARCH should return the correct value
	if shims.GOARCH() != runtime.GOARCH {
		t.Errorf("GOARCH should return %s, got %s", runtime.GOARCH, shims.GOARCH())
	}

	// And NumCPU should return a positive value
	if shims.NumCPU() <= 0 {
		t.Errorf("NumCPU should return a positive value, got %d", shims.NumCPU())
	}

	// And VirtualMemory should return valid memory stats
	vmStat, err := shims.VirtualMemory()
	if err != nil {
		t.Errorf("VirtualMemory should not return an error, got %v", err)
	}
	if vmStat == nil {
		t.Error("VirtualMemory should return non-nil stats")
	}
	if vmStat.Total == 0 {
		t.Error("VirtualMemory should return non-zero total memory")
	}

	// And NewYAMLEncoder should create a valid encoder
	var buf bytes.Buffer
	encoder := shims.NewYAMLEncoder(&buf)
	if encoder == nil {
		t.Error("NewYAMLEncoder should return a non-nil encoder")
	}
	if err := encoder.Encode(map[string]string{"test": "value"}); err != nil {
		t.Errorf("Encoder.Encode should not return an error, got %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Errorf("Encoder.Close should not return an error, got %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encoder should write data to the buffer")
	}
}
