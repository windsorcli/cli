package config_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLoadSchemaFromBytes(t *testing.T) {
	t.Run("RejectsInvalidSchemas", func(t *testing.T) {
		// Given schemas with a missing, removed, or unknown dialect, or bad YAML
		cases := map[string]string{
			"missing": "type: object\n",
			"removed": "$schema: https://windsorcli.dev/draft/2026-02/schema\n",
			"unknown": "$schema: https://json-schema.org/draft-07/schema\n",
			"badYAML": "$schema: [",
		}
		for name, content := range cases {
			f := newFixture(t)

			// When the schema is loaded
			err := f.handler.LoadSchemaFromBytes([]byte(content))

			// Then an error is returned and no schema is kept
			if err == nil || f.handler.GetSchema() != nil {
				t.Errorf("%s: expected an error and no schema, got %v", name, err)
			}
		}
	})

	t.Run("MergesFragmentsWithTheLaterOneWinning", func(t *testing.T) {
		// Given two fragments that share one property
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n    default: us-east-1\n")
		f.loadSchema(t, "  region:\n    type: string\n    default: eu-west-1\n  zone:\n    type: string\n    default: a\n")

		// When defaults are read
		region := f.handler.GetString("region")
		zone := f.handler.GetString("zone")

		// Then both fragments apply and the later default wins
		if region != "eu-west-1" || zone != "a" {
			t.Errorf("unexpected defaults region=%q zone=%q", region, zone)
		}
	})

	t.Run("AddsTheSystemSchema", func(t *testing.T) {
		// Given a schema that defines only region
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n")

		// When the schema is read
		properties, _ := f.handler.GetSchema()["properties"].(map[string]any)

		// Then the system platform and workstation properties are present
		for _, key := range []string{"region", "platform", "workstation"} {
			if _, ok := properties[key]; !ok {
				t.Errorf("expected property %q", key)
			}
		}
	})
}

func TestSchemaFragmentMerge(t *testing.T) {
	t.Run("KeepsTheMostRestrictiveConstraints", func(t *testing.T) {
		// Given two fragments with overlapping bounds, enums, required lists, and closure
		f := newFixture(t)
		f.loadSchema(t, "  size:\n    type: integer\n    minimum: 1\n    maximum: 10\n  tier:\n    type: string\n    enum: [a, b, c]\n  labels:\n    type: object\nrequired: [size]\n")
		f.loadSchema(t, "  size:\n    type: integer\n    minimum: 5\n    maximum: 20\n  tier:\n    type: string\n    enum: [b, c, d]\n  labels:\n    type: object\n    additionalProperties: false\nrequired: [tier, size]\n")

		// When the merged schema is read
		schema := f.handler.GetSchema()
		properties := schema["properties"].(map[string]any)
		size := properties["size"].(map[string]any)
		tier := properties["tier"].(map[string]any)
		labels := properties["labels"].(map[string]any)

		// Then the tighter bound, the enum intersection, the required union, and closure win
		if fmt.Sprint(size["minimum"]) != "5" || fmt.Sprint(size["maximum"]) != "10" {
			t.Errorf("unexpected bounds min=%v max=%v", size["minimum"], size["maximum"])
		}
		if !reflect.DeepEqual(tier["enum"], []any{"b", "c"}) {
			t.Errorf("unexpected enum %v", tier["enum"])
		}
		if !reflect.DeepEqual(schema["required"], []any{"size", "tier"}) {
			t.Errorf("unexpected required %v", schema["required"])
		}
		if labels["additionalProperties"] != false {
			t.Errorf("expected closed labels, got %v", labels["additionalProperties"])
		}
	})

	t.Run("RejectsEveryValueForDisjointEnums", func(t *testing.T) {
		// Given two fragments whose enums share no member
		f := newFixture(t)
		f.loadSchema(t, "  tier:\n    type: string\n    enum: [a]\n")
		f.loadSchema(t, "  tier:\n    type: string\n    enum: [b]\n")
		_ = f.handler.Set("tier", "a")

		// When the values are validated
		err := f.handler.ValidateContextValues()

		// Then the value is rejected
		if err == nil {
			t.Error("expected a validation error")
		}
	})

	t.Run("ReplacesAPropertyWithAConflictingType", func(t *testing.T) {
		// Given two fragments that type one property differently
		f := newFixture(t)
		f.loadSchema(t, "  port:\n    type: string\n    pattern: ^[0-9]+$\n")
		f.loadSchema(t, "  port:\n    type: integer\n")

		// When the merged property is read
		port := f.handler.GetSchema()["properties"].(map[string]any)["port"]

		// Then the later fragment replaces it whole
		if !reflect.DeepEqual(port, map[string]any{"type": "integer"}) {
			t.Errorf("unexpected property %v", port)
		}
	})

	t.Run("MergesArrayItemSchemas", func(t *testing.T) {
		// Given two fragments that add properties to the same array items
		f := newFixture(t)
		f.loadSchema(t, "  nodes:\n    type: array\n    items:\n      type: object\n      properties:\n        name:\n          type: string\n")
		f.loadSchema(t, "  nodes:\n    type: array\n    items:\n      type: object\n      properties:\n        ip:\n          type: string\n")

		// When the merged item schema is read
		items := f.handler.GetSchema()["properties"].(map[string]any)["nodes"].(map[string]any)["items"].(map[string]any)

		// Then both item properties are present
		itemProps := items["properties"].(map[string]any)
		if _, ok := itemProps["name"]; !ok {
			t.Error("expected name")
		}
		if _, ok := itemProps["ip"]; !ok {
			t.Error("expected ip")
		}
	})
}

