package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/project"
)

var composeBlueprintJSON bool
var composeKustomizationJSON bool

var composeCmd = &cobra.Command{
	Use:   "compose",
	Short: "Compose and output resources to stdout",
	Long:  "Compose and output resources to stdout",
}

var composeBlueprintCmd = &cobra.Command{
	Use:          "blueprint",
	Short:        "Output the fully compiled blueprint",
	Long:         "Output the fully compiled blueprint to stdout before cleaning/removing transient fields. Defaults to YAML output, use --json for JSON output.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		blueprint, err := getBlueprint(cmd)
		if err != nil {
			return err
		}

		return outputResource(blueprint, composeBlueprintJSON, "blueprint")
	},
}

var composeKustomizationCmd = &cobra.Command{
	Use:          "kustomization <component-name>",
	Short:        "Output the Flux Kustomization resource for a component",
	Long:         "Output the Flux Kustomization resource for the specified component to stdout. Defaults to YAML output, use --json for JSON output.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentName := args[0]

		blueprint, err := getBlueprint(cmd)
		if err != nil {
			return err
		}

		var kustomization *blueprintv1alpha1.Kustomization
		for i := range blueprint.Kustomizations {
			if blueprint.Kustomizations[i].Name == componentName {
				kustomization = &blueprint.Kustomizations[i]
				break
			}
		}

		if kustomization == nil {
			return fmt.Errorf("kustomization %q not found in blueprint", componentName)
		}

		defaultSourceName := blueprint.Metadata.Name
		namespace := constants.DefaultFluxSystemNamespace
		fluxKustomization := kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources)

		return outputResource(fluxKustomization, composeKustomizationJSON, "kustomization")
	},
}

func init() {
	composeBlueprintCmd.Flags().BoolVar(&composeBlueprintJSON, "json", false, "Output blueprint as JSON instead of YAML")
	composeKustomizationCmd.Flags().BoolVar(&composeKustomizationJSON, "json", false, "Output kustomization as JSON instead of YAML")
	composeCmd.AddCommand(composeBlueprintCmd)
	composeCmd.AddCommand(composeKustomizationCmd)
	rootCmd.AddCommand(composeCmd)
}

// getBlueprint initializes a project and generates the composed blueprint.
// It handles project creation, configuration, initialization, and blueprint generation.
// Returns the generated blueprint or an error if any step fails.
func getBlueprint(cmd *cobra.Command) (*blueprintv1alpha1.Blueprint, error) {
	var opts []*project.Project
	if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
		opts = []*project.Project{overridesVal.(*project.Project)}
	}

	proj, err := project.NewProject("", opts...)
	if err != nil {
		return nil, err
	}

	proj.Runtime.Shell.SetVerbosity(verbose)

	if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
		return nil, fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
	}

	if err := proj.Configure(nil); err != nil {
		return nil, err
	}

	if err := proj.Initialize(false); err != nil {
		return nil, err
	}

	blueprint := proj.Composer.BlueprintHandler.Generate()
	if blueprint == nil {
		return nil, fmt.Errorf("failed to generate blueprint")
	}

	return blueprint, nil
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
