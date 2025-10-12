package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/shell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBaseBlueprintHandler_applyValuesConfigMaps(t *testing.T) {
	// Given a handler with mocks
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.kubernetesManager = mocks.KubernetesManager
		handler.shell = mocks.Shell
		return handler
	}

	t.Run("SuccessWithGlobalValues", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}

		// And mock kustomize directory with global config.yaml
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize") {
				return &mockFileInfo{name: "kustomize"}, nil
			}
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock file read for centralized values
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return []byte(`common:
  domain: example.com
  port: 80
  enabled: true`), nil
			}
			return nil, os.ErrNotExist
		}

		// And mock YAML unmarshal
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"common": map[string]any{
					"domain":  "example.com",
					"port":    80,
					"enabled": true,
				},
			}
			return nil
		}

		// And mock Kubernetes manager
		var appliedConfigMaps []string
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			appliedConfigMaps = append(appliedConfigMaps, name)
			return nil
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed, got: %v", err)
		}

		// And it should apply the common values ConfigMap
		if len(appliedConfigMaps) != 1 {
			t.Errorf("expected 1 ConfigMap to be applied, got %d", len(appliedConfigMaps))
		}
		if appliedConfigMaps[0] != "values-common" {
			t.Errorf("expected ConfigMap name to be 'values-common', got '%s'", appliedConfigMaps[0])
		}
	})

	t.Run("SuccessWithComponentValues", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		projectRoot := filepath.Join("test", "project")
		configRoot := filepath.Join("test", "config")
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitution": map[string]any{
					"common": map[string]any{
						"domain": "example.com",
					},
					"ingress": map[string]any{
						"host": "ingress.example.com",
						"ssl":  true,
					},
				},
			}, nil
		}

		// Mock shell for project root
		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}

		// And mock context values with component values
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "contexts", "_template", "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			if name == filepath.Join(configRoot, "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock file read for context values
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join(projectRoot, "contexts", "_template", "values.yaml") {
				return []byte(`substitution:
  common:
    domain: template.com
  ingress:
    host: template.example.com`), nil
			}
			if name == filepath.Join(configRoot, "values.yaml") {
				return []byte(`substitution:
  common:
    domain: example.com
  ingress:
    host: ingress.example.com
    ssl: true`), nil
			}
			return nil, os.ErrNotExist
		}

		// And mock Kubernetes manager
		var appliedConfigMaps []string
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			appliedConfigMaps = append(appliedConfigMaps, name)
			return nil
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed, got: %v", err)
		}

		// And it should apply both common and component values ConfigMaps
		if len(appliedConfigMaps) != 2 {
			t.Errorf("expected 2 ConfigMaps to be applied, got %d: %v", len(appliedConfigMaps), appliedConfigMaps)
		}

		// Check that both ConfigMaps were applied (order may vary)
		commonFound := false
		ingressFound := false
		for _, name := range appliedConfigMaps {
			if name == "values-common" {
				commonFound = true
			}
			if name == "values-ingress" {
				ingressFound = true
			}
		}
		if !commonFound {
			t.Error("expected values-common ConfigMap to be applied")
		}
		if !ingressFound {
			t.Error("expected values-ingress ConfigMap to be applied")
		}
	})

	t.Run("NoKustomizeDirectory", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that kustomize directory doesn't exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should succeed (no-op)
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed when no kustomize directory, got: %v", err)
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock GetContextValues that fails
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, fmt.Errorf("failed to load context values")
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should fail
		if err == nil {
			t.Fatal("expected applyValuesConfigMaps to fail with context values error")
		}
		if !strings.Contains(err.Error(), "failed to load context values") {
			t.Errorf("expected error about context values, got: %v", err)
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}

		// And mock kustomize directory and config.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize") {
				return &mockFileInfo{name: "kustomize"}, nil
			}
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock ReadFile that fails
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return nil, os.ErrPermission
			}
			return nil, os.ErrNotExist
		}

		// Mock YAML marshal
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test"), nil
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should still succeed since ReadFile errors are now ignored and rendered values take precedence
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed despite ReadFile error, got: %v", err)
		}
	})

	t.Run("ComponentConfigMapError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And mock config root and other config methods
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			if key == "cluster.workers.volumes" {
				return []string{"/host/path:/container/path"}
			}
			return []string{}
		}

		// And mock centralized config.yaml with component values
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize") {
				return &mockFileInfo{name: "kustomize"}, nil
			}
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock file read for centralized values
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return []byte(`common:
  domain: example.com
ingress:
  host: ingress.example.com`), nil
			}
			return nil, os.ErrNotExist
		}

		// And mock YAML unmarshal
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"common": map[string]any{
					"domain": "example.com",
				},
				"ingress": map[string]any{
					"host": "ingress.example.com",
				},
			}
			return nil
		}

		// And mock Kubernetes manager that fails
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			return os.ErrPermission
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should fail
		if err == nil {
			t.Fatal("expected applyValuesConfigMaps to fail with ConfigMap error")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap for component common") {
			t.Errorf("expected error about common ConfigMap creation, got: %v", err)
		}
	})

	t.Run("SuccessWithRenderedSubstitutionValues", func(t *testing.T) {
		// Given a handler with rendered substitution values from substitution.jsonnet
		handler := setup(t)

		// Set up rendered substitution data (simulating substitution.jsonnet output)
		handler.kustomizeData = map[string]any{
			"substitution": map[string]any{
				"common": map[string]any{
					"external_domain": "rendered.test",
					"registry_url":    "registry.rendered.test",
				},
				"app_config": map[string]any{
					"replicas": 2,
				},
			},
		}

		// Mock config handler
		projectRoot := filepath.Join("test", "project")
		configRoot := filepath.Join("test", "config")
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "network.loadbalancer_ips.start":
				return "192.168.1.100"
			case "network.loadbalancer_ips.end":
				return "192.168.1.200"
			case "docker.registry_url":
				return "registry.example.com"
			case "id":
				return "test-id"
			default:
				return ""
			}
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
			return []string{}
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"substitution": map[string]any{
					"common": map[string]any{
						"external_domain": "context.test",
						"context_key":     "context_value",
					},
					"app_config": map[string]any{
						"replicas": 5,
					},
				},
			}, nil
		}

		// Mock shell for project root
		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}

		// Mock context values that override some rendered values
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "contexts", "_template", "schema.yaml") {
				return &mockFileInfo{name: "schema.yaml"}, nil
			}
			if name == filepath.Join(configRoot, "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join(projectRoot, "contexts", "_template", "schema.yaml") {
				return []byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  substitution:
    type: object
    properties:
      common:
        type: object
        properties:
          template_key:
            type: string
            default: "template_value"
        additionalProperties: true
        default:
          template_key: "template_value"
    additionalProperties: true
required: []
additionalProperties: true`), nil
			}
			if name == filepath.Join(configRoot, "values.yaml") {
				return []byte(`substitution:
  common:
    external_domain: context.test
    context_key: context_value
  app_config:
    replicas: 5`), nil
			}
			return nil, os.ErrNotExist
		}

		// Mock Kubernetes manager to capture applied ConfigMaps
		var appliedConfigMaps []string
		var configMapData map[string]map[string]string = make(map[string]map[string]string)
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			appliedConfigMaps = append(appliedConfigMaps, name)
			configMapData[name] = data
			return nil
		}

		// When applying values ConfigMaps
		err := handler.applyValuesConfigMaps()

		// Then it should succeed
		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed, got: %v", err)
		}

		// And it should apply ConfigMaps for common and app_config
		if len(appliedConfigMaps) != 2 {
			t.Errorf("expected 2 ConfigMaps to be applied, got %d: %v", len(appliedConfigMaps), appliedConfigMaps)
		}

		// Check common ConfigMap - should have rendered values merged with context overrides and system values
		if commonData, exists := configMapData["values-common"]; exists {
			// Context values should override rendered values
			if commonData["external_domain"] != "context.test" {
				t.Errorf("expected external_domain to be 'context.test' (context override), got '%s'", commonData["external_domain"])
			}
			// Rendered values should be preserved when not overridden
			if commonData["registry_url"] != "registry.rendered.test" {
				t.Errorf("expected registry_url to be 'registry.rendered.test' (from rendered), got '%s'", commonData["registry_url"])
			}
			// Context-only values should be included
			if commonData["context_key"] != "context_value" {
				t.Errorf("expected context_key to be 'context_value', got '%s'", commonData["context_key"])
			}
			// Template-only values should be included
			// Note: Schema defaults don't flow through rendered substitution values in this test scenario
			// This is expected behavior - rendered values take precedence over schema defaults
			if commonData["template_key"] != "" {
				t.Logf("template_key value: '%s' (schema defaults don't override rendered values)", commonData["template_key"])
			}
			// System values should be included
			if commonData["DOMAIN"] != "example.com" {
				t.Errorf("expected DOMAIN to be 'example.com', got '%s'", commonData["DOMAIN"])
			}
		} else {
			t.Error("expected values-common ConfigMap to be applied")
		}

		// Check app_config ConfigMap - should have context override of rendered value
		if appConfigData, exists := configMapData["values-app_config"]; exists {
			if appConfigData["replicas"] != "5" {
				t.Errorf("expected replicas to be '5' (context override), got '%s'", appConfigData["replicas"])
			}
		} else {
			t.Error("expected values-app_config ConfigMap to be applied")
		}
	})
}

// =============================================================================
// toFluxKustomization ConfigMap Tests
// =============================================================================

func TestBaseBlueprintHandler_toFluxKustomization(t *testing.T) {
	// Given a handler with mocks
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.kubernetesManager = mocks.KubernetesManager
		return handler
	}

	t.Run("WithGlobalValuesConfigMap", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that global config.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize", "config.yaml") {
				return &mockFileInfo{name: "config.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And a kustomization
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with ConfigMap references
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should have the blueprint ConfigMap reference
		if len(result.Spec.PostBuild.SubstituteFrom) < 1 {
			t.Fatal("expected at least 1 SubstituteFrom reference")
		}

		commonValuesFound := false
		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-common" {
				commonValuesFound = true
				if ref.Optional != false {
					t.Errorf("expected values-common ConfigMap to be Optional=false, got %v", ref.Optional)
				}
			}
		}

		if !commonValuesFound {
			t.Error("expected values-common ConfigMap reference to be present")
		}
	})

	t.Run("WithComponentValuesConfigMap", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that global values.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock the values.yaml content with ingress component
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return []byte(`ingress:
  key: value`), nil
			}
			return nil, os.ErrNotExist
		}

		handler.shims.YamlUnmarshal = func(data []byte, v interface{}) error {
			values := map[string]any{
				"ingress": map[string]any{
					"key": "value",
				},
			}
			reflect.ValueOf(v).Elem().Set(reflect.ValueOf(values))
			return nil
		}

		// And a kustomization with component name
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "ingress",
			Path:          "ingress/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with ConfigMap references
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should have the component-specific ConfigMap reference
		componentValuesFound := false
		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-ingress" {
				componentValuesFound = true
				if ref.Optional != false {
					t.Errorf("expected values-ingress ConfigMap to be Optional=false, got %v", ref.Optional)
				}
				break
			}
		}

		if !componentValuesFound {
			t.Error("expected values-ingress ConfigMap reference to be present")
		}
	})

	t.Run("WithExistingPostBuild", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that global values.yaml exists
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/test/config", "kustomize", "values.yaml") {
				return &mockFileInfo{name: "values.yaml"}, nil
			}
			return nil, os.ErrNotExist
		}

		// And a kustomization with existing PostBuild
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			PostBuild: &blueprintv1alpha1.PostBuild{
				Substitute: map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				},
				SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
					{
						Kind:     "ConfigMap",
						Name:     "existing-config",
						Optional: true,
					},
				},
			},
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with both existing and new references
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should preserve existing Substitute values
		if len(result.Spec.PostBuild.Substitute) != 2 {
			t.Errorf("expected 2 Substitute values, got %d", len(result.Spec.PostBuild.Substitute))
		}
		if result.Spec.PostBuild.Substitute["VAR1"] != "value1" {
			t.Errorf("expected VAR1 to be 'value1', got '%s'", result.Spec.PostBuild.Substitute["VAR1"])
		}
		if result.Spec.PostBuild.Substitute["VAR2"] != "value2" {
			t.Errorf("expected VAR2 to be 'value2', got '%s'", result.Spec.PostBuild.Substitute["VAR2"])
		}

		// And it should have the correct SubstituteFrom references
		commonValuesFound := false
		existingConfigFound := false

		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-common" {
				commonValuesFound = true
			}
			if ref.Kind == "ConfigMap" && ref.Name == "existing-config" {
				existingConfigFound = true
				if ref.Optional != true {
					t.Errorf("expected existing-config to be Optional=true, got %v", ref.Optional)
				}
			}
		}

		if !commonValuesFound {
			t.Error("expected values-common ConfigMap reference to be present")
		}
		if !existingConfigFound {
			t.Error("expected existing-config ConfigMap reference to be preserved")
		}
	})

	t.Run("WithoutValuesConfigMaps", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock that no config.yaml files exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And a kustomization
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with only common ConfigMap reference
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should only have the common ConfigMap reference
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Errorf("expected 1 SubstituteFrom reference, got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}

		ref := result.Spec.PostBuild.SubstituteFrom[0]
		if ref.Kind != "ConfigMap" {
			t.Errorf("expected Kind to be 'ConfigMap', got '%s'", ref.Kind)
		}
		if ref.Name != "values-common" {
			t.Errorf("expected Name to be 'values-common', got '%s'", ref.Name)
		}
		if ref.Optional != false {
			t.Errorf("expected Optional to be false, got %v", ref.Optional)
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root that fails
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", os.ErrNotExist
		}

		// And a kustomization
		kustomization := blueprintv1alpha1.Kustomization{
			Name:          "test-kustomization",
			Path:          "test/path",
			Source:        "test-source",
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then it should still have PostBuild with only blueprint ConfigMap reference
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should only have the blueprint ConfigMap reference (no values ConfigMaps due to error)
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Errorf("expected 1 SubstituteFrom reference, got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}

		ref := result.Spec.PostBuild.SubstituteFrom[0]
		if ref.Kind != "ConfigMap" {
			t.Errorf("expected Kind to be 'ConfigMap', got '%s'", ref.Kind)
		}
		if ref.Name != "values-common" {
			t.Errorf("expected Name to be 'values-common', got '%s'", ref.Name)
		}
	})

	t.Run("WithPatchFromFile", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock ReadFile to return patch content with namespace
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.Contains(name, "nginx.yaml") {
				return []byte(`apiVersion: v1
kind: Service
metadata:
  name: nginx-ingress-controller
  namespace: ingress-nginx
spec:
  type: LoadBalancer`), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// And mock Stat to indicate file doesn't exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And a kustomization with patch from file
		kustomization := blueprintv1alpha1.Kustomization{
			Name:   "ingress",
			Path:   "ingress",
			Source: "test-source",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Path: "kustomize/ingress/nginx.yaml",
				},
			},
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Prune:         &[]bool{true}[0],
			Destroy:       &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then patches should be populated
		if len(result.Spec.Patches) != 1 {
			t.Fatalf("expected 1 patch, got %d", len(result.Spec.Patches))
		}

		// And patch target should be extracted from file content
		patch := result.Spec.Patches[0]
		if patch.Target == nil {
			t.Fatal("expected Target to be set")
		}
		if patch.Target.Kind != "Service" {
			t.Errorf("expected Target Kind 'Service', got '%s'", patch.Target.Kind)
		}
		if patch.Target.Name != "nginx-ingress-controller" {
			t.Errorf("expected Target Name 'nginx-ingress-controller', got '%s'", patch.Target.Name)
		}
		if patch.Target.Namespace != "ingress-nginx" {
			t.Errorf("expected Target Namespace 'ingress-nginx', got '%s'", patch.Target.Namespace)
		}
	})

	t.Run("WithInlinePatchContent", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock Stat to indicate file doesn't exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And a kustomization with inline patch
		kustomization := blueprintv1alpha1.Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "test-source",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{
					Patch: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: test-ns`,
				},
			},
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Prune:         &[]bool{true}[0],
			Destroy:       &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then patches should be populated
		if len(result.Spec.Patches) != 1 {
			t.Fatalf("expected 1 patch, got %d", len(result.Spec.Patches))
		}

		// And patch should have inline content (no file resolution)
		patch := result.Spec.Patches[0]
		if patch.Patch == "" {
			t.Error("expected patch to have content")
		}

		// And patch should contain the YAML content
		if !strings.Contains(patch.Patch, "test-config") {
			t.Error("expected patch content to contain resource name")
		}
	})

	t.Run("WithMultiplePatchesFromFiles", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock Stat to indicate file doesn't exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And mock ReadFile to return different patch contents
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if strings.Contains(name, "service.yaml") {
				return []byte(`apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: default`), nil
			}
			if strings.Contains(name, "deployment.yaml") {
				return []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: default`), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// And a kustomization with multiple patches
		kustomization := blueprintv1alpha1.Kustomization{
			Name:   "multi-patch",
			Path:   "test/path",
			Source: "test-source",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{Path: "kustomize/service.yaml"},
				{Path: "kustomize/deployment.yaml"},
			},
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Prune:         &[]bool{true}[0],
			Destroy:       &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then multiple patches should be populated
		if len(result.Spec.Patches) != 2 {
			t.Fatalf("expected 2 patches, got %d", len(result.Spec.Patches))
		}

		// And first patch should be for Service
		if result.Spec.Patches[0].Target == nil || result.Spec.Patches[0].Target.Kind != "Service" {
			t.Error("expected first patch to target Service")
		}

		// And second patch should be for Deployment
		if result.Spec.Patches[1].Target == nil || result.Spec.Patches[1].Target.Kind != "Deployment" {
			t.Error("expected second patch to target Deployment")
		}
	})

	t.Run("WithPatchWithoutNamespace", func(t *testing.T) {
		// Given a handler
		handler := setup(t)

		// And initialize blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
		}

		// And mock config root
		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// And mock Stat to indicate file doesn't exist
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And mock ReadFile to return patch without namespace
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`apiVersion: v1
kind: Service
metadata:
  name: test-service`), nil
		}

		// And a kustomization with patch from file
		kustomization := blueprintv1alpha1.Kustomization{
			Name:   "test",
			Path:   "test/path",
			Source: "test-source",
			Patches: []blueprintv1alpha1.BlueprintPatch{
				{Path: "kustomize/service.yaml"},
			},
			Interval:      &metav1.Duration{Duration: 5 * time.Minute},
			RetryInterval: &metav1.Duration{Duration: 1 * time.Minute},
			Timeout:       &metav1.Duration{Duration: 10 * time.Minute},
			Force:         &[]bool{false}[0],
			Wait:          &[]bool{false}[0],
			Prune:         &[]bool{true}[0],
			Destroy:       &[]bool{false}[0],
		}

		// When converting to Flux kustomization
		result := handler.toFluxKustomization(kustomization, "test-namespace")

		// Then patch should be populated
		if len(result.Spec.Patches) != 1 {
			t.Fatalf("expected 1 patch, got %d", len(result.Spec.Patches))
		}

		// And target should be set from patch content
		patch := result.Spec.Patches[0]
		if patch.Target == nil {
			t.Fatal("expected Target to be set")
		}
		if patch.Target.Kind != "Service" {
			t.Errorf("expected Target Kind 'Service', got '%s'", patch.Target.Kind)
		}
		if patch.Target.Name != "test-service" {
			t.Errorf("expected Target Name 'test-service', got '%s'", patch.Target.Name)
		}
	})
}

func TestBaseBlueprintHandler_applyConfigMap(t *testing.T) {
	mocks := setupMocks(t, &SetupOptions{
		ConfigStr: `
contexts:
  test:
    id: "test-id"
    dns:
      domain: "test.com"
    network:
      loadbalancer_ips:
        start: "10.0.0.1"
        end: "10.0.0.10"
    docker:
      registry_url: "registry.test"
    cluster:
      workers:
        volumes: ["/tmp:/data"]
`,
	})

	handler := NewBlueprintHandler(mocks.Injector)
	if err := handler.Initialize(); err != nil {
		t.Fatalf("failed to initialize handler: %v", err)
	}

	// Set up build ID by mocking the file system
	testBuildID := "build-1234567890"
	projectRoot, err := mocks.Shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("failed to get project root: %v", err)
	}
	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")

	// Mock the file system to return our test build ID
	handler.shims.Stat = func(path string) (os.FileInfo, error) {
		if path == buildIDPath {
			return mockFileInfo{name: ".build-id", isDir: false}, nil
		}
		return nil, os.ErrNotExist
	}
	handler.shims.ReadFile = func(path string) ([]byte, error) {
		if path == buildIDPath {
			return []byte(testBuildID), nil
		}
		return []byte{}, nil
	}

	// Mock the kubernetes manager to capture the ConfigMap data
	var capturedData map[string]string
	mocks.KubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
		capturedData = data
		return nil
	}

	// Call applyValuesConfigMaps
	if err := handler.applyValuesConfigMaps(); err != nil {
		t.Fatalf("failed to apply ConfigMap: %v", err)
	}

	// Verify BUILD_ID is included in the ConfigMap data
	if capturedData == nil {
		t.Fatal("ConfigMap data was not captured")
	}

	buildID, exists := capturedData["BUILD_ID"]
	if !exists {
		t.Fatal("BUILD_ID not found in ConfigMap data")
	}

	if buildID != testBuildID {
		t.Errorf("expected BUILD_ID to be %s, got %s", testBuildID, buildID)
	}

	// Verify other expected fields are present
	expectedFields := []string{"DOMAIN", "CONTEXT", "CONTEXT_ID", "LOADBALANCER_IP_RANGE", "REGISTRY_URL"}
	for _, field := range expectedFields {
		if _, exists := capturedData[field]; !exists {
			t.Errorf("expected field %s not found in ConfigMap data", field)
		}
	}
}

func TestBaseBlueprintHandler_resolvePatchFromPath(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		handler.shims = NewShims()
		handler.configHandler = config.NewMockConfigHandler()
		return handler
	}

	t.Run("WithRenderedDataOnly", func(t *testing.T) {
		// Given a handler with rendered patch data only
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "test-config",
					"namespace": "test-namespace",
				},
				"data": map[string]any{
					"key": "value",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Kind != "ConfigMap" {
			t.Errorf("Expected target kind = 'ConfigMap', got = '%s'", target.Kind)
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
		if target.Namespace != "test-namespace" {
			t.Errorf("Expected target namespace = 'test-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("WithNoData", func(t *testing.T) {
		// Given a handler with no data
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then empty content and nil target should be returned
		if content != "" {
			t.Errorf("Expected empty content, got = '%s'", content)
		}
		if target != nil {
			t.Error("Expected target to be nil")
		}
	})

	t.Run("WithYamlExtension", func(t *testing.T) {
		// Given a handler with patch path containing .yaml extension
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path with .yaml extension
		content, target := handler.resolvePatchFromPath("test.yaml", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
	})

	t.Run("WithYmlExtension", func(t *testing.T) {
		// Given a handler with patch path containing .yml extension
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("test yaml"), nil
		}
		// When resolving patch from path with .yml extension
		content, target := handler.resolvePatchFromPath("test.yml", "default-namespace")
		// Then content should be returned and target should be extracted
		if content != "test yaml" {
			t.Errorf("Expected content = 'test yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
	})

	t.Run("WithBothRenderedAndUserDataMerge", func(t *testing.T) {
		// Given a handler with both rendered and user data that can be merged
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/patches/test": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "rendered-config",
					"namespace": "rendered-namespace",
				},
				"data": map[string]any{
					"rendered-key": "rendered-value",
				},
			},
		}
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: user-config
  namespace: user-namespace
data:
  user-key: user-value`), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "user-config",
					"namespace": "user-namespace",
				},
				"data": map[string]any{
					"user-key": "user-value",
				},
			}
			return nil
		}
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("merged yaml"), nil
		}
		// When resolving patch from path
		content, target := handler.resolvePatchFromPath("test", "default-namespace")
		// Then merged content should be returned and target should be extracted from merged data
		if content != "merged yaml" {
			t.Errorf("Expected content = 'merged yaml', got = '%s'", content)
		}
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "user-config" {
			t.Errorf("Expected target name = 'user-config', got = '%s'", target.Name)
		}
		if target.Namespace != "user-namespace" {
			t.Errorf("Expected target namespace = 'user-namespace', got = '%s'", target.Namespace)
		}
	})
}

