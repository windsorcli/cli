package env

import (
	"testing"
)

func TestNewShims(t *testing.T) {
	t.Run("CreatesShimsWithAllFunctions", func(t *testing.T) {
		shims := NewShims()

		// Verify all shim functions are initialized
		if shims.Stat == nil {
			t.Error("Stat shim not initialized")
		}
		if shims.Getwd == nil {
			t.Error("Getwd shim not initialized")
		}
		if shims.Glob == nil {
			t.Error("Glob shim not initialized")
		}
		if shims.WriteFile == nil {
			t.Error("WriteFile shim not initialized")
		}
		if shims.ReadDir == nil {
			t.Error("ReadDir shim not initialized")
		}
		if shims.YamlUnmarshal == nil {
			t.Error("YamlUnmarshal shim not initialized")
		}
		if shims.YamlMarshal == nil {
			t.Error("YamlMarshal shim not initialized")
		}
		if shims.Remove == nil {
			t.Error("Remove shim not initialized")
		}
		if shims.RemoveAll == nil {
			t.Error("RemoveAll shim not initialized")
		}
		if shims.CryptoRandRead == nil {
			t.Error("CryptoRandRead shim not initialized")
		}
		if shims.Goos == nil {
			t.Error("Goos shim not initialized")
		}
		if shims.UserHomeDir == nil {
			t.Error("UserHomeDir shim not initialized")
		}
		if shims.MkdirAll == nil {
			t.Error("MkdirAll shim not initialized")
		}
		if shims.ReadFile == nil {
			t.Error("ReadFile shim not initialized")
		}
		if shims.LookPath == nil {
			t.Error("LookPath shim not initialized")
		}
		if shims.LookupEnv == nil {
			t.Error("LookupEnv shim not initialized")
		}
		if shims.Environ == nil {
			t.Error("Environ shim not initialized")
		}
		if shims.Getenv == nil {
			t.Error("Getenv shim not initialized")
		}
	})
}
