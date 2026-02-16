package blueprint

import (
	"path/filepath"
	"testing"
)

func TestOrdinalFromBasename(t *testing.T) {
	t.Run("Returns100ForConfigPrefix", func(t *testing.T) {
		if got := OrdinalFromBasename("config-cluster.yaml"); got != 100 {
			t.Errorf("OrdinalFromBasename(config-cluster.yaml) = %d, want 100", got)
		}
		if got := OrdinalFromBasename("config-.yaml"); got != 100 {
			t.Errorf("OrdinalFromBasename(config-.yaml) = %d, want 100", got)
		}
	})
	t.Run("Returns199ForProviderBase", func(t *testing.T) {
		if got := OrdinalFromBasename("provider-base.yaml"); got != 199 {
			t.Errorf("OrdinalFromBasename(provider-base.yaml) = %d, want 199", got)
		}
		if got := OrdinalFromBasename("platform-base.yaml"); got != 199 {
			t.Errorf("OrdinalFromBasename(platform-base.yaml) = %d, want 199", got)
		}
	})
	t.Run("Returns200ForProviderOrPlatformWithoutBase", func(t *testing.T) {
		if got := OrdinalFromBasename("provider-aws.yaml"); got != 200 {
			t.Errorf("OrdinalFromBasename(provider-aws.yaml) = %d, want 200", got)
		}
		if got := OrdinalFromBasename("platform-docker.yaml"); got != 200 {
			t.Errorf("OrdinalFromBasename(platform-docker.yaml) = %d, want 200", got)
		}
	})
	t.Run("Returns300ForOptionsPrefix", func(t *testing.T) {
		if got := OrdinalFromBasename("options-net.yaml"); got != 300 {
			t.Errorf("OrdinalFromBasename(options-net.yaml) = %d, want 300", got)
		}
	})
	t.Run("Returns400ForAddonsPrefix", func(t *testing.T) {
		if got := OrdinalFromBasename("addons-observability.yaml"); got != 400 {
			t.Errorf("OrdinalFromBasename(addons-observability.yaml) = %d, want 400", got)
		}
	})
	t.Run("Returns0ForNoMatch", func(t *testing.T) {
		if got := OrdinalFromBasename("custom.yaml"); got != 0 {
			t.Errorf("OrdinalFromBasename(custom.yaml) = %d, want 0", got)
		}
		if got := OrdinalFromBasename("feature.yaml"); got != 0 {
			t.Errorf("OrdinalFromBasename(feature.yaml) = %d, want 0", got)
		}
	})
	t.Run("Returns0ForEmptyBasename", func(t *testing.T) {
		if got := OrdinalFromBasename(""); got != 0 {
			t.Errorf("OrdinalFromBasename(\"\") = %d, want 0", got)
		}
	})
}

func TestOrdinalFromFacetPath(t *testing.T) {
	t.Run("UsesBasenameForOrdinal", func(t *testing.T) {
		path := filepath.Join("_template", "facets", "config-cluster.yaml")
		if got := OrdinalFromFacetPath(path); got != 100 {
			t.Errorf("OrdinalFromFacetPath(%s) = %d, want 100", path, got)
		}
	})
	t.Run("ProviderBaseInPathReturns199", func(t *testing.T) {
		path := filepath.Join("facets", "provider-base.yaml")
		if got := OrdinalFromFacetPath(path); got != 199 {
			t.Errorf("OrdinalFromFacetPath(%s) = %d, want 199", path, got)
		}
	})
}