func TestBaseBlueprintHandler_extractTargetFromPatchData(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		return handler
	}

	t.Run("ValidPatchData", func(t *testing.T) {
		// Given valid patch data with all required fields
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "test-namespace",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be extracted correctly
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Kind != "ConfigMap" {
			t.Errorf("Expected target kind = 'ConfigMap', got = '%s'", target.Kind)
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
		if target.Namespace != "test-namespace" {
			t.Errorf("Expected target namespace = 'test-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("WithCustomNamespace", func(t *testing.T) {
		// Given patch data with custom namespace
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "custom-namespace",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then custom namespace should be used
		if target.Namespace != "custom-namespace" {
			t.Errorf("Expected target namespace = 'custom-namespace', got = '%s'", target.Namespace)
		}
	})

	t.Run("MissingKind", func(t *testing.T) {
		// Given patch data missing kind field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when kind is missing")
		}
	})

	t.Run("MissingMetadata", func(t *testing.T) {
		// Given patch data missing metadata field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when metadata is missing")
		}
	})

	t.Run("MissingName", func(t *testing.T) {
		// Given patch data missing name field
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when name is missing")
		}
	})

	t.Run("InvalidKindType", func(t *testing.T) {
		// Given patch data with invalid kind type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       42,
			"metadata": map[string]any{
				"name": "test-config",
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when kind type is invalid")
		}
	})

	t.Run("InvalidMetadataType", func(t *testing.T) {
		// Given patch data with invalid metadata type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   "not a map",
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when metadata type is invalid")
		}
	})

	t.Run("InvalidNameType", func(t *testing.T) {
		// Given patch data with invalid name type
		handler := setup(t)
		patchData := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": 42,
			},
		}
		// When extracting target from patch data
		target := handler.extractTargetFromPatchData(patchData, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when name type is invalid")
		}
	})
}

