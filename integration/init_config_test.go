//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/integration/helpers"
)

// readYAMLFile loads a YAML file into a map for structured assertions.
func readYAMLFile(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %s to be readable: %v", path, err)
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		t.Fatalf("expected file %s to parse as YAML: %v", path, err)
	}

	if data == nil {
		return map[string]any{}
	}

	return data
}

// hasFile reports whether path exists and is readable via stat.
func hasFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// getPathValue returns a nested map value by dot path and whether it exists.
func getPathValue(data map[string]any, path ...string) (any, bool) {
	current := any(data)
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[key]
		if !ok {
			return nil, false
		}
		current = next
	}

	return current, true
}

func TestInitConfig_PersistsSetValues(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local", "--set", "dns.enabled=false", "--set", "cluster.endpoint=https://127.0.0.1:6443"}, env)
	if err != nil {
		t.Fatalf("init local with set flags: %v\nstderr: %s", err, stderr)
	}

	valuesPath := filepath.Join(dir, "contexts", "local", "values.yaml")
	values := readYAMLFile(t, valuesPath)
	if dnsEnabled, ok := getPathValue(values, "dns", "enabled"); !ok || dnsEnabled != false {
		t.Errorf("expected dns.enabled=false in values.yaml, got %v", dnsEnabled)
	}
	if endpoint, ok := getPathValue(values, "cluster", "endpoint"); !ok || endpoint != "https://127.0.0.1:6443" {
		t.Errorf("expected cluster.endpoint override in values.yaml, got %v", endpoint)
	}
}

func TestInitConfig_DevPlatformGoesToWorkstationState(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local-dev", "--platform", "docker"}, env)
	if err != nil {
		t.Fatalf("init local-dev --platform docker: %v\nstderr: %s", err, stderr)
	}

	statePath := filepath.Join(dir, ".windsor", "contexts", "local-dev", "workstation.yaml")
	if !hasFile(statePath) {
		t.Fatalf("expected workstation.yaml for local-dev context at %s", statePath)
	}
	state := readYAMLFile(t, statePath)
	if platform, ok := getPathValue(state, "platform"); !ok || platform != "docker" {
		t.Errorf("expected platform=docker in workstation.yaml for dev context, got %v", platform)
	}

	valuesPath := filepath.Join(dir, "contexts", "local-dev", "values.yaml")
	if hasFile(valuesPath) {
		values := readYAMLFile(t, valuesPath)
		if platform, ok := getPathValue(values, "platform"); ok {
			t.Errorf("expected platform not to be in values.yaml for dev context, got %v", platform)
		}
	}
}

func TestInitConfig_NonDevPlatformStaysInValues(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "prod", "--platform", "aws"}, env)
	if err != nil {
		t.Fatalf("init prod --platform aws: %v\nstderr: %s", err, stderr)
	}

	valuesPath := filepath.Join(dir, "contexts", "prod", "values.yaml")
	if !hasFile(valuesPath) {
		t.Fatalf("expected values.yaml for prod context at %s", valuesPath)
	}
	values := readYAMLFile(t, valuesPath)
	if platform, ok := getPathValue(values, "platform"); !ok || platform != "aws" {
		t.Errorf("expected platform=aws in values.yaml for non-dev context, got %v", platform)
	}

	statePath := filepath.Join(dir, ".windsor", "contexts", "prod", "workstation.yaml")
	if hasFile(statePath) {
		state := readYAMLFile(t, statePath)
		if platform, ok := getPathValue(state, "platform"); ok {
			t.Errorf("expected platform not to be in workstation.yaml for non-dev context, got %v", platform)
		}
	}
}

func TestInitConfig_RepeatedInitIsIdempotentForExplicitValues(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local", "--set", "custom.value=one"}, env)
	if err != nil {
		t.Fatalf("first init run: %v\nstderr: %s", err, stderr)
	}
	valuesPath := filepath.Join(dir, "contexts", "local", "values.yaml")
	first := readYAMLFile(t, valuesPath)

	_, stderr, err = helpers.RunCLI(dir, []string{"init", "local", "--set", "custom.value=one"}, env)
	if err != nil {
		t.Fatalf("second init run: %v\nstderr: %s", err, stderr)
	}
	second := readYAMLFile(t, valuesPath)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("expected semantic idempotence for values.yaml; first=%v second=%v", first, second)
	}
}

