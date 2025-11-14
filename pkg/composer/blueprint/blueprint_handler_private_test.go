package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
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
				"substitutions": map[string]any{
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
				return []byte(`substitutions:
  common:
    domain: template.com
  ingress:
    host: template.example.com`), nil
			}
			if name == filepath.Join(configRoot, "values.yaml") {
				return []byte(`substitutions:
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

	t.Run("SuccessWithKustomizationSubstitutions", func(t *testing.T) {
		handler := setup(t)

		tmpDir := t.TempDir()
		configRoot := filepath.Join(tmpDir, "config")
		projectRoot := filepath.Join(tmpDir, "project")

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
				return []string{"/host:/container"}
			}
			return []string{}
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}

		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, ".windsor", ".build-id") {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		handler.blueprint = blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "ingress",
					Path: "ingress",
				},
				{
					Name: "monitoring",
					Path: "monitoring",
				},
			},
		}

		handler.featureSubstitutions = map[string]map[string]string{
			"ingress": {
				"host":     "ingress.example.com",
				"replicas": "3",
			},
			"monitoring": {
				"retention": "30d",
				"enabled":   "true",
			},
		}

		var appliedConfigMaps []string
		var configMapData = make(map[string]map[string]string)
		mockKubernetesManager := handler.kubernetesManager.(*kubernetes.MockKubernetesManager)
		mockKubernetesManager.ApplyConfigMapFunc = func(name, namespace string, data map[string]string) error {
			appliedConfigMaps = append(appliedConfigMaps, name)
			configMapData[name] = data
			return nil
		}

		err := handler.applyValuesConfigMaps()

		if err != nil {
			t.Fatalf("expected applyValuesConfigMaps to succeed, got: %v", err)
		}

		if len(appliedConfigMaps) != 3 {
			t.Errorf("expected 3 ConfigMaps to be applied (common, ingress, monitoring), got %d: %v", len(appliedConfigMaps), appliedConfigMaps)
		}

		ingressFound := false
		monitoringFound := false
		for _, name := range appliedConfigMaps {
			if name == "values-ingress" {
				ingressFound = true
			}
			if name == "values-monitoring" {
				monitoringFound = true
			}
		}

		if !ingressFound {
			t.Error("expected values-ingress ConfigMap to be applied")
		}
		if !monitoringFound {
			t.Error("expected values-monitoring ConfigMap to be applied")
		}

		if data, ok := configMapData["values-ingress"]; ok {
			if data["host"] != "ingress.example.com" {
				t.Errorf("expected ingress host to be 'ingress.example.com', got '%s'", data["host"])
			}
			if data["replicas"] != "3" {
				t.Errorf("expected ingress replicas to be '3', got '%s'", data["replicas"])
			}
		}

		if data, ok := configMapData["values-monitoring"]; ok {
			if data["retention"] != "30d" {
				t.Errorf("expected monitoring retention to be '30d', got '%s'", data["retention"])
			}
			if data["enabled"] != "true" {
				t.Errorf("expected monitoring enabled to be 'true', got '%s'", data["enabled"])
			}
		}
	})

	t.Run("EvaluatesKustomizationSubstitutionExpressions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
kustomize:
  - name: ingress
    path: ingress
`)

		featureWithSubstitutions := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: ingress-config
when: ingress.enabled == true
kustomize:
  - name: ingress
    path: ingress
    substitutions:
      host: "${dns.domain}"
      replicas: "${cluster.workers.count}"
      url: "https://${dns.domain}"
      literal: "my-literal-value"
`)

		templateData := map[string][]byte{
			"blueprint":                 baseBlueprint,
			"features/ingress-cfg.yaml": featureWithSubstitutions,
		}

		config := map[string]any{
			"ingress": map[string]any{
				"enabled": true,
			},
			"dns": map[string]any{
				"domain": "example.com",
			},
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(handler.featureSubstitutions) != 1 {
			t.Fatalf("Expected 1 kustomization with substitutions, got %d", len(handler.featureSubstitutions))
		}

		ingressSubs, ok := handler.featureSubstitutions["ingress"]
		if !ok {
			t.Fatal("Expected ingress substitutions to be present")
		}

		if ingressSubs["host"] != "example.com" {
			t.Errorf("Expected host to be 'example.com', got '%s'", ingressSubs["host"])
		}

		if ingressSubs["replicas"] != "3" {
			t.Errorf("Expected replicas to be '3', got '%s'", ingressSubs["replicas"])
		}

		if ingressSubs["url"] != "https://example.com" {
			t.Errorf("Expected url to be 'https://example.com', got '%s'", ingressSubs["url"])
		}

		if ingressSubs["literal"] != "my-literal-value" {
			t.Errorf("Expected literal to be 'my-literal-value', got '%s'", ingressSubs["literal"])
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
}

func TestBaseBlueprintHandler_prepareAndConvertKustomization(t *testing.T) {
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
		// Given a handler with feature substitutions
		handler := setup(t)
		handler.featureSubstitutions = map[string]map[string]string{
			"test-kustomization": {
				"domain": "example.com",
			},
		}

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with ConfigMap references
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should have the component ConfigMap reference
		if len(result.Spec.PostBuild.SubstituteFrom) < 1 {
			t.Fatal("expected at least 1 SubstituteFrom reference")
		}

		componentValuesFound := false
		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-test-kustomization" {
				componentValuesFound = true
				if ref.Optional != false {
					t.Errorf("expected values-test-kustomization ConfigMap to be Optional=false, got %v", ref.Optional)
				}
			}
		}

		if !componentValuesFound {
			t.Error("expected values-test-kustomization ConfigMap reference to be present")
		}
	})

	t.Run("WithComponentValuesConfigMap", func(t *testing.T) {
		// Given a handler with feature substitutions
		handler := setup(t)
		handler.featureSubstitutions = map[string]map[string]string{
			"ingress": {
				"domain": "example.com",
			},
		}

		// And initialize the blueprint
		handler.blueprint = blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/test/repo.git",
			},
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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

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

	t.Run("WithoutFeatureSubstitutions", func(t *testing.T) {
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

		// And a kustomization without PostBuild
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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with only ConfigMap references from feature substitutions
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set")
		}

		// And it should not have any Substitute values since PostBuild is no longer user-facing
		if len(result.Spec.PostBuild.Substitute) != 0 {
			t.Errorf("expected 0 Substitute values, got %d", len(result.Spec.PostBuild.Substitute))
		}

		// And it should not add a component ConfigMap without feature substitutions
		for _, ref := range result.Spec.PostBuild.SubstituteFrom {
			if ref.Kind == "ConfigMap" && ref.Name == "values-test-kustomization" {
				t.Error("did not expect values-test-kustomization ConfigMap reference without feature substitutions")
			}
		}
	})

	t.Run("WithoutValuesConfigMaps", func(t *testing.T) {
		// Given a handler without feature substitutions
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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with only values-common (no component-specific ConfigMap)
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set with values-common")
		}
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Errorf("expected 1 SubstituteFrom reference (values-common), got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Name != "values-common" {
			t.Errorf("expected first SubstituteFrom to be values-common, got %s", result.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given a handler without feature substitutions
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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

		// Then it should have PostBuild with only values-common (no component-specific ConfigMap)
		if result.Spec.PostBuild == nil {
			t.Fatal("expected PostBuild to be set with values-common")
		}
		if len(result.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Errorf("expected 1 SubstituteFrom reference (values-common), got %d", len(result.Spec.PostBuild.SubstituteFrom))
		}
		if result.Spec.PostBuild.SubstituteFrom[0].Name != "values-common" {
			t.Errorf("expected first SubstituteFrom to be values-common, got %s", result.Spec.PostBuild.SubstituteFrom[0].Name)
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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

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
		result := handler.prepareAndConvertKustomization(kustomization, "test-namespace")

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

	// Set up build ID by ensuring project root is writable and creating the build ID file
	testBuildID := "build-1234567890"
	projectRoot, err := mocks.Shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("failed to get project root: %v", err)
	}
	buildIDDir := filepath.Join(projectRoot, ".windsor")
	buildIDPath := filepath.Join(buildIDDir, ".build-id")

	// Ensure the directory exists and create the build ID file
	if err := os.MkdirAll(buildIDDir, 0755); err != nil {
		t.Fatalf("failed to create build ID directory: %v", err)
	}
	if err := os.WriteFile(buildIDPath, []byte(testBuildID), 0644); err != nil {
		t.Fatalf("failed to create build ID file: %v", err)
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

func TestBaseBlueprintHandler_parseFeature(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("ParseValidFeature", func(t *testing.T) {
		handler := setup(t)

		featureYAML := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-observability
  description: Observability stack for AWS
when: provider == "aws"
terraform:
  - path: observability/quickwit
    when: observability.backend == "quickwit"
    values:
      storage_bucket: my-bucket
kustomize:
  - name: grafana
    path: observability/grafana
    when: observability.enabled == true
`)

		feature, err := handler.parseFeature(featureYAML)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if feature.Kind != "Feature" {
			t.Errorf("Expected kind 'Feature', got '%s'", feature.Kind)
		}
		if feature.ApiVersion != "blueprints.windsorcli.dev/v1alpha1" {
			t.Errorf("Expected apiVersion 'blueprints.windsorcli.dev/v1alpha1', got '%s'", feature.ApiVersion)
		}
		if feature.Metadata.Name != "aws-observability" {
			t.Errorf("Expected name 'aws-observability', got '%s'", feature.Metadata.Name)
		}
		if feature.When != `provider == "aws"` {
			t.Errorf("Expected when condition, got '%s'", feature.When)
		}
		if len(feature.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(feature.TerraformComponents))
		}
		if len(feature.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(feature.Kustomizations))
		}
	})

	t.Run("FailsOnInvalidYAML", func(t *testing.T) {
		handler := setup(t)

		invalidYAML := []byte(`this is not valid yaml: [`)

		_, err := handler.parseFeature(invalidYAML)

		if err == nil {
			t.Error("Expected error for invalid YAML, got nil")
		}
		if !strings.Contains(err.Error(), "invalid YAML") {
			t.Errorf("Expected 'invalid YAML' error, got %v", err)
		}
	})

	t.Run("FailsOnWrongKind", func(t *testing.T) {
		handler := setup(t)

		wrongKind := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
`)

		_, err := handler.parseFeature(wrongKind)

		if err == nil {
			t.Error("Expected error for wrong kind, got nil")
		}
		if !strings.Contains(err.Error(), "expected kind 'Feature'") {
			t.Errorf("Expected kind error, got %v", err)
		}
	})

	t.Run("FailsOnMissingApiVersion", func(t *testing.T) {
		handler := setup(t)

		missingVersion := []byte(`kind: Feature
metadata:
  name: test
`)

		_, err := handler.parseFeature(missingVersion)

		if err == nil {
			t.Error("Expected error for missing apiVersion, got nil")
		}
		if !strings.Contains(err.Error(), "apiVersion is required") {
			t.Errorf("Expected apiVersion error, got %v", err)
		}
	})

	t.Run("FailsOnMissingName", func(t *testing.T) {
		handler := setup(t)

		missingName := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  description: test
`)

		_, err := handler.parseFeature(missingName)

		if err == nil {
			t.Error("Expected error for missing name, got nil")
		}
		if !strings.Contains(err.Error(), "metadata.name is required") {
			t.Errorf("Expected name error, got %v", err)
		}
	})

	t.Run("ParseFeatureWithoutWhenCondition", func(t *testing.T) {
		handler := setup(t)

		featureYAML := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base-feature
terraform:
  - path: base/component
`)

		feature, err := handler.parseFeature(featureYAML)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if feature.When != "" {
			t.Errorf("Expected empty when condition, got '%s'", feature.When)
		}
		if len(feature.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(feature.TerraformComponents))
		}
	})
}

func TestBaseBlueprintHandler_loadFeatures(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("LoadMultipleFeatures", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/aws.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
`),
			"features/observability.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability-feature
`),
			"blueprint.jsonnet": []byte(`{}`),
			"schema.yaml":       []byte(`{}`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 2 {
			t.Errorf("Expected 2 features, got %d", len(features))
		}
		names := make(map[string]bool)
		for _, feature := range features {
			names[feature.Metadata.Name] = true
		}
		if !names["aws-feature"] || !names["observability-feature"] {
			t.Errorf("Expected both features to be loaded, got %v", names)
		}
	})

	t.Run("LoadNoFeatures", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"blueprint.jsonnet": []byte(`{}`),
			"schema.yaml":       []byte(`{}`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 0 {
			t.Errorf("Expected 0 features, got %d", len(features))
		}
	})

	t.Run("IgnoresNonFeatureYAMLFiles", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/aws.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
`),
			"schema.yaml":           []byte(`{}`),
			"values.yaml":           []byte(`key: value`),
			"terraform/module.yaml": []byte(`key: value`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 1 {
			t.Errorf("Expected 1 feature, got %d", len(features))
		}
		if features[0].Metadata.Name != "aws-feature" {
			t.Errorf("Expected 'aws-feature', got '%s'", features[0].Metadata.Name)
		}
	})

	t.Run("FailsOnInvalidFeature", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/valid.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: valid-feature
`),
			"features/invalid.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  description: missing name
`),
		}

		_, err := handler.loadFeatures(templateData)

		if err == nil {
			t.Error("Expected error for invalid feature, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse feature features/invalid.yaml") {
			t.Errorf("Expected parse error with path, got %v", err)
		}
		if !strings.Contains(err.Error(), "metadata.name is required") {
			t.Errorf("Expected name requirement error, got %v", err)
		}
	})

	t.Run("LoadFeaturesWithComplexStructures", func(t *testing.T) {
		handler := setup(t)

		templateData := map[string][]byte{
			"features/complex.yaml": []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: complex-feature
  description: Complex feature with multiple components
when: provider == "aws" && observability.enabled == true
terraform:
  - path: observability/quickwit
    when: observability.backend == "quickwit"
    values:
      storage_bucket: my-bucket
      replicas: 3
  - path: observability/grafana
    values:
      domain: grafana.example.com
kustomize:
  - name: monitoring
    path: monitoring/stack
    when: monitoring.enabled == true
  - name: logging
    path: logging/stack
`),
		}

		features, err := handler.loadFeatures(templateData)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(features) != 1 {
			t.Fatalf("Expected 1 feature, got %d", len(features))
		}
		feature := features[0]
		if feature.Metadata.Name != "complex-feature" {
			t.Errorf("Expected 'complex-feature', got '%s'", feature.Metadata.Name)
		}
		if len(feature.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(feature.TerraformComponents))
		}
		if len(feature.Kustomizations) != 2 {
			t.Errorf("Expected 2 kustomizations, got %d", len(feature.Kustomizations))
		}
		if feature.TerraformComponents[0].When != `observability.backend == "quickwit"` {
			t.Errorf("Expected when condition on terraform component, got '%s'", feature.TerraformComponents[0].When)
		}
	})
}

func TestBaseBlueprintHandler_processFeatures(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler
	}

	t.Run("ProcessFeaturesWithMatchingConditions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
terraform:
  - path: observability/quickwit
    values:
      bucket: my-bucket
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/aws.yaml": awsFeature,
		}

		config := map[string]any{
			"provider": "aws",
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}
		if handler.blueprint.TerraformComponents[0].Path != "observability/quickwit" {
			t.Errorf("Expected path 'observability/quickwit', got '%s'", handler.blueprint.TerraformComponents[0].Path)
		}
	})

	t.Run("SkipsFeaturesWithNonMatchingConditions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
terraform:
  - path: observability/quickwit
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/aws.yaml": awsFeature,
		}

		config := map[string]any{
			"provider": "gcp",
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("ProcessComponentLevelConditions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		observabilityFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability
terraform:
  - path: observability/quickwit
    when: observability.backend == "quickwit"
  - path: observability/grafana
    when: observability.backend == "grafana"
`)

		templateData := map[string][]byte{
			"blueprint":                   baseBlueprint,
			"features/observability.yaml": observabilityFeature,
		}

		config := map[string]any{
			"observability": map[string]any{
				"backend": "quickwit",
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}
		if handler.blueprint.TerraformComponents[0].Path != "observability/quickwit" {
			t.Errorf("Expected 'observability/quickwit', got '%s'", handler.blueprint.TerraformComponents[0].Path)
		}
	})

	t.Run("MergesMultipleMatchingFeatures", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
when: provider == "aws"
terraform:
  - path: network/vpc
`)

		observabilityFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: observability
when: observability.enabled == true
terraform:
  - path: observability/quickwit
`)

		templateData := map[string][]byte{
			"blueprint":                   baseBlueprint,
			"features/aws.yaml":           awsFeature,
			"features/observability.yaml": observabilityFeature,
		}

		config := map[string]any{
			"provider": "aws",
			"observability": map[string]any{
				"enabled": true,
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("SortsFeaturesDeterministically", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		featureZ := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: z-feature
terraform:
  - path: z/module
`)

		featureA := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: a-feature
terraform:
  - path: a/module
`)

		templateData := map[string][]byte{
			"blueprint":       baseBlueprint,
			"features/z.yaml": featureZ,
			"features/a.yaml": featureA,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
		if handler.blueprint.TerraformComponents[0].Path != "a/module" {
			t.Errorf("Expected first component 'a/module', got '%s'", handler.blueprint.TerraformComponents[0].Path)
		}
		if handler.blueprint.TerraformComponents[1].Path != "z/module" {
			t.Errorf("Expected second component 'z/module', got '%s'", handler.blueprint.TerraformComponents[1].Path)
		}
	})

	t.Run("ProcessesKustomizations", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		fluxFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: flux
when: gitops.enabled == true
kustomize:
  - name: flux-system
    path: gitops/flux
`)

		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/flux.yaml": fluxFeature,
		}

		config := map[string]any{
			"gitops": map[string]any{
				"enabled": true,
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(handler.blueprint.Kustomizations))
		}
		if handler.blueprint.Kustomizations[0].Name != "flux-system" {
			t.Errorf("Expected 'flux-system', got '%s'", handler.blueprint.Kustomizations[0].Name)
		}
	})

	t.Run("HandlesNoFeatures", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		templateData := map[string][]byte{
			"blueprint": baseBlueprint,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("HandlesNoBlueprint", func(t *testing.T) {
		handler := setup(t)

		awsFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
terraform:
  - path: network/vpc
`)

		templateData := map[string][]byte{
			"features/aws.yaml": awsFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}
	})

	t.Run("FailsOnInvalidFeatureCondition", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		badFeature := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: bad-feature
when: invalid syntax ===
terraform:
  - path: test/module
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/bad.yaml": badFeature,
		}

		config := map[string]any{}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Error("Expected error for invalid condition, got nil")
		}
		if !strings.Contains(err.Error(), "failed to evaluate feature condition") {
			t.Errorf("Expected condition evaluation error, got %v", err)
		}
	})

	t.Run("EvaluatesAndMergesInputs", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		featureWithInputs := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-eks
when: provider == "aws"
terraform:
  - path: cluster/aws-eks
    inputs:
      cluster_name: my-cluster
      node_groups:
        default:
          instance_types:
            - ${cluster.workers.instance_type}
          min_size: ${cluster.workers.count}
          max_size: ${cluster.workers.count + 2}
          desired_size: ${cluster.workers.count}
      region: us-east-1
      literal_string: my-literal-value
`)

		templateData := map[string][]byte{
			"blueprint":         baseBlueprint,
			"features/eks.yaml": featureWithInputs,
		}

		config := map[string]any{
			"provider": "aws",
			"cluster": map[string]any{
				"workers": map[string]any{
					"instance_type": "t3.medium",
					"count":         3,
				},
			},
		}

		err := handler.processFeatures(templateData, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(handler.blueprint.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(handler.blueprint.TerraformComponents))
		}

		component := handler.blueprint.TerraformComponents[0]

		if component.Inputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be 'my-cluster', got %v", component.Inputs["cluster_name"])
		}

		nodeGroups, ok := component.Inputs["node_groups"].(map[string]any)
		if !ok {
			t.Fatalf("Expected node_groups to be a map, got %T", component.Inputs["node_groups"])
		}

		defaultGroup, ok := nodeGroups["default"].(map[string]any)
		if !ok {
			t.Fatalf("Expected default group to be a map, got %T", nodeGroups["default"])
		}

		instanceTypes, ok := defaultGroup["instance_types"].([]any)
		if !ok {
			t.Fatalf("Expected instance_types to be an array, got %T", defaultGroup["instance_types"])
		}
		if len(instanceTypes) != 1 || instanceTypes[0] != "t3.medium" {
			t.Errorf("Expected instance_types to be ['t3.medium'], got %v", instanceTypes)
		}

		if defaultGroup["min_size"] != 3 {
			t.Errorf("Expected min_size to be 3, got %v", defaultGroup["min_size"])
		}

		if defaultGroup["max_size"] != 5 {
			t.Errorf("Expected max_size to be 5 (3+2), got %v", defaultGroup["max_size"])
		}

		if defaultGroup["desired_size"] != 3 {
			t.Errorf("Expected desired_size to be 3, got %v", defaultGroup["desired_size"])
		}

		if component.Inputs["region"] != "us-east-1" {
			t.Errorf("Expected region to be literal 'us-east-1', got %v", component.Inputs["region"])
		}

		if component.Inputs["literal_string"] != "my-literal-value" {
			t.Errorf("Expected literal_string to be 'my-literal-value', got %v", component.Inputs["literal_string"])
		}
	})

	t.Run("FailsOnInvalidExpressions", func(t *testing.T) {
		handler := setup(t)

		baseBlueprint := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
`)

		featureWithBadExpression := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test