func TestLoadSchema(t *testing.T) {
	t.Run("LoadsAFile", func(t *testing.T) {
		// Given a schema file
		f := newFixture(t)
		f.write(t, "schema.yaml", testSchemaHeader+"type: object\nproperties:\n  region:\n    type: string\n    default: us-east-1\n")

		// When it is loaded
		err := f.handler.LoadSchema(filepath.Join(f.root, "schema.yaml"))

		// Then its defaults apply
		if err != nil || f.handler.GetString("region") != "us-east-1" {
			t.Errorf("expected the default, got err=%v region=%q", err, f.handler.GetString("region"))
		}
	})

	t.Run("ReturnsAnErrorForAMissingFile", func(t *testing.T) {
		// Given no schema file
		f := newFixture(t)

		// When a missing path is loaded
		err := f.handler.LoadSchema(filepath.Join(f.root, "missing.yaml"))

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})
}

func TestGetSchema(t *testing.T) {
	t.Run("ReturnsNilWithoutASchema", func(t *testing.T) {
		// Given a handler with no schema
		f := newFixture(t)

		// When the schema is read
		got := f.handler.GetSchema()

		// Then it is nil
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("ReturnsACopyOfTheTopLevel", func(t *testing.T) {
		// Given a loaded schema
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n")

		// When a top-level key is added to the returned map
		f.handler.GetSchema()["injected"] = true

		// Then the handler's schema is unchanged
		if _, ok := f.handler.GetSchema()["injected"]; ok {
			t.Error("expected the schema to be unchanged")
		}
	})
}

func TestSensitivePaths(t *testing.T) {
	t.Run("ListsNestedAndWildcardPaths", func(t *testing.T) {
		// Given a schema with a sensitive nested field and a sensitive free-form map
		f := newFixture(t)
		f.loadSchema(t, "  dns:\n    type: object\n    properties:\n      token:\n        type: string\n        sensitive: true\n  tokens:\n    type: object\n    additionalProperties:\n      type: string\n      sensitive: true\n")

		// When sensitive paths are read and checked
		paths := f.handler.GetSensitivePaths()

		// Then both paths are listed and matched
		if !reflect.DeepEqual(paths, []string{"dns.token", "tokens.*"}) {
			t.Errorf("unexpected paths %v", paths)
		}
		if !f.handler.IsSensitivePath("dns.token") || !f.handler.IsSensitivePath("tokens.github") || !f.handler.IsSensitivePath("tokens[github]") {
			t.Error("expected sensitive matches")
		}
		if f.handler.IsSensitivePath("dns.domain") || f.handler.IsSensitivePath("tokens.github.extra") {
			t.Error("expected non-sensitive paths to miss")
		}
	})

	t.Run("ReturnsNothingWithoutASchema", func(t *testing.T) {
		// Given a handler with no schema
		f := newFixture(t)

		// When sensitive paths are read
		paths := f.handler.GetSensitivePaths()

		// Then none are returned
		if paths != nil || f.handler.IsSensitivePath("anything") {
			t.Errorf("expected no sensitive paths, got %v", paths)
		}
	})
}

func TestValidateContextValues(t *testing.T) {
	t.Run("ChecksTheIDWithoutASchema", func(t *testing.T) {
		// Given ids that break and follow DNS label rules
		cases := map[string]bool{"abc-1": true, "": true, "1abc": false, "Abc": false, "abc-": false, "a--b": false, "a_b": false}
		for id, valid := range cases {
			f := newFixture(t)
			if id != "" {
				_ = f.handler.LoadConfigString("id: " + id + "\n")
			}

			// When the values are validated
			err := f.handler.ValidateContextValues()

			// Then only valid ids pass
			if (err == nil) != valid {
				t.Errorf("%q: expected valid=%v, got %v", id, valid, err)
			}
		}
	})

	t.Run("ListsSchemaViolations", func(t *testing.T) {
		// Given a value that breaks the system platform enum
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n")
		_ = f.handler.Set("platform", "bogus")

		// When the values are validated
		err := f.handler.ValidateContextValues()

		// Then the error lists the violation
		if err == nil || !strings.Contains(err.Error(), "context value validation failed:\n  - ") || !strings.Contains(err.Error(), "platform") {
			t.Errorf("expected a listed platform violation, got %v", err)
		}
	})

	t.Run("MergesRequiredErrorsAtOneLocation", func(t *testing.T) {
		// Given two required lists for one object that overlap
		f := newFixture(t)
		f.loadSchema(t, "  cluster:\n    type: object\n    properties:\n      driver:\n        type: string\n      storage:\n        type: string\n    allOf:\n      - required: [driver]\n      - required: [storage, driver]\n")
		_ = f.handler.Set("cluster.other", "x")

		// When the values are validated
		err := f.handler.ValidateContextValues()

		// Then one line names both missing fields
		if err == nil || !strings.Contains(err.Error(), "/cluster: required: Required properties 'driver', 'storage' are missing") {
			t.Fatalf("expected a merged required line, got %v", err)
		}
		if strings.Contains(err.Error(), "Required property 'driver' is missing") {
			t.Errorf("expected no separate driver line, got %v", err)
		}
	})

	t.Run("HidesCascadeErrorsBehindASpecificViolation", func(t *testing.T) {
		// Given a missing object field and a value outside the platform enum
		f := newFixture(t)
		f.loadSchema(t, "  identity:\n    type: object\nrequired: [identity]\n")
		_ = f.handler.Set("platform", "bogus")

		// When the values are validated
		err := f.handler.ValidateContextValues()

		// Then the enum violation shows and the rest are counted as hidden
		if err == nil || !strings.Contains(err.Error(), "/platform: enum: Value bogus should be one of the allowed values") {
			t.Fatalf("expected the enum violation, got %v", err)
		}
		if !strings.Contains(err.Error(), "other error(s) are hidden. Run with --debug to see them.") {
			t.Errorf("expected a hidden-error note, got %v", err)
		}
	})

	t.Run("RefersBackToAnEarlierValuesWarning", func(t *testing.T) {
		// Given an invalid values.yaml already warned about during load
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n")
		f.write(t, "contexts/local/values.yaml", "platform: bogus\n")
		f.load(t)

		// When the values are validated
		err := f.handler.ValidateContextValues()

		// Then the error points at the warning
		if err == nil || !strings.Contains(err.Error(), "see the values.yaml warning above") {
			t.Errorf("expected a reference to the warning, got %v", err)
		}
	})

	t.Run("PassesValidValues", func(t *testing.T) {
		// Given values that satisfy the schema
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n")
		_ = f.handler.Set("platform", "docker")

		// When the values are validated
		err := f.handler.ValidateContextValues()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestGenerateContextID(t *testing.T) {
	t.Run("GeneratesAValidIDAndMarksItForSaving", func(t *testing.T) {
		// Given an existing values.yaml without an id
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "platform: docker\n")
		f.load(t)

		// When an id is generated and config is saved with overwrite
		if err := f.handler.GenerateContextID(); err != nil {
			t.Fatalf("GenerateContextID: %v", err)
		}
		_ = f.handler.SaveConfig(true)

		// Then the id has the expected format and is written
		id := f.handler.GetString("id")
		if !regexp.MustCompile(`^w[a-z0-9]{7}$`).MatchString(id) {
			t.Errorf("unexpected id %q", id)
		}
		if got := f.readYAML(t, "contexts/local/values.yaml")["id"]; got != id {
			t.Errorf("expected saved id %q, got %v", id, got)
		}
	})

	t.Run("KeepsAnExistingID", func(t *testing.T) {
		// Given a handler with an id
		f := newFixture(t)
		_ = f.handler.Set("id", "abc")

		// When an id is generated
		_ = f.handler.GenerateContextID()

		// Then the id is unchanged
		if got := f.handler.GetString("id"); got != "abc" {
			t.Errorf("expected abc, got %q", got)
		}
	})
}

func TestGetContextValues(t *testing.T) {
	t.Run("MergesSchemaDefaultsUnderSetValues", func(t *testing.T) {
		// Given schema defaults and one overriding value
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n    default: us-east-1\n  zone:\n    type: string\n    default: a\n")
		_ = f.handler.Set("zone", "b")

		// When the context values are read
		values, err := f.handler.GetContextValues()

		// Then defaults fill gaps and set values win
		if err != nil || values["region"] != "us-east-1" || values["zone"] != "b" {
			t.Errorf("unexpected values %v (%v)", values, err)
		}
	})

	t.Run("SkipsSchemaDefaultsForTestContexts", func(t *testing.T) {
		// Given schema defaults under test contexts
		for _, name := range []string{"test", "test-a"} {
			f := newFixture(t)
			f.loadSchema(t, "  region:\n    type: string\n    default: us-east-1\n")
			handler := f.handler.WithContext(name)

			// When the context values are read
			values, _ := handler.GetContextValues()

			// Then no default is applied
			if _, ok := values["region"]; ok {
				t.Errorf("%s: expected no region default", name)
			}
		}
	})

	t.Run("AppliesSchemaDefaultsToATestContextOnRequest", func(t *testing.T) {
		// Given a test context that opts into schema defaults
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n    default: us-east-1\n")
		handler := f.handler.WithContext("test")
		handler.SetApplySchemaDefaults(true)

		// When the context values are read
		values, _ := handler.GetContextValues()

		// Then the default is applied
		if values["region"] != "us-east-1" {
			t.Errorf("expected region default, got %v", values["region"])
		}
	})

	t.Run("DerivesTheClusterDriverFromThePlatform", func(t *testing.T) {
		// Given each platform with no cluster driver
		cases := map[string]string{
			"platform: docker": "talos", "platform: incus": "talos", "platform: metal": "talos",
			"platform: hetzner": "talos", "platform: omni": "talos", "platform: hyperv": "talos",
			"platform: vsphere": "talos", "platform: aws": "eks", "platform: azure": "aks",
			"platform: gcp": "gke", "provider: aws": "eks",
		}
		for content, want := range cases {
			f := newFixture(t)
			_ = f.handler.LoadConfigString(content + "\n")

			// When the context values are read
			values, _ := f.handler.GetContextValues()

			// Then the driver is derived without changing stored config
			cluster, _ := values["cluster"].(map[string]any)
			if cluster["driver"] != want {
				t.Errorf("%s: expected driver %q, got %v", content, want, cluster["driver"])
			}
			if f.handler.Get("cluster.driver") != nil {
				t.Errorf("%s: expected stored config unchanged", content)
			}
		}
	})

	t.Run("KeepsAnExplicitClusterDriver", func(t *testing.T) {
		// Given an explicit driver on a platform with a derived default
		f := newFixture(t)
		_ = f.handler.LoadConfigString("platform: aws\ncluster:\n  driver: talos\n")

		// When the context values are read
		values, _ := f.handler.GetContextValues()

		// Then the explicit driver is kept
		if got := values["cluster"].(map[string]any)["driver"]; got != "talos" {
			t.Errorf("expected talos, got %v", got)
		}
	})

	t.Run("FillsWorkstationArchAndClusterNodes", func(t *testing.T) {
		// Given a handler with no workstation or cluster values
		f := newFixture(t)

		// When the context values are read
		values, _ := f.handler.GetContextValues()

		// Then the host arch and empty node maps are present
		wantArch := runtime.GOARCH
		if wantArch == "arm" {
			wantArch = "arm64"
		}
		if got := values["workstation"].(map[string]any)["arch"]; got != wantArch {
			t.Errorf("expected arch %q, got %v", wantArch, got)
		}
		cluster := values["cluster"].(map[string]any)
		for _, group := range []string{"controlplanes", "workers"} {
			nodes, ok := cluster[group].(map[string]any)["nodes"].(map[string]any)
			if !ok || len(nodes) != 0 {
				t.Errorf("%s: expected an empty nodes map, got %v", group, cluster[group])
			}
		}
	})

	t.Run("KeepsExistingArchAndNodes", func(t *testing.T) {
		// Given an explicit arch and worker nodes
		f := newFixture(t)
		_ = f.handler.LoadConfigString("workstation:\n  arch: amd64\ncluster:\n  workers:\n    nodes:\n      w1:\n        ip: 10.0.0.2\n")

		// When the context values are read
		values, _ := f.handler.GetContextValues()

		// Then the explicit values are kept and the missing group is filled
		if got := values["workstation"].(map[string]any)["arch"]; got != "amd64" {
			t.Errorf("expected amd64, got %v", got)
		}
		cluster := values["cluster"].(map[string]any)
		if _, ok := cluster["workers"].(map[string]any)["nodes"].(map[string]any)["w1"]; !ok {
			t.Error("expected worker w1 kept")
		}
		if _, ok := cluster["controlplanes"].(map[string]any)["nodes"].(map[string]any); !ok {
			t.Error("expected controlplanes nodes filled")
		}
	})
}

func TestGetSetValues(t *testing.T) {
	t.Run("ReturnsAnIndependentDeepCopy", func(t *testing.T) {
		// Given nested map and list values
		f := newFixture(t)
		_ = f.handler.LoadConfigString("dns:\n  domain: test\nnames: [a]\n")

		// When the copy's nested values change
		values := f.handler.GetSetValues()
		values["dns"].(map[string]any)["domain"] = "changed"
		values["names"].([]any)[0] = "changed"

		// Then the handler keeps its own values
		if got := f.handler.GetString("dns.domain"); got != "test" {
			t.Errorf("expected dns.domain test, got %q", got)
		}
		if got := f.handler.GetStringSlice("names"); got[0] != "a" {
			t.Errorf("expected names [a], got %v", got)
		}
	})

	t.Run("ReturnsSetValuesWithoutDefaults", func(t *testing.T) {
		// Given a schema default and one nested set value
		f := newFixture(t)
		f.loadSchema(t, "  region:\n    type: string\n    default: us-east-1\n")
		_ = f.handler.Set("dns.domain", "test")

		// When the set values are read and a top-level key is added
		values := f.handler.GetSetValues()
		values["added"] = true

		// Then only the set value is present and the handler is unchanged
		if !reflect.DeepEqual(values, map[string]any{"dns": map[string]any{"domain": "test"}, "added": true}) {
			t.Errorf("unexpected values %v", values)
		}
		if f.handler.Get("added") != nil {
			t.Error("expected handler unchanged")
		}
	})
}

// =============================================================================
// Test Package Functions
// =============================================================================

func TestRenderValuesWithDescriptionsOutput(t *testing.T) {
	t.Run("AnnotatesSetKeysAndListsUnsetOnes", func(t *testing.T) {
		// Given values and a schema with descriptions and an unset key
		values := map[string]any{"dns": map[string]any{"domain": "test"}, "id": "abc"}
		schema := map[string]any{"properties": map[string]any{
			"dns":    map[string]any{"type": "object", "description": "DNS settings", "properties": map[string]any{"domain": map[string]any{"type": "string", "description": "Base domain"}}},
			"id":     map[string]any{"type": "string"},
			"region": map[string]any{"type": "string", "description": "Cloud region"},
		}}

		// When the values are rendered
		got := config.RenderValuesWithDescriptions(values, schema)

		// Then set keys carry descriptions and the unset key is commented out
		want := "# DNS settings\ndns:\n  # Base domain\n  domain: test\n\nid: abc\n\n# Cloud region\n# region:\n"
		if got != want {
			t.Errorf("expected\n%s\ngot\n%s", want, got)
		}
	})

	t.Run("RendersPlainYAMLWithoutASchema", func(t *testing.T) {
		// Given values and no schema
		values := map[string]any{"names": []any{"a", "b"}, "empty": map[string]any{}}

		// When the values are rendered
		got := config.RenderValuesWithDescriptions(values, nil)

		// Then they render as YAML in key order
		want := "empty: {}\n\nnames:\n  - a\n  - b\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})
}

func TestRedactSensitiveValuesInPlace(t *testing.T) {
	t.Run("RedactsExistingPathsOnly", func(t *testing.T) {
		// Given values with a nested secret and a map of tokens
		values := map[string]any{
			"dns":    map[string]any{"token": "s1", "domain": "test"},
			"tokens": map[string]any{"a": "s2", "b": "s3"},
		}

		// When nested, wildcard, and missing paths are redacted
		got := config.RedactSensitiveValues(values, []string{"dns.token", "tokens.*", "missing.key"})

		// Then existing secrets are replaced and nothing is created
		want := map[string]any{
			"dns":    map[string]any{"token": config.SensitiveRedactionMarker, "domain": "test"},
			"tokens": map[string]any{"a": config.SensitiveRedactionMarker, "b": config.SensitiveRedactionMarker},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})
}

func TestDefaultTerraformBackendTypeByPlatform(t *testing.T) {
	t.Run("MapsEachPlatform", func(t *testing.T) {
		// Given each platform
		cases := map[string]string{
			"aws": "s3", "azure": "azurerm", "gcp": "gcs", "metal": "kubernetes", "docker": "kubernetes",
			"incus": "kubernetes", "hetzner": "kubernetes", "hyperv": "kubernetes", "vsphere": "kubernetes",
			"none": "", "": "",
		}
		for platform, want := range cases {
			// When its default backend is read
			got := config.DefaultTerraformBackendTypeForPlatform(platform)

			// Then it matches
			if got != want {
				t.Errorf("%q: expected %q, got %q", platform, want, got)
			}
		}
	})
}