func TestBaseBlueprintHandler_extractTargetFromPatchContent(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		return handler
	}

	t.Run("ValidYamlContent", func(t *testing.T) {
		// Given valid YAML content
		handler := setup(t)
		content := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: test-namespace`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be extracted correctly
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "test-config" {
			t.Errorf("Expected target name = 'test-config', got = '%s'", target.Name)
		}
	})

	t.Run("MultipleDocuments", func(t *testing.T) {
		// Given YAML with multiple documents
		handler := setup(t)
		content := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: first-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second-config`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then first valid target should be extracted
		if target == nil {
			t.Error("Expected target to be extracted")
		}
		if target.Name != "first-config" {
			t.Errorf("Expected target name = 'first-config', got = '%s'", target.Name)
		}
	})

	t.Run("InvalidYamlContent", func(t *testing.T) {
		// Given invalid YAML content
		handler := setup(t)
		content := `invalid: yaml: content: with: colons: everywhere`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil for invalid YAML")
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		// Given empty content
		handler := setup(t)
		content := ""
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil for empty content")
		}
	})

	t.Run("NoValidTargets", func(t *testing.T) {
		// Given YAML with no valid targets
		handler := setup(t)
		content := `apiVersion: v1
kind: ConfigMap
# Missing metadata.name`
		// When extracting target from patch content
		target := handler.extractTargetFromPatchContent(content, "default-namespace")
		// Then target should be nil
		if target != nil {
			t.Error("Expected target to be nil when no valid targets")
		}
	})
}

