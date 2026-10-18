package config_test

import (
	"reflect"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGet(t *testing.T) {
	t.Run("ReadsDottedAndBracketPaths", func(t *testing.T) {
		// Given nested values
		f := newFixture(t)
		_ = f.handler.LoadConfigString("cluster:\n  nodes:\n    cp-1.lab:\n      ip: 10.0.0.1\n")

		// When paths are read in each notation
		dotted := f.handler.Get("cluster.nodes")
		bracket := f.handler.Get("cluster.nodes[cp-1.lab].ip")

		// Then both resolve
		if dotted == nil || bracket != "10.0.0.1" {
			t.Errorf("unexpected values dotted=%v bracket=%v", dotted, bracket)
		}
	})

	t.Run("ReturnsNilForEmptyAndMissingPaths", func(t *testing.T) {
		// Given a handler with one value
		f := newFixture(t)
		_ = f.handler.Set("platform", "docker")

		// When empty, missing, and through-scalar paths are read
		got := []any{f.handler.Get(""), f.handler.Get("missing"), f.handler.Get("platform.child")}

		// Then each is nil
		for i, v := range got {
			if v != nil {
				t.Errorf("case %d: expected nil, got %v", i, v)
			}
		}
	})

	t.Run("FallsBackToSchemaDefaults", func(t *testing.T) {
		// Given a schema with top-level and nested defaults
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n    default: us-east-1\n  dns:\n    type: object\n    properties:\n      domain:\n        type: string\n        default: test\n")

		// When unset paths are read
		region := f.handler.Get("region")
		domain := f.handler.Get("dns.domain")

		// Then the schema defaults are returned
		if region != "us-east-1" || domain != "test" {
			t.Errorf("expected defaults, got region=%v domain=%v", region, domain)
		}
	})

	t.Run("PrefersSetValuesOverSchemaDefaults", func(t *testing.T) {
		// Given a schema default and a set value
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n    default: us-east-1\n")
		_ = f.handler.Set("region", "eu-west-1")

		// When the path is read
		got := f.handler.Get("region")

		// Then the set value wins
		if got != "eu-west-1" {
			t.Errorf("expected eu-west-1, got %v", got)
		}
	})
}

func TestTypedGetters(t *testing.T) {
	t.Run("ReturnDefaultsForMissingKeys", func(t *testing.T) {
		// Given an empty handler
		f := newFixture(t)

		// When missing keys are read with and without defaults
		h := f.handler

		// Then the default or the zero value is returned
		if h.GetString("x") != "" || h.GetString("x", "d") != "d" {
			t.Error("unexpected GetString default")
		}
		if h.GetInt("x") != 0 || h.GetInt("x", 7) != 7 {
			t.Error("unexpected GetInt default")
		}
		if h.GetBool("x") || !h.GetBool("x", true) {
			t.Error("unexpected GetBool default")
		}
		if got := h.GetStringSlice("x"); got == nil || len(got) != 0 {
			t.Errorf("expected empty slice, got %#v", got)
		}
		if got := h.GetStringSlice("x", []string{"a"}); !reflect.DeepEqual(got, []string{"a"}) {
			t.Errorf("unexpected GetStringSlice default %v", got)
		}
		if got := h.GetStringMap("x"); got == nil || len(got) != 0 {
			t.Errorf("expected empty map, got %#v", got)
		}
		if got := h.GetStringMap("x", map[string]string{"a": "b"}); got["a"] != "b" {
			t.Errorf("unexpected GetStringMap default %v", got)
		}
	})

	t.Run("ConvertLoadedValues", func(t *testing.T) {
		// Given values of several types loaded from YAML
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "count: 3\nport: \"8080\"\nratio: 1.5\nenabled: true\nnames: [a, 2]\nlabels:\n  tier: web\n  replicas: 2\n")
		f.load(t)
		h := f.handler

		// When each getter reads its key
		gotString := h.GetString("count")
		gotInt := h.GetInt("count")
		gotIntFromString := h.GetInt("port")
		gotBool := h.GetBool("enabled")
		gotSlice := h.GetStringSlice("names")
		gotMap := h.GetStringMap("labels")

		// Then each value converts to the getter's type
		if gotString != "3" || gotInt != 3 || gotIntFromString != 8080 || !gotBool {
			t.Errorf("unexpected scalars string=%q int=%d intFromString=%d bool=%v", gotString, gotInt, gotIntFromString, gotBool)
		}
		if !reflect.DeepEqual(gotSlice, []string{"a", "2"}) {
			t.Errorf("unexpected slice %v", gotSlice)
		}
		if !reflect.DeepEqual(gotMap, map[string]string{"tier": "web", "replicas": "2"}) {
			t.Errorf("unexpected map %v", gotMap)
		}
	})

	t.Run("FallBackWhenTheTypeDoesNotConvert", func(t *testing.T) {
		// Given values that do not match the getters' types
		f := newFixture(t)
		_ = f.handler.LoadConfigString("ratio: 1.5\nword: abc\nflag: yes-please\nscalar: x\n")
		h := f.handler

		// When the getters read them with defaults
		gotFloat := h.GetInt("ratio", 9)
		gotWord := h.GetInt("word", 9)
		gotFlag := h.GetBool("flag", true)
		gotSlice := h.GetStringSlice("scalar", []string{"d"})
		gotMap := h.GetStringMap("scalar", map[string]string{"d": "d"})

		// Then GetInt uses its default, and the others return their zero value
		if gotFloat != 9 || gotWord != 9 {
			t.Errorf("expected GetInt default, got %d and %d", gotFloat, gotWord)
		}
		if gotFlag {
			t.Error("expected GetBool false for a non-bool value")
		}
		if len(gotSlice) != 0 || len(gotMap) != 0 {
			t.Errorf("expected empty slice and map, got %v and %v", gotSlice, gotMap)
		}
	})
}

