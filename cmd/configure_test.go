//go:build integration
// +build integration

package cmd

// configure_test.go holds integration tests only (build tag: integration).
// TestIntegration_ConfigureNetwork runs the full configure network command via runCmd with a
// project and mocked shell so no real Terraform, Docker, or auth are required.

import (
	"context"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/terraform"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

const windsorYAMLNoWorkstation = `version: v1alpha1
contexts:
  local:
    workstation:
      enabled: false
`

// configureTestEnv holds the context, project, and shell capture from setupConfigureTest.
// Proj is the same instance used when running the command (via context override).
type configureTestEnv struct {
	Ctx     context.Context
	Proj    *project.Project
	Capture *ShellCapture
}

// runInitWithMocks runs init "local" with mocked runtime and composer so no OCI blueprint,
// Terraform, or Docker are used. Use after SetupIntegrationProject(t, windsorYAML).
func runInitWithMocks(t *testing.T, projectRoot string) {
	t.Helper()
	m := shell.NewMockShell()
	m.GetProjectRootFunc = func() (string, error) { return projectRoot, nil }
	m.GetSessionTokenFunc = func() (string, error) { return "test-session-token", nil }
	m.WriteResetTokenFunc = func() (string, error) { return "", nil }
	mockTools := tools.NewMockToolsManager()
	rt := runtime.NewRuntime(&runtime.Runtime{Shell: m, ProjectRoot: projectRoot, ToolsManager: mockTools})
	mockBH := blueprint.NewMockBlueprintHandler()
	mockTR := terraform.NewMockModuleResolver()
	comp := composer.NewComposer(rt, &composer.Composer{BlueprintHandler: mockBH, TerraformResolver: mockTR})
	ctx := context.WithValue(context.Background(), runtimeOverridesKey, rt)
	ctx = context.WithValue(ctx, composerOverridesKey, comp)
	_, _, err := runCmd(t, ctx, []string{"init", "local"})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

// setupConfigureTest creates a project by running init with mocks, then returns an env with a
// context, the project, and a capturing mock shell so tests can assert on shell calls and config.
// No real Terraform, Docker, or auth are required.
func setupConfigureTest(t *testing.T) *configureTestEnv {
	t.Helper()
	projectRoot := SetupIntegrationProject(t, minimalWindsorYAML)
	runInitWithMocks(t, projectRoot)
	capture := NewShellCapture()
	m := NewMockShellWithCapture(capture)
	m.GetProjectRootFunc = func() (string, error) { return projectRoot, nil }
	m.GetSessionTokenFunc = func() (string, error) { return "test-session-token", nil }
	m.WriteResetTokenFunc = func() (string, error) { return "", nil }
	rt := runtime.NewRuntime(&runtime.Runtime{Shell: m, ProjectRoot: projectRoot})
	proj := project.NewProject("local", &project.Project{Runtime: rt})
	return &configureTestEnv{
		Ctx:     context.WithValue(context.Background(), projectOverridesKey, proj),
		Proj:    proj,
		Capture: capture,
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestIntegration_ConfigureNetwork(t *testing.T) {
	t.Run("ConfigureNetworkCommandStructure", func(t *testing.T) {
		if configureNetworkCmd.Use != "network" {
			t.Errorf("Expected Use to be 'network', got %s", configureNetworkCmd.Use)
		}
		if configureNetworkCmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if configureNetworkCmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}
		if !configureNetworkCmd.SilenceUsage {
			t.Error("Expected SilenceUsage to be true")
		}
		dnsFlag := configureNetworkCmd.Flags().Lookup("dns-address")
		if dnsFlag == nil {
			t.Fatal("Expected 'dns-address' flag to exist")
		}
		if dnsFlag.DefValue != "" {
			t.Errorf("Expected 'dns-address' flag default value to be empty, got %s", dnsFlag.DefValue)
		}
		if dnsFlag.Usage == "" {
			t.Error("Expected 'dns-address' flag to have usage description")
		}
		found := false
		for _, subCmd := range configureCmd.Commands() {
			if subCmd.Use == "network" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected 'network' to be a subcommand of configure")
		}
		found = false
		for _, subCmd := range rootCmd.Commands() {
			if subCmd.Use == "configure" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected 'configure' to be a subcommand of root")
		}
	})

	t.Run("ConfigureNetworkFailsWhenNotInTrustedDirectory", func(t *testing.T) {
		SetupIntegrationProject(t, minimalWindsorYAML)
		_, _, err := runCmd(t, context.Background(), []string{"configure", "network"})
		assertFailureAndErrorContains(t, err, "trusted")
	})

	t.Run("ConfigureNetworkSucceedsWhenWorkstationNotEnabled", func(t *testing.T) {
		projectRoot := SetupIntegrationProject(t, windsorYAMLNoWorkstation)
		runInitWithMocks(t, projectRoot)
		m := shell.NewMockShell()
		m.GetProjectRootFunc = func() (string, error) { return projectRoot, nil }
		m.GetSessionTokenFunc = func() (string, error) { return "test-session-token", nil }
		m.WriteResetTokenFunc = func() (string, error) { return "", nil }
		rt := runtime.NewRuntime(&runtime.Runtime{Shell: m, ProjectRoot: projectRoot})
		proj := project.NewProject("local", &project.Project{Runtime: rt})
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		_, stderr, err := runCmd(t, ctx, []string{"configure", "network"})
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if stderr != "" && !strings.Contains(stderr, "skipped") {
			t.Errorf("Expected stderr to contain user message, got %q", stderr)
		}
	})

	t.Run("ConfigureNetworkSucceedsWithDnsAddressFlag", func(t *testing.T) {
		env := setupConfigureTest(t)
		_, stderr, err := runCmd(t, env.Ctx, []string{"configure", "network", "--dns-address=10.5.0.2"})
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if stderr != "" && !strings.Contains(stderr, "network") {
			t.Errorf("Expected stderr to contain user message, got %q", stderr)
		}
		if env.Capture.TotalCalls() > 0 {
			for _, c := range env.Capture.SudoCalls {
				t.Logf("ExecSudo: %s %v", c.Command, c.Args)
			}
		}
	})

	t.Run("ConfigureNetworkSucceedsWithoutDnsAddress", func(t *testing.T) {
		env := setupConfigureTest(t)
		_, stderr, err := runCmd(t, env.Ctx, []string{"configure", "network"})
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if stderr != "" && !strings.Contains(stderr, "network") {
			t.Errorf("Expected stderr to contain user message, got %q", stderr)
		}
		if env.Capture.TotalCalls() > 0 {
			for _, c := range env.Capture.ExecCalls {
				t.Logf("Exec: %s %v", c.Command, c.Args)
			}
			for _, c := range env.Capture.SudoCalls {
				t.Logf("ExecSudo: %s %v", c.Command, c.Args)
			}
			for _, c := range env.Capture.SilentCalls {
				t.Logf("ExecSilent: %s %v", c.Command, c.Args)
			}
			for _, c := range env.Capture.SilentWithTimeoutCalls {
				t.Logf("ExecSilentWithTimeout: %s %v (timeout %v)", c.Command, c.Args, c.Timeout)
			}
		}
	})
}