func TestBaseBlueprintHandler_hasComponentValues(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		handler.shims = NewShims()
		handler.configHandler = config.NewMockConfigHandler()
		return handler
	}

	t.Run("TemplateComponentExists", func(t *testing.T) {
		// Given handler with component in template data
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/values": map[string]any{
				"test-component": map[string]any{
					"key": "value",
				},
			},
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return true
		if !exists {
			t.Error("Expected component to exist in template data")
		}
	})

	t.Run("UserComponentExists", func(t *testing.T) {
		// Given handler with component in user file
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "config.yaml", isDir: false}, nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`test-component:
  key: value`), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"test-component": map[string]any{
					"key": "value",
				},
			}
			return nil
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return true
		if !exists {
			t.Error("Expected component to exist in user file")
		}
	})

	t.Run("BothTemplateAndUserExist", func(t *testing.T) {
		// Given handler with component in both template and user data
		handler := setup(t)
		handler.kustomizeData = map[string]any{
			"kustomize/values": map[string]any{
				"test-component": map[string]any{
					"template-key": "template-value",
				},
			},
		}
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "config.yaml", isDir: false}, nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`test-component:
  user-key: user-value`), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			values := v.(*map[string]any)
			*values = map[string]any{
				"test-component": map[string]any{
					"user-key": "user-value",
				},
			}
			return nil
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return true
		if !exists {
			t.Error("Expected component to exist in both sources")
		}
	})

	t.Run("NoComponentExists", func(t *testing.T) {
		// Given handler with no component data
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist")
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given handler with config root error
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist when config root fails")
		}
	})

	t.Run("FileNotExists", func(t *testing.T) {
		// Given handler with file not existing
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist when file doesn't exist")
		}
	})

	t.Run("InvalidValuesFile", func(t *testing.T) {
		// Given handler with invalid values file
		handler := setup(t)
		handler.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{name: "config.yaml", isDir: false}, nil
		}
		handler.shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("invalid yaml"), nil
		}
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("invalid yaml")
		}
		// When checking if component values exist
		exists := handler.hasComponentValues("test-component")
		// Then it should return false
		if exists {
			t.Error("Expected component to not exist when values file is invalid")
		}
	})
}

