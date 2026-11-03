package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type PlanMocks struct {
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	BlueprintHandler *blueprint.MockBlueprintHandler
	TerraformStack   *terraforminfra.MockStack
	ToolsManager     *tools.MockToolsManager
	Runtime          *runtime.Runtime
	TmpDir           string
}

func setupPlanTest(t *testing.T, opts ...*SetupOptions) *PlanMocks {
	t.Helper()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string { return "test-context" }
	mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "terraform.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	mockConfigHandler.LoadConfigFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(hasSetFlags ...bool) error { return nil }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }

	testOpts := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		testOpts = opts[0]
	}
	testOpts.ConfigHandler = mockConfigHandler
	baseMocks := setupMocks(t, testOpts)
	tmpDir := baseMocks.TmpDir
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

	baseMocks.Shell.CheckTrustedDirectoryFunc = func() error { return nil }

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	testBlueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "test"},
	}
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }

	mockTerraformStack := terraforminfra.NewMockStack()
	mockTerraformStack.PlanFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error { return nil }

	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	return &PlanMocks{
		ConfigHandler:    baseMocks.ConfigHandler,
		Shell:            baseMocks.Shell,
		BlueprintHandler: mockBlueprintHandler,
		TerraformStack:   mockTerraformStack,
		ToolsManager:     baseMocks.ToolsManager,
		Runtime:          rt,
		TmpDir:           tmpDir,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestDescribePlanMode(t *testing.T) {
	// projectWithMarker wires a seeded kubeconfig (so the version gate engages) and a mock
	// kubernetes manager returning the given marker into a plan project.
	projectWithMarker := func(t *testing.T, getFn func(string) (kubernetes.VersionMarker, bool, error)) *project.Project {
		t.Helper()
		mocks := setupPlanTest(t)
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".kube"), 0755); err != nil {
			t.Fatalf("seed kubeconfig dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, ".kube", "config"), []byte("apiVersion: v1\nkind: Config\n"), 0644); err != nil {
			t.Fatalf("seed kubeconfig: %v", err)
		}
		mocks.Runtime.ConfigRoot = root
		km := kubernetes.NewMockKubernetesManager()
		km.GetVersionMarkerFunc = getFn
		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler
		prov := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: km,
		})
		return project.NewProject("", &project.Project{Runtime: mocks.Runtime, Composer: comp, Provisioner: prov})
	}

	// The bare test blueprint has no sources, so its derived source set is empty; a marker with a
	// non-empty applied set is therefore a version mismatch.
	blueprint := &blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "test"}}

	run := func(proj *project.Project) string {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&buf)
		describePlanMode(cmd, proj, blueprint)
		return buf.String()
	}

	t.Run("AnnouncesPendingTransitionWhenVersionDiffers", func(t *testing.T) {
		proj := projectWithMarker(t, func(string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{
				Phase:          kubernetes.VersionMarkerPhaseIdle,
				AppliedSources: map[string]kubernetes.SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}},
			}, true, nil
		})
		out := run(proj)
		if !strings.Contains(out, "targets a different version") || !strings.Contains(out, "upgrade") {
			t.Errorf("Expected a version-transition notice, got: %q", out)
		}
	})

	t.Run("AnnouncesInFlightUpgrade", func(t *testing.T) {
		proj := projectWithMarker(t, func(string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{Phase: kubernetes.VersionMarkerPhaseUpgrading}, true, nil
		})
		out := run(proj)
		if !strings.Contains(out, "in progress") || !strings.Contains(out, "upgrade") {
			t.Errorf("Expected an in-flight upgrade notice, got: %q", out)
		}
	})

	t.Run("SilentWhenVersionMatches", func(t *testing.T) {
		proj := projectWithMarker(t, func(string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{Phase: kubernetes.VersionMarkerPhaseIdle}, true, nil
		})
		if out := run(proj); out != "" {
			t.Errorf("Expected no notice when the version matches, got: %q", out)
		}
	})

	t.Run("SilentForLegacyContextWithNoMarker", func(t *testing.T) {
		proj := projectWithMarker(t, func(string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, nil
		})
		if out := run(proj); out != "" {
			t.Errorf("Expected no notice for a legacy context, got: %q", out)
		}
	})

	t.Run("SilentWhenMarkerReadFails", func(t *testing.T) {
		proj := projectWithMarker(t, func(string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, fmt.Errorf("connection refused")
		})
		if out := run(proj); out != "" {
			t.Errorf("Expected best-effort silence when the marker cannot be read, got: %q", out)
		}
	})
}

func TestDescribePendingPrunes(t *testing.T) {
	projectWithPrunable := func(t *testing.T, listFn func(*blueprintv1alpha1.Blueprint, string) ([]string, error)) *project.Project {
		t.Helper()
		mocks := setupPlanTest(t)
		km := kubernetes.NewMockKubernetesManager()
		km.ListPrunableKustomizationsFunc = listFn
		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler
		prov := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: km,
		})
		return project.NewProject("", &project.Project{Runtime: mocks.Runtime, Composer: comp, Provisioner: prov})
	}

	blueprint := &blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "test"}}

	run := func(proj *project.Project) string {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&buf)
		describePendingPrunes(cmd, proj, blueprint)
		return buf.String()
	}

	t.Run("ListsPendingPrunes", func(t *testing.T) {
		proj := projectWithPrunable(t, func(*blueprintv1alpha1.Blueprint, string) ([]string, error) {
			return []string{"old-thing", "stale-app"}, nil
		})
		out := run(proj)
		if !strings.Contains(out, "would be pruned") || !strings.Contains(out, "old-thing") || !strings.Contains(out, "stale-app") {
			t.Errorf("Expected a prune preview listing the orphans, got: %q", out)
		}
	})

	t.Run("SilentWhenNothingPrunable", func(t *testing.T) {
		proj := projectWithPrunable(t, func(*blueprintv1alpha1.Blueprint, string) ([]string, error) {
			return nil, nil
		})
		if out := run(proj); out != "" {
			t.Errorf("Expected no output when nothing is prunable, got: %q", out)
		}
	})

	t.Run("SilentWhenListingFails", func(t *testing.T) {
		proj := projectWithPrunable(t, func(*blueprintv1alpha1.Blueprint, string) ([]string, error) {
			return nil, fmt.Errorf("cluster unreachable")
		})
		if out := run(proj); out != "" {
			t.Errorf("Expected best-effort silence on error, got: %q", out)
		}
	})
}
