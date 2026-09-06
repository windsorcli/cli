package terraform

// The stack_show file is a terraform plan document retriever for the Stack.
// It provides PlanResourceChangesJSON and PlanAllResourceChangesJSON, which run
// terraform plan and show -json and print the resulting resource-changes document.
// Both plan and show steps use capture-only exec, since the document can contain
// sensitive values and must never echo to the terminal, even under --verbose.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
)

// =============================================================================
// Public Methods
// =============================================================================

// PlanResourceChangesJSON runs terraform init, plan, and show -json for a single component
// identified by componentID, printing the resulting plan document to stdout. Returns an
// error if the component is not found, the directory does not exist, or any terraform
// operation fails.
func (s *TerraformStack) PlanResourceChangesJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	component, terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()

	return s.planResourceChangesJSON(component, terraformVars, scopedKeys, terraformArgs)
}

// PlanAllResourceChangesJSON runs terraform init, plan, and show -json for every enabled
// component in the blueprint, streaming each component's plan document directly to stdout.
// Stops on the first error. Returns an error if blueprint is nil or any component's
// terraform operation fails.
func (s *TerraformStack) PlanAllResourceChangesJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("error getting project root: project root is empty")
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)
	for i := range components {
		component := &components[i]

		s.printComponentHeader(component.Path)

		terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
		if err != nil {
			return err
		}
		err = s.planResourceChangesJSON(component, terraformVars, scopedKeys, terraformArgs)
		cleanup()
		if err != nil {
			return err
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// planResourceChangesJSON runs terraform plan (writing a plan file), then terraform show
// -json against that plan file, and prints the resulting document to stdout. Both steps use
// ExecCaptureWithEnv, never ExecSilentWithEnv: the plan diff and the show document can both
// carry the same sensitive plaintext, and neither may echo to the terminal under --verbose.
//
// Windsor's shell layer still scrubs any value registered via RegisterSecret (secrets sourced
// through 'secret(...)' from a vault/SOPS provider) to "********" in this output, the same as
// every other windsor command. Values a provider or resource computes itself (a generated
// password, a TLS private key) are not registered, and so are not scrubbed: they reach stdout
// as terraform produced them.
func (s *TerraformStack) planResourceChangesJSON(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs) error {
	terraformVars["TF_VAR_operation"] = "apply"

	if err := s.runTerraformInit(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
		return err
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)

	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-no-color"}
	planArgs = append(planArgs, terraformArgs.PlanArgs...)
	if _, err := s.runtime.Shell.ExecCaptureWithEnv(terraformCommand, planEnv, planArgs...); err != nil {
		return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
	}

	tfPlanPath := filepath.ToSlash(filepath.Join(terraformArgs.TFDataDir, "terraform.tfplan"))
	showArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "show", "-json", tfPlanPath}
	showOutput, err := s.runtime.Shell.ExecCaptureWithEnv(terraformCommand, planEnv, showArgs...)
	if err != nil {
		return fmt.Errorf("error reading terraform plan JSON for %s: %w", component.Path, err)
	}

	compacted, err := compactPlanJSON(showOutput)
	if err != nil {
		return fmt.Errorf("error parsing terraform plan JSON for %s: %w", component.Path, err)
	}
	fmt.Fprintln(os.Stdout, compacted)

	return nil
}

// compactPlanJSON strips insignificant whitespace from a terraform show -json document, so
// PlanAllResourceChangesJSON's per-component output is newline-delimited and safe to split.
// It uses json.Compact rather than a decode/re-encode round trip: json.Compact validates and
// reformats the raw bytes without touching values, so it neither reorders terraform's object
// keys nor HTML-escapes characters like '&' the way json.Marshal would.
func compactPlanJSON(raw string) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(raw)); err != nil {
		return "", err
	}
	return buf.String(), nil
}