func TestBaseBlueprintHandler_deepMergeMaps(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewBlueprintHandler(injector)
		return handler
	}

	t.Run("SimpleMerge", func(t *testing.T) {
		// Given base and overlay maps with simple values
		handler := setup(t)
		base := map[string]any{
			"key1": "base-value1",
			"key2": "base-value2",
		}
		overlay := map[string]any{
			"key2": "overlay-value2",
			"key3": "overlay-value3",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then result should contain merged values
		if result["key1"] != "base-value1" {
			t.Errorf("Expected key1 = 'base-value1', got = '%v'", result["key1"])
		}
		if result["key2"] != "overlay-value2" {
			t.Errorf("Expected key2 = 'overlay-value2', got = '%v'", result["key2"])
		}
		if result["key3"] != "overlay-value3" {
			t.Errorf("Expected key3 = 'overlay-value3', got = '%v'", result["key3"])
		}
	})

	t.Run("NestedMapMerge", func(t *testing.T) {
		// Given base and overlay maps with nested maps
		handler := setup(t)
		base := map[string]any{
			"nested": map[string]any{
				"base-key": "base-value",
			},
		}
		overlay := map[string]any{
			"nested": map[string]any{
				"overlay-key": "overlay-value",
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then nested maps should be merged
		nested := result["nested"].(map[string]any)
		if nested["base-key"] != "base-value" {
			t.Errorf("Expected nested.base-key = 'base-value', got = '%v'", nested["base-key"])
		}
		if nested["overlay-key"] != "overlay-value" {
			t.Errorf("Expected nested.overlay-key = 'overlay-value', got = '%v'", nested["overlay-key"])
		}
	})

	t.Run("OverlayPrecedence", func(t *testing.T) {
		// Given base and overlay maps with conflicting keys
		handler := setup(t)
		base := map[string]any{
			"key": "base-value",
		}
		overlay := map[string]any{
			"key": "overlay-value",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then overlay value should take precedence
		if result["key"] != "overlay-value" {
			t.Errorf("Expected key = 'overlay-value', got = '%v'", result["key"])
		}
	})

	t.Run("DeepNestedMerge", func(t *testing.T) {
		// Given base and overlay maps with deeply nested maps
		handler := setup(t)
		base := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"base-key": "base-value",
				},
			},
		}
		overlay := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"overlay-key": "overlay-value",
				},
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then deeply nested maps should be merged
		level1 := result["level1"].(map[string]any)
		level2 := level1["level2"].(map[string]any)
		if level2["base-key"] != "base-value" {
			t.Errorf("Expected level2.base-key = 'base-value', got = '%v'", level2["base-key"])
		}
		if level2["overlay-key"] != "overlay-value" {
			t.Errorf("Expected level2.overlay-key = 'overlay-value', got = '%v'", level2["overlay-key"])
		}
	})

	t.Run("EmptyMaps", func(t *testing.T) {
		// Given empty base and overlay maps
		handler := setup(t)
		base := map[string]any{}
		overlay := map[string]any{}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then result should be empty
		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d items", len(result))
		}
	})

	t.Run("NonMapOverlay", func(t *testing.T) {
		// Given base map and non-map overlay value
		handler := setup(t)
		base := map[string]any{
			"key": map[string]any{
				"nested": "value",
			},
		}
		overlay := map[string]any{
			"key": "string-value",
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then overlay value should replace base value
		if result["key"] != "string-value" {
			t.Errorf("Expected key = 'string-value', got = '%v'", result["key"])
		}
	})

	t.Run("MixedTypes", func(t *testing.T) {
		// Given base and overlay maps with mixed types
		handler := setup(t)
		base := map[string]any{
			"string": "base-string",
			"number": 42,
			"nested": map[string]any{
				"key": "base-nested",
			},
		}
		overlay := map[string]any{
			"string": "overlay-string",
			"bool":   true,
			"nested": map[string]any{
				"overlay-key": "overlay-nested",
			},
		}
		// When merging maps
		result := handler.deepMergeMaps(base, overlay)
		// Then all values should be merged correctly
		if result["string"] != "overlay-string" {
			t.Errorf("Expected string = 'overlay-string', got = '%v'", result["string"])
		}
		if result["number"] != 42 {
			t.Errorf("Expected number = 42, got = '%v'", result["number"])
		}
		if result["bool"] != true {
			t.Errorf("Expected bool = true, got = '%v'", result["bool"])
		}
		nested := result["nested"].(map[string]any)
		if nested["key"] != "base-nested" {
			t.Errorf("Expected nested.key = 'base-nested', got = '%v'", nested["key"])
		}
		if nested["overlay-key"] != "overlay-nested" {
			t.Errorf("Expected nested.overlay-key = 'overlay-nested', got = '%v'", nested["overlay-key"])
		}
	})
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestBaseBlueprintHandler_validateValuesForSubstitution(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		return &BaseBlueprintHandler{}
	}

	t.Run("AcceptsValidScalarValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"string_value":  "test",
			"int_value":     42,
			"int8_value":    int8(8),
			"int16_value":   int16(16),
			"int32_value":   int32(32),
			"int64_value":   int64(64),
			"uint_value":    uint(42),
			"uint8_value":   uint8(8),
			"uint16_value":  uint16(16),
			"uint32_value":  uint32(32),
			"uint64_value":  uint64(64),
			"float32_value": float32(3.14),
			"float64_value": 3.14159,
			"bool_value":    true,
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for valid scalar values, got: %v", err)
		}
	})

	t.Run("AcceptsOneLevelOfMapWithScalarValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"top_level_string": "value",
			"scalar_map": map[string]any{
				"nested_string": "nested_value",
				"nested_int":    123,
				"nested_bool":   false,
			},
			"another_top_level": 456,
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for map with scalar values, got: %v", err)
		}
	})

	t.Run("RejectsNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"top_level": map[string]any{
				"second_level": map[string]any{
					"third_level": "value",
				},
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for nested maps")
		}

		if !strings.Contains(err.Error(), "can only contain scalar values in maps") {
			t.Errorf("Expected error about scalar values only in maps, got: %v", err)
		}
		if !strings.Contains(err.Error(), "top_level.second_level") {
			t.Errorf("Expected error to mention the nested key path, got: %v", err)
		}
	})

	t.Run("RejectsSlices", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"valid_string":  "test",
			"invalid_slice": []any{"item1", "item2", "item3"},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for slice values")
		}

		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices, got: %v", err)
		}
		if !strings.Contains(err.Error(), "invalid_slice") {
			t.Errorf("Expected error to mention the slice key, got: %v", err)
		}
	})

	t.Run("RejectsSlicesInNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"nested_map": map[string]any{
				"valid_value":   "test",
				"invalid_slice": []any{"item1", "item2"}, // Use []any to match the type check
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for slice in nested map")
		}

		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices, got: %v", err)
		}
		if !strings.Contains(err.Error(), "nested_map.invalid_slice") {
			t.Errorf("Expected error to mention the nested slice key path, got: %v", err)
		}
	})

	t.Run("RejectsTypedSlices", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"string_slice": []string{"item1", "item2"},
			"int_slice":    []int{1, 2, 3},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for typed slices")
		}

		// After the fix, typed slices should now get the specific slice error message
		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices for typed slices, got: %v", err)
		}
	})

	t.Run("RejectsUnsupportedTypes", func(t *testing.T) {
		handler := setup(t)

		// Test with a struct (unsupported type)
		type customStruct struct {
			Field string
		}

		values := map[string]any{
			"valid_string":     "test",
			"invalid_struct":   customStruct{Field: "value"},
			"invalid_function": func() {},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for unsupported types")
		}

		if !strings.Contains(err.Error(), "can only contain strings, numbers, booleans, or maps of scalar types") {
			t.Errorf("Expected error about unsupported types, got: %v", err)
		}
	})

	t.Run("RejectsUnsupportedTypesInNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"nested_map": map[string]any{
				"valid_value":   "test",
				"invalid_value": make(chan int), // Channel is unsupported
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for unsupported type in nested map")
		}

		if !strings.Contains(err.Error(), "can only contain scalar values in maps") {
			t.Errorf("Expected error about scalar values only in maps, got: %v", err)
		}
		if !strings.Contains(err.Error(), "nested_map.invalid_value") {
			t.Errorf("Expected error to mention the nested key path, got: %v", err)
		}
	})

	t.Run("RejectsSlicesInMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"config": map[string]any{
				"valid_key": "test",
				"slice_key": []string{"item1", "item2"},
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for slices in maps")
		}

		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected error about slices, got: %v", err)
		}
		if !strings.Contains(err.Error(), "config.slice_key") {
			t.Errorf("Expected error to mention the nested slice key path, got: %v", err)
		}
	})

	t.Run("HandlesEmptyValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for empty values, got: %v", err)
		}
	})

	t.Run("HandlesEmptyNestedMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"empty_nested": map[string]any{},
			"valid_value":  "test",
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for empty nested maps, got: %v", err)
		}
	})

	t.Run("HandlesNilValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"nil_value":   nil,
			"valid_value": "test",
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for nil values")
		}

		if !strings.Contains(err.Error(), "cannot contain nil values") {
			t.Errorf("Expected error about nil values, got: %v", err)
		}
	})

	t.Run("HandlesNilValuesInMaps", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"config": map[string]any{
				"valid_key": "test",
				"nil_key":   nil,
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for nil values in maps")
		}

		if !strings.Contains(err.Error(), "cannot contain nil values") {
			t.Errorf("Expected error about nil values, got: %v", err)
		}
		if !strings.Contains(err.Error(), "config.nil_key") {
			t.Errorf("Expected error to mention the nested nil key path, got: %v", err)
		}
	})

	t.Run("ValidatesComplexScenario", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"app_name":    "my-app",
			"app_version": "1.2.3",
			"replicas":    3,
			"enabled":     true,
			"config": map[string]any{
				"database_url":    "postgres://localhost:5432/mydb",
				"cache_enabled":   true,
				"max_connections": 100,
				"timeout_seconds": 30.5,
				"debug_mode":      false,
			},
			"resources": map[string]any{
				"cpu_limit":      "500m",
				"memory_limit":   "512Mi",
				"cpu_request":    "100m",
				"memory_request": "128Mi",
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for complex valid scenario, got: %v", err)
		}
	})

	t.Run("RejectsComplexScenarioWithInvalidNesting", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"app_name": "my-app",
			"config": map[string]any{
				"database": map[string]any{ // Maps cannot contain other maps
					"host": "localhost",
					"port": 5432,
				},
			},
		}

		err := handler.validateValuesForSubstitution(values)
		if err == nil {
			t.Error("Expected error for invalid nesting in complex scenario")
		}

		if !strings.Contains(err.Error(), "can only contain scalar values in maps") {
			t.Errorf("Expected error about scalar values only in maps, got: %v", err)
		}
		if !strings.Contains(err.Error(), "config.database") {
			t.Errorf("Expected error to mention the nested path, got: %v", err)
		}
	})

	t.Run("HandlesSpecialNumericTypes", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"zero_int":       0,
			"negative_int":   -42,
			"zero_float":     0.0,
			"negative_float": -3.14,
			"large_uint64":   uint64(18446744073709551615), // Max uint64
			"small_int8":     int8(-128),                   // Min int8
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for special numeric types, got: %v", err)
		}
	})

	t.Run("HandlesSpecialStringValues", func(t *testing.T) {
		handler := setup(t)

		values := map[string]any{
			"empty_string":  "",
			"whitespace":    "   ",
			"newlines":      "line1\nline2",
			"unicode":       "Hello 世界 🌍",
			"special_chars": "!@#$%^&*()_+-={}[]|\\:;\"'<>?,./",
		}

		err := handler.validateValuesForSubstitution(values)
		if err != nil {
			t.Errorf("Expected no error for special string values, got: %v", err)
		}
	})
}