func TestInitConfig_PreservesUserValuesAcrossInit(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")
	contextDir := filepath.Join(dir, "contexts", "prod")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatalf("expected no error creating context dir, got %v", err)
	}
	initial := "custom_key: keep-me\nprovider: aws\n"
	if err := os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(initial), 0644); err != nil {
		t.Fatalf("expected no error writing initial values.yaml, got %v", err)
	}

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "prod"}, env)
	if err != nil {
		t.Fatalf("init prod with existing values.yaml: %v\nstderr: %s", err, stderr)
	}

	values := readYAMLFile(t, filepath.Join(contextDir, "values.yaml"))
	if customKey, ok := getPathValue(values, "custom_key"); !ok || customKey != "keep-me" {
		t.Errorf("expected custom_key=keep-me to be preserved across init, got %v", customKey)
	}
}

func TestInitConfig_SetContextThenInitUsesSelectedContext(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "prod", "--platform", "aws"}, env)
	if err != nil {
		t.Fatalf("init prod --platform aws: %v\nstderr: %s", err, stderr)
	}
	_, stderr, err = helpers.RunCLI(dir, []string{"set-context", "prod"}, env)
	if err != nil {
		t.Fatalf("set-context prod: %v\nstderr: %s", err, stderr)
	}
	_, stderr, err = helpers.RunCLI(dir, []string{"init"}, env)
	if err != nil {
		t.Fatalf("init without context should use selected context: %v\nstderr: %s", err, stderr)
	}

	contextPath := filepath.Join(dir, ".windsor", "context")
	contextValue, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("expected context file at %s, got %v", contextPath, err)
	}
	if strings.TrimSpace(string(contextValue)) != "prod" {
		t.Errorf("expected selected context to remain prod, got %q", strings.TrimSpace(string(contextValue)))
	}
}

func TestInitConfig_DevContextOwnershipStableAcrossReinit(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local-dev", "--platform", "docker", "--set", "dns.enabled=false"}, env)
	if err != nil {
		t.Fatalf("first init local-dev: %v\nstderr: %s", err, stderr)
	}
	_, stderr, err = helpers.RunCLI(dir, []string{"init", "local-dev", "--platform", "docker", "--set", "dns.enabled=false"}, env)
	if err != nil {
		t.Fatalf("second init local-dev: %v\nstderr: %s", err, stderr)
	}

	valuesPath := filepath.Join(dir, "contexts", "local-dev", "values.yaml")
	values := readYAMLFile(t, valuesPath)
	if enabled, ok := getPathValue(values, "dns", "enabled"); !ok || enabled != false {
		t.Errorf("expected dns.enabled=false in values.yaml, got %v", enabled)
	}
	if platform, ok := getPathValue(values, "platform"); ok {
		t.Errorf("expected platform omitted from values.yaml in dev context, got %v", platform)
	}

	workstationPath := filepath.Join(dir, ".windsor", "contexts", "local-dev", "workstation.yaml")
	workstation := readYAMLFile(t, workstationPath)
	if platform, ok := getPathValue(workstation, "platform"); !ok || platform != "docker" {
		t.Errorf("expected platform=docker in workstation.yaml, got %v", platform)
	}
}

func TestInitConfig_InvalidValuesDoNotBlockShowCommand(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "default"}, env)
	if err != nil {
		t.Fatalf("init default: %v\nstderr: %s", err, stderr)
	}

	valuesPath := filepath.Join(dir, "contexts", "default", "values.yaml")
	if err := os.WriteFile(valuesPath, []byte("invalid_key: true\n"), 0644); err != nil {
		t.Fatalf("expected to write invalid values.yaml, got %v", err)
	}

	envWithContext := append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, envWithContext)
	if err != nil {
		t.Fatalf("show blueprint should still succeed with schema-invalid values: %v\nstderr: %s", err, stderr)
	}
	if len(stdout) == 0 {
		t.Fatal("expected blueprint output to be non-empty")
	}
}
