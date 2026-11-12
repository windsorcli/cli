package config_test

import (
	"reflect"
	"strings"
	"testing"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

// endpoint returns a registry entry with endpoint as its mirror.
func endpoint(value string) cliconfig.Registry {
	return cliconfig.Registry{Endpoint: &value}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewMirrors(t *testing.T) {
	t.Run("RemovesTheSchemeAndTheTrailingSlash", func(t *testing.T) {
		for _, value := range []string{"harbor.example.com:8443/windsor", "https://harbor.example.com:8443/windsor/"} {
			// Given an entry with an endpoint
			registries := map[string]cliconfig.Registry{"ghcr.io": endpoint(value)}

			// When the mirrors are read
			mirrors, err := config.NewMirrors(registries)

			// Then the mirror location has no scheme and is not insecure
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got := mirrors["ghcr.io"]; got != (config.Mirror{Location: "harbor.example.com:8443/windsor"}) {
				t.Errorf("endpoint %s: got mirror %+v", value, got)
			}
		}
	})

	t.Run("MarksAnHTTPEndpointInsecure", func(t *testing.T) {
		// Given an entry with an http endpoint
		registries := map[string]cliconfig.Registry{"ghcr.io": endpoint("http://registry.test:5000")}

		// When the mirrors are read
		mirrors, err := config.NewMirrors(registries)

		// Then the mirror is insecure
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := mirrors["ghcr.io"]; got != (config.Mirror{Location: "registry.test:5000", Insecure: true}) {
			t.Errorf("got mirror %+v", got)
		}
	})

	t.Run("UsesTheLocalHostAsTheUpstream", func(t *testing.T) {
		// Given an entry whose local value names a different host than its key
		local := "docker.io"
		entry := endpoint("harbor.example.com/docker")
		entry.Local = &local

		// When the mirrors are read
		mirrors, err := config.NewMirrors(map[string]cliconfig.Registry{"registry-1.docker.io": entry})

		// Then the mirror belongs to the local host
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if _, ok := mirrors.For("docker.io"); !ok {
			t.Errorf("expected a mirror for docker.io, got %+v", mirrors)
		}
	})

	t.Run("FailsWhenTwoEntriesGiveOneHostDifferentMirrors", func(t *testing.T) {
		// Given two entries for docker.io with different endpoints
		local := "docker.io"
		second := endpoint("b.example.com")
		second.Local = &local
		registries := map[string]cliconfig.Registry{"docker.io": endpoint("a.example.com"), "registry-1.docker.io": second}

		// When the mirrors are read
		_, err := config.NewMirrors(registries)

		// Then the error names the host
		if err == nil || !strings.Contains(err.Error(), "docker.io") {
			t.Errorf("expected an error that names docker.io, got %v", err)
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMirrors_For(t *testing.T) {
	t.Run("UsesTheCatchAllForAHostWithNoEntry", func(t *testing.T) {
		// Given a catch-all mirror
		mirrors, _ := config.NewMirrors(map[string]cliconfig.Registry{"*": endpoint("harbor.example.com")})

		// When a host with no entry is looked up
		mirror, ok := mirrors.For("quay.io")

		// Then it gets the catch-all mirror
		if !ok || mirror.Location != "harbor.example.com" {
			t.Errorf("got %+v, %v", mirror, ok)
		}
	})

	t.Run("AnEntryWithNoEndpointTurnsOffTheCatchAll", func(t *testing.T) {
		// Given a catch-all mirror and a proxy entry with no endpoint
		remote := "https://ghcr.io"
		mirrors, _ := config.NewMirrors(map[string]cliconfig.Registry{
			"*":       endpoint("harbor.example.com"),
			"ghcr.io": {Remote: &remote},
		})

		// When the proxy host is looked up
		_, ok := mirrors.For("ghcr.io")

		// Then it has no mirror
		if ok {
			t.Error("expected no mirror for ghcr.io")
		}
	})

	t.Run("GivesNoMirrorForTheMirrorHost", func(t *testing.T) {
		// Given a catch-all mirror
		mirrors, _ := config.NewMirrors(map[string]cliconfig.Registry{"*": endpoint("harbor.example.com/windsor")})

		// When the mirror host is looked up
		_, ok := mirrors.For("harbor.example.com")

		// Then it has no mirror, so nothing points at itself
		if ok {
			t.Error("expected no mirror for the mirror host")
		}
	})
}

func TestMirrors_OCIURL(t *testing.T) {
	t.Run("RewritesTheHostAndKeepsThePathAndTag", func(t *testing.T) {
		// Given a mirror for ghcr.io
		mirrors, _ := config.NewMirrors(map[string]cliconfig.Registry{"ghcr.io": endpoint("https://harbor.example.com/windsor")})

		// When an oci:// URL on ghcr.io is resolved
		url, insecure := mirrors.OCIURL("oci://ghcr.io/windsorcli/core:v0.5.0")

		// Then it names the mirror
		if url != "oci://harbor.example.com/windsor/windsorcli/core:v0.5.0" || insecure {
			t.Errorf("got %q, insecure %v", url, insecure)
		}
	})

	t.Run("ReportsAnHTTPMirror", func(t *testing.T) {
		// Given an http mirror
		mirrors, _ := config.NewMirrors(map[string]cliconfig.Registry{"*": endpoint("http://registry.test:5000")})

		// When an oci:// URL is resolved
		url, insecure := mirrors.OCIURL("oci://ghcr.io/windsorcli/core")

		// Then the mirror is insecure
		if url != "oci://registry.test:5000/windsorcli/core" || !insecure {
			t.Errorf("got %q, insecure %v", url, insecure)
		}
	})

	t.Run("KeepsAURLWithNoMirror", func(t *testing.T) {
		// Given no mirrors
		mirrors, _ := config.NewMirrors(nil)

		// When URLs are resolved
		for _, in := range []string{"oci://ghcr.io/windsorcli/core:v0.5.0", "https://github.com/windsorcli/core", "oci://ghcr.io"} {
			url, insecure := mirrors.OCIURL(in)

			// Then each URL is unchanged
			if url != in || insecure {
				t.Errorf("%s: got %q, insecure %v", in, url, insecure)
			}
		}
	})
}

func TestConfigHandler_CheckRegistriesKeys(t *testing.T) {
	t.Run("AcceptsOnlyRegistries", func(t *testing.T) {
		// Given a context that sets registries
		f := newFixture(t)
		_ = f.handler.LoadConfigString("registries:\n  ghcr.io:\n    endpoint: harbor.example.com\n")

		// When the keys are checked
		legacy, err := f.handler.CheckRegistriesKeys()

		// Then there is no warning and no error
		if legacy || err != nil {
			t.Errorf("got legacy %v, err %v", legacy, err)
		}
	})

	t.Run("ReportsOnlyDockerRegistries", func(t *testing.T) {
		// Given a context that sets only docker.registries
		f := newFixture(t)
		_ = f.handler.LoadConfigString("docker:\n  registries:\n    ghcr.io:\n      endpoint: harbor.example.com\n")

		// When the keys are checked
		legacy, err := f.handler.CheckRegistriesKeys()

		// Then the deprecated key is reported
		if !legacy || err != nil {
			t.Errorf("got legacy %v, err %v", legacy, err)
		}
	})

	t.Run("IgnoresSchemaDefaults", func(t *testing.T) {
		// Given a schema with a docker.registries default and a context that sets registries
		f := newFixture(t)
		f.loadSchema(t, "  docker:\n    type: object\n    properties:\n      registries:\n        type: object\n        default:\n          quay.io: {}\n")
		_ = f.handler.LoadConfigString("registries:\n  ghcr.io:\n    endpoint: harbor.example.com\n")

		// When the keys are checked
		legacy, err := f.handler.CheckRegistriesKeys()

		// Then the schema default is not a set key
		if legacy || err != nil {
			t.Errorf("got legacy %v, err %v", legacy, err)
		}
	})

	t.Run("FailsWhenBothKeysAreSet", func(t *testing.T) {
		// Given a context that sets both keys
		f := newFixture(t)
		_ = f.handler.LoadConfigString("registries:\n  ghcr.io: {}\ndocker:\n  registries:\n    quay.io: {}\n")

		// When the keys are checked
		_, err := f.handler.CheckRegistriesKeys()

		// Then the error names both keys
		if err == nil || !strings.Contains(err.Error(), "docker.registries") {
			t.Errorf("expected an error that names both keys, got %v", err)
		}
	})
}

func TestRegistriesMerge(t *testing.T) {
	legacy := "docker:\n  registries:\n    ghcr.io:\n      endpoint: harbor.example.com\n"
	want := map[string]any{"ghcr.io": map[string]any{"endpoint": "harbor.example.com"}}

	t.Run("GetHeaderReadsDockerRegistries", func(t *testing.T) {
		// Given a context that sets only docker.registries
		f := newFixture(t)
		_ = f.handler.LoadConfigString(legacy)

		// When the header is read
		got := f.handler.GetHeader().Registries["ghcr.io"]

		// Then Registries holds the docker.registries entry
		if got.Endpoint == nil || *got.Endpoint != "harbor.example.com" {
			t.Errorf("expected the docker.registries entry, got %+v", got)
		}
	})

	t.Run("GetHeaderPrefersRegistries", func(t *testing.T) {
		// Given a context that sets registries
		f := newFixture(t)
		_ = f.handler.LoadConfigString("registries:\n  quay.io:\n    endpoint: harbor.example.com\n")

		// When the header is read
		got := f.handler.GetHeader().Registries

		// Then Registries holds the registries entry
		if _, ok := got["quay.io"]; !ok || len(got) != 1 {
			t.Errorf("expected only quay.io, got %+v", got)
		}
	})

	t.Run("GetContextValuesSetsRegistries", func(t *testing.T) {
		// Given a context that sets only docker.registries
		f := newFixture(t)
		_ = f.handler.LoadConfigString(legacy)

		// When the context values are read
		values, err := f.handler.GetContextValues()

		// Then registries holds the docker.registries map
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !reflect.DeepEqual(values["registries"], want) {
			t.Errorf("expected registries %v, got %v", want, values["registries"])
		}
	})

	t.Run("GetSetValuesSetsRegistriesAndKeepsTheData", func(t *testing.T) {
		// Given a context that sets only docker.registries
		f := newFixture(t)
		_ = f.handler.LoadConfigString(legacy)

		// When the set values are read
		values := f.handler.GetSetValues()

		// Then registries holds the map, and the handler data does not change
		if !reflect.DeepEqual(values["registries"], want) {
			t.Errorf("expected registries %v, got %v", want, values["registries"])
		}
		if f.handler.Get("registries") != nil {
			t.Error("expected the handler data to have no registries key")
		}
	})
}