terraform:
  - path: test/module
    inputs:
      bad_path: ${cluster.workrs.count}
`)

		templateData := map[string][]byte{
			"blueprint":          baseBlueprint,
			"features/test.yaml": featureWithBadExpression,
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		err := handler.processFeatures(templateData, config)

		if err == nil {
			t.Fatal("Expected error for invalid expression, got nil")
		}
		if !strings.Contains(err.Error(), "failed to evaluate inputs") {
			t.Errorf("Expected inputs evaluation error, got %v", err)
		}
	})
}

func TestBaseBlueprintHandler_setRepositoryDefaults(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.shell = mocks.Shell
		return handler
	}

	t.Run("PreservesExistingRepositoryURL", func(t *testing.T) {
		handler := setup(t)
		handler.blueprint.Repository.Url = "https://github.com/existing/repo"

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return false
			}
			return false
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.blueprint.Repository.Url != "https://github.com/existing/repo" {
			t.Errorf("Expected URL to remain unchanged, got %s", handler.blueprint.Repository.Url)
		}
	})

	t.Run("UsesDevelopmentURLWhenDevFlagEnabled", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			return ""
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/path/to/my-project", nil
		}

		handler.shims.FilepathBase = func(path string) string {
			return "my-project"
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "http://git.example.com/git/my-project"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

	t.Run("FallsBackToGitRemoteOrigin", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" && len(args) == 3 && args[0] == "config" && args[2] == "remote.origin.url" {
				return "https://github.com/user/repo.git\n", nil
			}
			return "", fmt.Errorf("command not found")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "https://github.com/user/repo.git"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

	t.Run("PreservesSSHGitRemoteOrigin", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" && len(args) == 3 && args[0] == "config" && args[2] == "remote.origin.url" {
				return "git@github.com:windsorcli/core.git\n", nil
			}
			return "", fmt.Errorf("command not found")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "git@github.com:windsorcli/core.git"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

	t.Run("HandlesGitRemoteOriginError", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("not a git repository")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error even when git fails, got %v", err)
		}
		if handler.blueprint.Repository.Url != "" {
			t.Errorf("Expected URL to remain empty when git fails, got %s", handler.blueprint.Repository.Url)
		}
	})

	t.Run("HandlesEmptyGitRemoteOriginOutput", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", nil
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.blueprint.Repository.Url != "" {
			t.Errorf("Expected URL to remain empty, got %s", handler.blueprint.Repository.Url)
		}
	})

	t.Run("DevModeFallsBackToGitWhenDevelopmentURLFails", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "dev" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" {
				return "https://github.com/fallback/repo.git", nil
			}
			return "", fmt.Errorf("command not found")
		}

		err := handler.setRepositoryDefaults()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedURL := "https://github.com/fallback/repo.git"
		if handler.blueprint.Repository.Url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, handler.blueprint.Repository.Url)
		}
	})

}

func TestBaseBlueprintHandler_normalizeGitURL(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		return handler
	}

	t.Run("PreservesSSHURL", func(t *testing.T) {
		handler := setup(t)

		input := "git@github.com:windsorcli/core.git"
		expected := "git@github.com:windsorcli/core.git"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PreservesHTTPSURL", func(t *testing.T) {
		handler := setup(t)

		input := "https://github.com/windsorcli/core.git"
		expected := "https://github.com/windsorcli/core.git"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PreservesHTTPURL", func(t *testing.T) {
		handler := setup(t)

		input := "http://git.test/git/core"
		expected := "http://git.test/git/core"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PrependsHTTPSToPlainURL", func(t *testing.T) {
		handler := setup(t)

		input := "github.com/windsorcli/core.git"
		expected := "https://github.com/windsorcli/core.git"
		result := handler.normalizeGitURL(input)

		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})
}

func TestBaseBlueprintHandler_getDevelopmentRepositoryURL(t *testing.T) {
	setup := func(t *testing.T) *BaseBlueprintHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		handler.configHandler = mocks.ConfigHandler
		handler.shell = mocks.Shell
		return handler
	}

	t.Run("GeneratesCorrectDevelopmentURL", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "dev.example.com"
			}
			return ""
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/home/user/projects/my-awesome-project", nil
		}

		handler.shims.FilepathBase = func(path string) string {
			return "my-awesome-project"
		}

		url := handler.getDevelopmentRepositoryURL()

		expectedURL := "http://git.dev.example.com/git/my-awesome-project"
		if url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, url)
		}
	})

	t.Run("UsesDefaultDomainWhenNotSet", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" && len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/home/user/projects/my-project", nil
		}

		handler.shims.FilepathBase = func(path string) string {
			return "my-project"
		}

		url := handler.getDevelopmentRepositoryURL()

		expectedURL := "http://git.test/git/my-project"
		if url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, url)
		}
	})

	t.Run("ReturnsEmptyWhenProjectRootFails", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			return ""
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root not found")
		}

		url := handler.getDevelopmentRepositoryURL()

		if url != "" {
			t.Errorf("Expected empty URL when project root fails, got %s", url)
		}
	})

	t.Run("ReturnsEmptyWhenFolderNameEmpty", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			return ""
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/home/user/projects/", nil
		}

		handler.shims.FilepathBase = func(path string) string {
			return ""
		}

		url := handler.getDevelopmentRepositoryURL()

		if url != "" {
			t.Errorf("Expected empty URL when folder name is empty, got %s", url)
		}
	})

	t.Run("HandlesComplexProjectPaths", func(t *testing.T) {
		handler := setup(t)

		mockConfigHandler := handler.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "staging.example.io"
			}
			return ""
		}

		mockShell := handler.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/var/www/projects/nested/deep/project-with-dashes", nil
		}

		handler.shims.FilepathBase = func(path string) string {
			return "project-with-dashes"
		}

		url := handler.getDevelopmentRepositoryURL()

		expectedURL := "http://git.staging.example.io/git/project-with-dashes"
		if url != expectedURL {
			t.Errorf("Expected URL to be %s, got %s", expectedURL, url)
		}
	})
}
