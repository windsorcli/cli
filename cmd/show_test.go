package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ShowMocks struct {
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	Shims            *Shims
	BlueprintHandler *blueprint.MockBlueprintHandler
	Runtime          *runtime.Runtime
	TmpDir           string
}

func setupShowTest(t *testing.T, opts ...*SetupOptions) *ShowMocks {
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
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
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

	testBlueprint := &blueprintv1alpha1.Blueprint{
		Kind:       "Blueprint",
		ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
		Metadata: blueprintv1alpha1.Metadata{
			Name:        "test-blueprint",
			Description: "Test blueprint for show command",
		},
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{
				Name: "test-component",
				Path: "test/path",
				Inputs: map[string]any{
					"deferred": "${terraform_output(\"cluster\", \"endpoint\")}",
					"literal":  "value",
				},
			},
		},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{
				Name: "test-kustomization",
				Path: "${terraform_output(\"cluster\", \"path\")}",
				Substitutions: map[string]string{
					"DEFERRED_ENDPOINT": "${terraform_output(\"cluster\", \"endpoint\")}",
					"LITERAL":           "value",
				},
			},
		},
	}

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
	mockBlueprintHandler.GetDeferredPathsFunc = func() map[string]bool {
		return map[string]bool{
			"terraform.test-component.inputs.deferred":       true,
			"kustomize.test-kustomization.path":              true,
			"kustomize.test-kustomization.substitutions.DEFERRED_ENDPOINT": true,
		}
	}

	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	return &ShowMocks{
		ConfigHandler:    baseMocks.ConfigHandler,
		Shell:            baseMocks.Shell,
		Shims:            baseMocks.Shims,
		BlueprintHandler: mockBlueprintHandler,
		Runtime:          rt,
		TmpDir:           tmpDir,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestShowBlueprintCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		showBlueprintJSON = false
		showBlueprintRaw = false
		cmd := &cobra.Command{
			Use:          "blueprint",
			Short:        "Display the fully rendered blueprint",
			RunE:         showBlueprintCmd.RunE,
			SilenceUsage: true,
		}

		showBlueprintCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	setupOutput := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer, func()) {
		t.Helper()
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		oldStdout := os.Stdout
		oldStderr := os.Stderr

		rStdout, wStdout, _ := os.Pipe()
		rStderr, wStderr, _ := os.Pipe()

		os.Stdout = wStdout
		os.Stderr = wStderr

		doneStdout := make(chan struct{})
		doneStderr := make(chan struct{})

		go func() {
			io.Copy(stdout, rStdout)
			rStdout.Close()
			close(doneStdout)
		}()
		go func() {
			io.Copy(stderr, rStderr)
			rStderr.Close()
			close(doneStderr)
		}()

		cleanup := func() {
			wStdout.Close()
			wStderr.Close()
			<-doneStdout
			<-doneStderr
			os.Stdout = oldStdout
			os.Stderr = oldStderr
		}
		t.Cleanup(cleanup)

		return stdout, stderr, func() {
			wStdout.Close()
			wStderr.Close()
			<-doneStdout
			<-doneStderr
		}
	}

	t.Run("SuccessWithYAMLOutput", func(t *testing.T) {
		mocks := setupShowTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}

		var bp blueprintv1alpha1.Blueprint
		if err := yaml.Unmarshal([]byte(output), &bp); err != nil {
			t.Errorf("Expected valid YAML output, got error: %v", err)
		}

		if bp.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected blueprint name 'test-blueprint', got %q", bp.Metadata.Name)
		}
		if got := bp.TerraformComponents[0].Inputs["deferred"]; got != "<deferred>" {
			t.Errorf("Expected deferred input to be <deferred>, got %v", got)
		}
		if got := bp.TerraformComponents[0].Inputs["literal"]; got != "value" {
			t.Errorf("Expected literal input to remain value, got %v", got)
		}

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithJSONOutput", func(t *testing.T) {
		mocks := setupShowTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--json"})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}

		var bp blueprintv1alpha1.Blueprint
		if err := json.Unmarshal([]byte(output), &bp); err != nil {
			t.Errorf("Expected valid JSON output, got error: %v", err)
		}

		if bp.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected blueprint name 'test-blueprint', got %q", bp.Metadata.Name)
		}
		if got := bp.TerraformComponents[0].Inputs["deferred"]; got != "<deferred>" {
			t.Errorf("Expected deferred input to be <deferred>, got %v", got)
		}
		if got := bp.TerraformComponents[0].Inputs["literal"]; got != "value" {
			t.Errorf("Expected literal input to remain value, got %v", got)
		}

		if !strings.Contains(output, "\"Kind\"") {
			t.Error("Expected JSON output to contain JSON structure")
		}

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithRawYAMLOutput", func(t *testing.T) {
		mocks := setupShowTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--raw"})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}

		var bp blueprintv1alpha1.Blueprint
		if err := yaml.Unmarshal([]byte(output), &bp); err != nil {
			t.Errorf("Expected valid YAML output, got error: %v", err)
		}
		if got := bp.TerraformComponents[0].Inputs["deferred"]; got != "${terraform_output(\"cluster\", \"endpoint\")}" {
			t.Errorf("Expected deferred input to remain expression in raw mode, got %v", got)
		}
		if strings.Contains(output, "<deferred>") {
			t.Error("Expected raw mode output to not contain <deferred>")
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		mocks := setupShowTest(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("BlueprintGenerationFailure", func(t *testing.T) {
		mocks := setupShowTest(t)

		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return nil
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to generate blueprint") {
			t.Errorf("Expected blueprint generation error, got: %v", err)
		}
	})

	t.Run("CommandInitialization", func(t *testing.T) {
		cmd := showBlueprintCmd

		if cmd.Use != "blueprint" {
			t.Errorf("Expected Use to be 'blueprint', got %q", cmd.Use)
		}
		if cmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if cmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}

		jsonFlag := cmd.Flags().Lookup("json")
		if jsonFlag == nil {
			t.Error("Expected --json flag to be defined")
		}
		if jsonFlag.Usage == "" {
			t.Error("Expected non-empty flag usage")
		}
		rawFlag := cmd.Flags().Lookup("raw")
		if rawFlag == nil {
			t.Error("Expected --raw flag to be defined")
		}
		if rawFlag.Usage == "" {
			t.Error("Expected non-empty raw flag usage")
		}
	})

	t.Run("ShowCommandInitialization", func(t *testing.T) {
		cmd := showCmd

		if cmd.Use != "show" {
			t.Errorf("Expected Use to be 'show', got %q", cmd.Use)
		}
		if cmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if cmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}
	})
}

func TestShowValuesCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		showValuesJSON = false
		cmd := &cobra.Command{
			Use:          "values",
			Short:        "Display the effective context values",
			RunE:         showValuesCmd.RunE,
			SilenceUsage: true,
		}

		showValuesCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	setupOutput := func(t *testing.T) (*bytes.Buffer, func()) {
		t.Helper()
		stdout := new(bytes.Buffer)

		oldStdout := os.Stdout
		rStdout, wStdout, _ := os.Pipe()
		os.Stdout = wStdout

		done := make(chan struct{})
		go func() {
			io.Copy(stdout, rStdout)
			rStdout.Close()
			close(done)
		}()

		t.Cleanup(func() {
			wStdout.Close()
			<-done
			os.Stdout = oldStdout
		})

		return stdout, func() {
			wStdout.Close()
			<-done
		}
	}

	t.Run("SuccessWithYAMLAndDescriptions", func(t *testing.T) {
		mocks := setupShowTest(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "docker", "dev": true}, nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetSchemaFunc = func() map[string]any {
			return map[string]any{
				"$schema": "https://windsorcli.dev/schema/2026-02/schema",
				"type":    "object",
				"properties": map[string]any{
					"provider": map[string]any{
						"type":        "string",
						"description": "Cloud or platform provider.",
					},
				},
			}
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		stdout, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}
		if !strings.Contains(output, "# Cloud or platform provider.") {
			t.Errorf("Expected description comment in output, got:\n%s", output)
		}
		if !strings.Contains(output, "provider: docker") {
			t.Errorf("Expected provider value in output, got:\n%s", output)
		}
	})

	t.Run("SchemaOnlyFieldsRenderedCommentedOut", func(t *testing.T) {
		mocks := setupShowTest(t)
		// values has gateway.enabled but not gateway.driver (no default in schema)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"gateway": map[string]any{"enabled": true},
			}, nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetSchemaFunc = func() map[string]any {
			return map[string]any{
				"$schema": "https://windsorcli.dev/schema/2026-02/schema",
				"type":    "object",
				"properties": map[string]any{
					"gateway": map[string]any{
						"type":        "object",
						"description": "Gateway configuration.",
						"properties": map[string]any{
							"enabled": map[string]any{
								"type":        "boolean",
								"description": "Enable gateway.",
							},
							"driver": map[string]any{
								"type":        "string",
								"description": "Gateway driver.",
							},
						},
					},
				},
			}
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		stdout, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if !strings.Contains(output, "enabled: true") {
			t.Errorf("Expected enabled value in output, got:\n%s", output)
		}
		if !strings.Contains(output, "# Gateway driver.") {
			t.Errorf("Expected driver description comment in output, got:\n%s", output)
		}
		if !strings.Contains(output, "# driver:") {
			t.Errorf("Expected commented-out driver field in output, got:\n%s", output)
		}
	})

	t.Run("SuccessWithYAMLNoSchema", func(t *testing.T) {
		mocks := setupShowTest(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "docker"}, nil
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		stdout, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}
		if !strings.Contains(output, "provider: docker") {
			t.Errorf("Expected provider value in output, got:\n%s", output)
		}
		if strings.Contains(output, "#") {
			t.Errorf("Expected no comments when schema is absent, got:\n%s", output)
		}
	})

	t.Run("SuccessWithJSONOutput", func(t *testing.T) {
		mocks := setupShowTest(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "aws"}, nil
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		stdout, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--json"})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}

		var values map[string]any
		if err := json.Unmarshal([]byte(output), &values); err != nil {
			t.Errorf("Expected valid JSON output, got error: %v", err)
		}
		if values["provider"] != "aws" {
			t.Errorf("Expected provider 'aws', got %v", values["provider"])
		}
		if strings.Contains(output, "#") {
			t.Errorf("Expected no comments in JSON output, got:\n%s", output)
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		mocks := setupShowTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("GetContextValuesError", func(t *testing.T) {
		mocks := setupShowTest(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return nil, fmt.Errorf("schema load failure")
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to get context values") {
			t.Errorf("Expected context values error, got: %v", err)
		}
	})

	t.Run("CommandInitialization", func(t *testing.T) {
		cmd := showValuesCmd

		if cmd.Use != "values" {
			t.Errorf("Expected Use to be 'values', got %q", cmd.Use)
		}
		if cmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if cmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}

		jsonFlag := cmd.Flags().Lookup("json")
		if jsonFlag == nil {
			t.Error("Expected --json flag to be defined")
		}
		if jsonFlag.Usage == "" {
			t.Error("Expected non-empty flag usage")
		}
	})
}

func TestShowKustomizationCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		showKustomizationJSON = false
		showKustomizationRaw = false
		cmd := &cobra.Command{
			Use:          "kustomization",
			Short:        "Display the Flux Kustomization resource for a component",
			RunE:         showKustomizationCmd.RunE,
			SilenceUsage: true,
		}

		showKustomizationCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	setupOutput := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer, func()) {
		t.Helper()
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		oldStdout := os.Stdout
		oldStderr := os.Stderr

		rStdout, wStdout, _ := os.Pipe()
		rStderr, wStderr, _ := os.Pipe()

		os.Stdout = wStdout
		os.Stderr = wStderr

		doneStdout := make(chan struct{})
		doneStderr := make(chan struct{})

		go func() {
			io.Copy(stdout, rStdout)
			rStdout.Close()
			close(doneStdout)
		}()
		go func() {
			io.Copy(stderr, rStderr)
			rStderr.Close()
			close(doneStderr)
		}()

		cleanup := func() {
			wStdout.Close()
			wStderr.Close()
			<-doneStdout
			<-doneStderr
			os.Stdout = oldStdout
			os.Stderr = oldStderr
		}
		t.Cleanup(cleanup)

		return stdout, stderr, func() {
			wStdout.Close()
			wStderr.Close()
			<-doneStdout
			<-doneStderr
		}
	}

	t.Run("SuccessWithYAMLOutput", func(t *testing.T) {
		mocks := setupShowTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization"})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}

		var kustomization kustomizev1.Kustomization
		if err := yaml.Unmarshal([]byte(output), &kustomization); err != nil {
			t.Errorf("Expected valid YAML output, got error: %v", err)
		}

		if kustomization.Name != "test-kustomization" {
			t.Errorf("Expected kustomization name 'test-kustomization', got %q", kustomization.Name)
		}

		if kustomization.Kind != "Kustomization" {
			t.Errorf("Expected Kind 'Kustomization', got %q", kustomization.Kind)
		}

		if kustomization.APIVersion != "kustomize.toolkit.fluxcd.io/v1" {
			t.Errorf("Expected APIVersion 'kustomize.toolkit.fluxcd.io/v1', got %q", kustomization.APIVersion)
		}
		if got := kustomization.Spec.Path; got != "<deferred>" {
			t.Errorf("Expected deferred kustomization path to be <deferred>, got %q", got)
		}

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithJSONOutput", func(t *testing.T) {
		mocks := setupShowTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization", "--json"})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}

		var kustomization kustomizev1.Kustomization
		if err := json.Unmarshal([]byte(output), &kustomization); err != nil {
			t.Errorf("Expected valid JSON output, got error: %v", err)
		}

		if kustomization.Name != "test-kustomization" {
			t.Errorf("Expected kustomization name 'test-kustomization', got %q", kustomization.Name)
		}

		if !strings.Contains(output, "\"kind\"") {
			t.Error("Expected JSON output to contain JSON structure")
		}
		if got := kustomization.Spec.Path; got != "<deferred>" {
			t.Errorf("Expected deferred kustomization path to be <deferred>, got %q", got)
		}

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithRawYAMLOutput", func(t *testing.T) {
		mocks := setupShowTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization", "--raw"})
		_ = cmd.Execute()

		closePipes()

		output := stdout.String()
		if output == "" {
			t.Error("Expected non-empty stdout output")
		}

		var kustomization kustomizev1.Kustomization
		if err := yaml.Unmarshal([]byte(output), &kustomization); err != nil {
			t.Errorf("Expected valid YAML output, got error: %v", err)
		}
		if got := kustomization.Spec.Path; got != "kustomize/${terraform_output(\"cluster\", \"path\")}" {
			t.Errorf("Expected deferred path to remain expression in raw mode, got %q", got)
		}
		if strings.Contains(output, "<deferred>") {
			t.Error("Expected raw mode output to not contain <deferred>")
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		mocks := setupShowTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"nonexistent-kustomization"})
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "kustomization \"nonexistent-kustomization\" not found") {
			t.Errorf("Expected kustomization not found error, got: %v", err)
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		mocks := setupShowTest(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization"})
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("BlueprintGenerationFailure", func(t *testing.T) {
		mocks := setupShowTest(t)

		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return nil
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization"})
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to generate blueprint") {
			t.Errorf("Expected blueprint generation error, got: %v", err)
		}
	})

	t.Run("CommandInitialization", func(t *testing.T) {
		cmd := showKustomizationCmd

		if cmd.Use != "kustomization <component-name>" {
			t.Errorf("Expected Use to be 'kustomization <component-name>', got %q", cmd.Use)
		}
		if cmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if cmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}

		jsonFlag := cmd.Flags().Lookup("json")
		if jsonFlag == nil {
			t.Error("Expected --json flag to be defined")
		}
		if jsonFlag.Usage == "" {
			t.Error("Expected non-empty flag usage")
		}
		rawFlag := cmd.Flags().Lookup("raw")
		if rawFlag == nil {
			t.Error("Expected --raw flag to be defined")
		}
		if rawFlag.Usage == "" {
			t.Error("Expected non-empty raw flag usage")
		}
	})
}