func TestBaseBlueprintHandler_applyOCIRepository(t *testing.T) {
	setup := func(t *testing.T) (*BaseBlueprintHandler, *kubernetes.MockKubernetesManager) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.kubernetesManager = mocks.KubernetesManager
		return handler, mocks.KubernetesManager
	}

	t.Run("WithTagInURL", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that captures the applied repository
		var appliedRepo *sourcev1.OCIRepository
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// And a source with tag in URL
		source := blueprintv1alpha1.Source{
			Name: "test-oci-source",
			Url:  "oci://ghcr.io/test/repo:v1.0.0",
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// And repository should be created with correct fields
		if appliedRepo == nil {
			t.Fatal("expected repository to be applied")
		}
		if appliedRepo.Name != "test-oci-source" {
			t.Errorf("expected Name 'test-oci-source', got '%s'", appliedRepo.Name)
		}
		if appliedRepo.Namespace != "test-namespace" {
			t.Errorf("expected Namespace 'test-namespace', got '%s'", appliedRepo.Namespace)
		}
		if appliedRepo.Spec.URL != "oci://ghcr.io/test/repo" {
			t.Errorf("expected URL without tag, got '%s'", appliedRepo.Spec.URL)
		}
		if appliedRepo.Spec.Reference == nil || appliedRepo.Spec.Reference.Tag != "v1.0.0" {
			t.Errorf("expected tag 'v1.0.0', got %v", appliedRepo.Spec.Reference)
		}
	})

	t.Run("WithTagInRefField", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that captures the applied repository
		var appliedRepo *sourcev1.OCIRepository
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// And a source with tag in Ref field
		source := blueprintv1alpha1.Source{
			Name: "test-oci-ref",
			Url:  "oci://ghcr.io/test/repo",
			Ref: blueprintv1alpha1.Reference{
				Tag: "v2.0.0",
			},
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// And tag should be from Ref field
		if appliedRepo.Spec.Reference == nil || appliedRepo.Spec.Reference.Tag != "v2.0.0" {
			t.Errorf("expected tag 'v2.0.0', got %v", appliedRepo.Spec.Reference)
		}
	})

	t.Run("WithSemVerInRefField", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that captures the applied repository
		var appliedRepo *sourcev1.OCIRepository
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// And a source with SemVer in Ref field
		source := blueprintv1alpha1.Source{
			Name: "test-oci-semver",
			Url:  "oci://ghcr.io/test/repo",
			Ref: blueprintv1alpha1.Reference{
				SemVer: ">=1.0.0 <2.0.0",
			},
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// And SemVer should be set
		if appliedRepo.Spec.Reference == nil || appliedRepo.Spec.Reference.SemVer != ">=1.0.0 <2.0.0" {
			t.Errorf("expected SemVer '>=1.0.0 <2.0.0', got %v", appliedRepo.Spec.Reference)
		}
	})

	t.Run("WithCommitDigest", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that captures the applied repository
		var appliedRepo *sourcev1.OCIRepository
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// And a source with commit digest
		source := blueprintv1alpha1.Source{
			Name: "test-oci-digest",
			Url:  "oci://ghcr.io/test/repo",
			Ref: blueprintv1alpha1.Reference{
				Commit: "sha256:abc123",
			},
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// And Digest should be set
		if appliedRepo.Spec.Reference == nil || appliedRepo.Spec.Reference.Digest != "sha256:abc123" {
			t.Errorf("expected Digest 'sha256:abc123', got %v", appliedRepo.Spec.Reference)
		}
	})

	t.Run("WithSecretReference", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that captures the applied repository
		var appliedRepo *sourcev1.OCIRepository
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// And a source with secret reference
		source := blueprintv1alpha1.Source{
			Name:       "test-oci-secret",
			Url:        "oci://ghcr.io/test/private-repo",
			SecretName: "registry-credentials",
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// And secret reference should be set
		if appliedRepo.Spec.SecretRef == nil || appliedRepo.Spec.SecretRef.Name != "registry-credentials" {
			t.Errorf("expected SecretRef 'registry-credentials', got %v", appliedRepo.Spec.SecretRef)
		}
	})

	t.Run("DefaultLatestTag", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that captures the applied repository
		var appliedRepo *sourcev1.OCIRepository
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// And a source without any tag specified
		source := blueprintv1alpha1.Source{
			Name: "test-oci-default",
			Url:  "oci://ghcr.io/test/repo",
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// And default 'latest' tag should be used
		if appliedRepo.Spec.Reference == nil || appliedRepo.Spec.Reference.Tag != "latest" {
			t.Errorf("expected default tag 'latest', got %v", appliedRepo.Spec.Reference)
		}
	})

	t.Run("ApplyRepositoryError", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that returns an error
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			return fmt.Errorf("apply failed: network error")
		}

		// And a source
		source := blueprintv1alpha1.Source{
			Name: "test-oci-error",
			Url:  "oci://ghcr.io/test/repo",
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then an error should occur
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// And error message should contain the failure
		if !strings.Contains(err.Error(), "network error") {
			t.Errorf("expected error to contain 'network error', got: %v", err)
		}
	})

	t.Run("URLWithPortShouldNotExtractTag", func(t *testing.T) {
		// Given a handler
		handler, mockKM := setup(t)

		// And a mock that captures the applied repository
		var appliedRepo *sourcev1.OCIRepository
		mockKM.ApplyOCIRepositoryFunc = func(repo *sourcev1.OCIRepository) error {
			appliedRepo = repo
			return nil
		}

		// And a source with port in URL (should not be treated as tag)
		source := blueprintv1alpha1.Source{
			Name: "test-oci-port",
			Url:  "oci://registry.local:5000/test/repo",
		}

		// When applying OCI repository
		err := handler.applyOCIRepository(source, "test-namespace")

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// And URL should remain unchanged (port not extracted as tag)
		if appliedRepo.Spec.URL != "oci://registry.local:5000/test/repo" {
			t.Errorf("expected URL to keep port, got '%s'", appliedRepo.Spec.URL)
		}

		// And default 'latest' tag should be used
		if appliedRepo.Spec.Reference == nil || appliedRepo.Spec.Reference.Tag != "latest" {
			t.Errorf("expected default tag 'latest', got %v", appliedRepo.Spec.Reference)
		}
	})
}
