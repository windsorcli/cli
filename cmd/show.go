package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/project"
)

var showBlueprintJSON bool
var showKustomizationJSON bool

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Display rendered resources",
	Long:  "Display fully rendered resources to stdout, including all computed fields from blueprint composition.",
}

var showBlueprintCmd = &cobra.Command{
	Use:          "blueprint",
	Short:        "Display the fully rendered blueprint",
	Long:         "Display the fully rendered blueprint to stdout, including all fields from underlying sources and computed values. Defaults to YAML, use --json for JSON.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		blueprint, validationErr := getBlueprint(cmd)
		if blueprint == nil {
			if validationErr != nil {
				return validationErr
			}
			return fmt.Errorf("failed to generate blueprint")
		}

		if err := outputResource(blueprint, showBlueprintJSON, "blueprint"); err != nil {
			return err
		}

		if validationErr != nil {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: %v\033[0m\n", validationErr)
		}

		return nil
	},
}

var showKustomizationCmd = &cobra.Command{
	Use:          "kustomization <component-name>",
	Short:        "Display the Flux Kustomization resource for a component",
	Long:         "Display the Flux Kustomization resource for the specified component to stdout. Defaults to YAML, use --json for JSON.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentName := args[0]

		blueprint, validationErr := getBlueprint(cmd)
		if blueprint == nil {
			if validationErr != nil {
				return validationErr
			}
			return fmt.Errorf("failed to generate blueprint")
		}

		kustomization := findKustomization(blueprint, componentName)
		if kustomization == nil {
			return errKustomizationNotFound(componentName)
		}

		fluxKustomization := buildFluxKustomization(blueprint, kustomization)

		if err := outputResource(fluxKustomization, showKustomizationJSON, "kustomization"); err != nil {
			return err
		}

		if validationErr != nil {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: %v\033[0m\n", validationErr)
		}

		return nil
	},
}

func init() {
	showBlueprintCmd.Flags().BoolVar(&showBlueprintJSON, "json", false, "Output as JSON instead of YAML")
	showKustomizationCmd.Flags().BoolVar(&showKustomizationJSON, "json", false, "Output as JSON instead of YAML")
	showCmd.AddCommand(showBlueprintCmd)
	showCmd.AddCommand(showKustomizationCmd)
	rootCmd.AddCommand(showCmd)
}

// =============================================================================
// Helper Functions
// =============================================================================

// getBlueprint configures the project and composes the blueprint without running full initialization.
// It loads blueprint sources and composes them; it does not write blueprint files, process terraform
// modules, or generate tfvars. Returns the composed blueprint and any composition errors. Composition
// errors are non-fatal and allow the blueprint to be returned for inspection when possible.
func getBlueprint(cmd *cobra.Command) (*blueprintv1alpha1.Blueprint, error) {
	var opts []*project.Project
	if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
		opts = []*project.Project{overridesVal.(*project.Project)}
	}

	proj := project.NewProject("", opts...)

	proj.Runtime.Shell.SetVerbosity(verbose)

	if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
		return nil, fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
	}

	if err := proj.Configure(nil); err != nil {
		return nil, err
	}

	var validationErr error
	if err := proj.ComposeBlueprint(); err != nil {
		validationErr = err
	}

	blueprint := proj.Composer.BlueprintHandler.Generate()
	if blueprint == nil {
		if validationErr != nil {
			return nil, validationErr
		}
		return nil, fmt.Errorf("failed to generate blueprint")
	}

	return blueprint, validationErr
}

// outputResource serializes the provided resource to YAML or JSON and writes it to stdout.
// If useJSON is true, the resource is marshaled as JSON with indentation; otherwise, it's marshaled as YAML.
// The resourceType parameter is used in error messages to identify the type of resource being output.
func outputResource(resource any, useJSON bool, resourceType string) error {
	var output []byte
	var err error

	if useJSON {
		output, err = json.MarshalIndent(resource, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal %s to JSON: %w", resourceType, err)
		}
	} else {
		output, err = yaml.Marshal(resource)
		if err != nil {
			return fmt.Errorf("failed to marshal %s to YAML: %w", resourceType, err)
		}
	}

	fmt.Print(string(output))
	return nil
}

// findKustomization searches for a kustomization by name in the blueprint and returns a pointer
// to it if found, or nil if not found.
func findKustomization(blueprint *blueprintv1alpha1.Blueprint, name string) *blueprintv1alpha1.Kustomization {
	for i := range blueprint.Kustomizations {
		if blueprint.Kustomizations[i].Name == name {
			return &blueprint.Kustomizations[i]
		}
	}
	return nil
}

// errKustomizationNotFound returns a formatted error for when a kustomization is not found.
func errKustomizationNotFound(name string) error {
	return fmt.Errorf("kustomization %q not found in blueprint", name)
}

// buildFluxKustomization converts a blueprint Kustomization to a Flux Kustomization resource.
func buildFluxKustomization(blueprint *blueprintv1alpha1.Blueprint, kustomization *blueprintv1alpha1.Kustomization) kustomizev1.Kustomization {
	defaultSourceName := blueprint.Metadata.Name
	namespace := constants.DefaultFluxSystemNamespace
	return kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources)
}