func TestSet(t *testing.T) {
	t.Run("RejectsInvalidPaths", func(t *testing.T) {
		// Given a handler
		f := newFixture(t)

		// When malformed paths are set
		for _, path := range []string{"", ".a", "a.", "a..b"} {
			err := f.handler.Set(path, "x")

			// Then each returns an error
			if err == nil {
				t.Errorf("%q: expected an error", path)
			}
		}
	})

	t.Run("ConvertsStringsByPattern", func(t *testing.T) {
		// Given a handler with no schema
		f := newFixture(t)

		// When string values are set
		_ = f.handler.Set("a", "TRUE")
		_ = f.handler.Set("b", "42")
		_ = f.handler.Set("c", "1.5")
		_ = f.handler.Set("d", "text")

		// Then each is stored as its inferred type
		want := map[string]any{"a": true, "b": 42, "c": 1.5, "d": "text"}
		for key, value := range want {
			if got := f.handler.Get(key); got != value {
				t.Errorf("%s: expected %#v, got %#v", key, value, got)
			}
		}
	})

	t.Run("ConvertsStringsByTheSchemaTypeAtThePath", func(t *testing.T) {
		// Given a schema with typed top-level, nested, and free-form map fields
		f := newFixture(t)
		f.loadSchema(t, "  version:\n    type: string\n  count:\n    type: integer\n  cluster:\n    type: object\n    properties:\n      version:\n        type: string\n      enabled:\n        type: boolean\n  labels:\n    type: object\n    additionalProperties:\n      type: string\n")

		// When string values are set at each path
		_ = f.handler.Set("version", "1.30")
		_ = f.handler.Set("count", "3")
		_ = f.handler.Set("cluster.version", "1.30")
		_ = f.handler.Set("cluster.enabled", "true")
		_ = f.handler.Set("labels.tier", "2")

		// Then each value takes the type the schema gives its path
		want := map[string]any{"version": "1.30", "count": 3, "cluster.version": "1.30", "cluster.enabled": true, "labels.tier": "2"}
		for path, value := range want {
			if got := f.handler.Get(path); got != value {
				t.Errorf("%s: expected %#v, got %#v", path, value, got)
			}
		}
		if err := f.handler.ValidateContextValues(); err != nil {
			t.Errorf("expected valid values, got %v", err)
		}
	})

	t.Run("ConvertsAgainWhenASchemaLoadsLater", func(t *testing.T) {
		// Given a string value set before any schema types its path
		f := newFixture(t)
		_ = f.handler.Set("version", "1.30")

		// When a schema that types the path as a string loads
		f.loadSchema(t, "  version:\n    type: string\n")

		// Then the value is the original string
		if got := f.handler.Get("version"); got != "1.30" {
			t.Errorf("expected \"1.30\", got %#v", got)
		}
	})

	t.Run("KeepsALaterNonStringValueWhenASchemaLoads", func(t *testing.T) {
		// Given a string value that a later Set replaced with a number
		f := newFixture(t)
		_ = f.handler.Set("version", "1.30")
		_ = f.handler.Set("version", 5)

		// When a schema that types the path as a string loads
		f.loadSchema(t, "  version:\n    type: string\n")

		// Then the later value is kept
		if got := f.handler.Get("version"); got != 5 {
			t.Errorf("expected 5, got %#v", got)
		}
	})

	t.Run("KeepsNonStringValues", func(t *testing.T) {
		// Given a handler
		f := newFixture(t)

		// When a non-string value is set
		_ = f.handler.Set("names", []string{"a", "b"})

		// Then it is stored unchanged
		if got := f.handler.GetStringSlice("names"); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Errorf("unexpected value %v", got)
		}
	})

	t.Run("CreatesAndReplacesIntermediateMaps", func(t *testing.T) {
		// Given a scalar at the parent path
		f := newFixture(t)
		_ = f.handler.Set("cluster", "scalar")

		// When a nested path is set under it
		_ = f.handler.Set("cluster.workers.count", 2)

		// Then the scalar is replaced by a map
		if got := f.handler.GetInt("cluster.workers.count"); got != 2 {
			t.Errorf("expected 2, got %d", got)
		}
	})
}

func TestGetTerraformBackendType(t *testing.T) {
	t.Run("ResolvesExplicitThenPlatformDefault", func(t *testing.T) {
		// Given contexts with each combination of backend and platform
		cases := []struct {
			config string
			want   string
		}{
			{"terraform:\n  backend:\n    type: kubernetes\nplatform: aws\n", "kubernetes"},
			{"platform: aws\n", "s3"},
			{"platform: azure\n", "azurerm"},
			{"platform: gcp\n", "gcs"},
			{"platform: docker\n", "local"},
			{"provider: aws\n", "s3"},
			{"platform: none\n", "local"},
			{"", "local"},
		}
		for _, c := range cases {
			f := newFixture(t)
			_ = f.handler.LoadConfigString(c.config)

			// When the backend type is read
			got := f.handler.GetTerraformBackendType()

			// Then it matches the expected backend
			if got != c.want {
				t.Errorf("%q: expected %q, got %q", c.config, c.want, got)
			}
		}
	})
}
