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

type ComposeMocks struct {
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	Shims            *Shims
	BlueprintHandler *blueprint.MockBlueprintHandler
	Runtime          *runtime.Runtime
	TmpDir           string
}

func setupComposeTest(t *testing.T, opts ...*SetupOptions) *ComposeMocks {
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
			Description: "Test blueprint for compose command",
		},
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{
				Name: "test-component",
				Path: "test/path",
			},
		},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{
				Name: "test-kustomization",
			},
		},
	}

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }

	rt, err := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	return &ComposeMocks{
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

func TestComposeBlueprintCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:          "blueprint",
			Short:        "Output the fully compiled blueprint",
			RunE:         composeBlueprintCmd.RunE,
			SilenceUsage: true,
		}

		composeBlueprintCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

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
		mocks := setupComposeTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err = cmd.Execute()

		closePipes()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

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

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithJSONOutput", func(t *testing.T) {
		mocks := setupComposeTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"--json"})
		err = cmd.Execute()

		closePipes()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

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

		if !strings.Contains(output, "\"Kind\"") {
			t.Error("Expected JSON output to contain JSON structure")
		}

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		mocks := setupComposeTest(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err = cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("BlueprintGenerationFailure", func(t *testing.T) {
		mocks := setupComposeTest(t)

		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return nil
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})
		err = cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to generate blueprint") {
			t.Errorf("Expected blueprint generation error, got: %v", err)
		}
	})

	t.Run("CommandInitialization", func(t *testing.T) {
		cmd := composeBlueprintCmd

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
	})

	t.Run("ComposeCommandInitialization", func(t *testing.T) {
		cmd := composeCmd

		if cmd.Use != "compose" {
			t.Errorf("Expected Use to be 'compose', got %q", cmd.Use)
		}
		if cmd.Short == "" {
			t.Error("Expected non-empty Short description")
		}
		if cmd.Long == "" {
			t.Error("Expected non-empty Long description")
		}
	})
}

func TestComposeKustomizationCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:          "kustomization",
			Short:        "Output the Flux Kustomization resource for a component",
			RunE:         composeKustomizationCmd.RunE,
			SilenceUsage: true,
		}

		composeKustomizationCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

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
		mocks := setupComposeTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization"})
		err = cmd.Execute()

		closePipes()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

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

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithJSONOutput", func(t *testing.T) {
		mocks := setupComposeTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		stdout, stderr, closePipes := setupOutput(t)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization", "--json"})
		err = cmd.Execute()

		closePipes()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

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

		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		mocks := setupComposeTest(t)

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"nonexistent-kustomization"})
		err = cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "kustomization \"nonexistent-kustomization\" not found") {
			t.Errorf("Expected kustomization not found error, got: %v", err)
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		mocks := setupComposeTest(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization"})
		err = cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("BlueprintGenerationFailure", func(t *testing.T) {
		mocks := setupComposeTest(t)

		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return nil
		}

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler

		proj, err := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: comp,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"test-kustomization"})
		err = cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to generate blueprint") {
			t.Errorf("Expected blueprint generation error, got: %v", err)
		}
	})

	t.Run("CommandInitialization", func(t *testing.T) {
		cmd := composeKustomizationCmd

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
	})
}
