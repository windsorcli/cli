package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	blueprintcomposer "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

var showBlueprintJSON bool
var showBlueprintRaw bool
var showKustomizationJSON bool
var showKustomizationRaw bool
var showValuesJSON bool

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Display rendered resources",
	Long:  "Display fully rendered resources to stdout, including all computed fields from blueprint composition.",
}

var showBlueprintCmd = &cobra.Command{
	Use:          "blueprint",
	Short:        "Display the fully rendered blueprint",
	Long:         "Display the fully rendered blueprint to stdout, including all fields from underlying sources and computed values. Defaults to YAML, use --json for JSON. Unresolved deferred values are shown as <deferred> by default; use --raw to show expression text.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		blueprint, deferredPaths, validationErr := getBlueprint(cmd)
		if blueprint == nil {
			if validationErr != nil {
				return validationErr
			}
			return fmt.Errorf("failed to generate blueprint")
		}

		resource := blueprintcomposer.RenderDeferredPlaceholders(blueprint, showBlueprintRaw, deferredPaths)
		if err := outputResource(resource, showBlueprintJSON, "blueprint"); err != nil {
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
	Long:         "Display the Flux Kustomization resource for the specified component to stdout. Defaults to YAML, use --json for JSON. Unresolved deferred values are shown as <deferred> by default; use --raw to show expression text.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentName := args[0]

		blueprint, deferredPaths, validationErr := getBlueprint(cmd)
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
		resource := blueprintcomposer.RenderDeferredPlaceholders(fluxKustomization, showKustomizationRaw, deferredPaths)

		if err := outputResource(resource, showKustomizationJSON, "kustomization"); err != nil {
			return err
		}

		if validationErr != nil {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: %v\033[0m\n", validationErr)
		}

		return nil
	},
}

var showValuesCmd = &cobra.Command{
	Use:          "values",
	Short:        "Display the effective context values",
	Long:         "Display the effective context values to stdout, combining schema defaults with values.yaml overrides. YAML output includes schema descriptions as comments. Use --json for plain JSON.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		values, schema, err := getValues(cmd)
		if err != nil {
			return err
		}

		if showValuesJSON {
			return outputResource(values, true, "values")
		}

		fmt.Print(config.RenderValuesWithDescriptions(values, schema))
		return nil
	},
}

func init() {
	showBlueprintCmd.Flags().BoolVar(&showBlueprintJSON, "json", false, "Output as JSON instead of YAML")
	showBlueprintCmd.Flags().BoolVar(&showBlueprintRaw, "raw", false, "Output unresolved deferred values as expression text instead of <deferred>")
	showKustomizationCmd.Flags().BoolVar(&showKustomizationJSON, "json", false, "Output as JSON instead of YAML")
	showKustomizationCmd.Flags().BoolVar(&showKustomizationRaw, "raw", false, "Output unresolved deferred values as expression text instead of <deferred>")
	showValuesCmd.Flags().BoolVar(&showValuesJSON, "json", false, "Output as JSON instead of YAML")
	showCmd.AddCommand(showBlueprintCmd)
	showCmd.AddCommand(showKustomizationCmd)
	showCmd.AddCommand(showValuesCmd)
	rootCmd.AddCommand(showCmd)
}

// =============================================================================
// Helper Functions
// =============================================================================

// getBlueprint configures the project and composes the blueprint without running full initialization.
// It loads blueprint sources and composes them; it does not write blueprint files, process terraform
// modules, or generate tfvars. Returns the composed blueprint, deferred composed paths, and any
// composition errors. Composition errors are non-fatal and allow the blueprint to be returned for
// inspection when possible.
func getBlueprint(cmd *cobra.Command) (*blueprintv1alpha1.Blueprint, map[string]bool, error) {
	proj, err := configureProject(cmd)
	if err != nil {
		return nil, nil, err
	}

	var validationErr error
	if err := proj.ComposeBlueprint(); err != nil {
		validationErr = err
	}

	blueprint := proj.Composer.BlueprintHandler.Generate()
	if blueprint == nil {
		if validationErr != nil {
			return nil, nil, validationErr
		}
		return nil, nil, fmt.Errorf("failed to generate blueprint")
	}
	deferredPaths := proj.Composer.BlueprintHandler.GetDeferredPaths()
	return blueprint, deferredPaths, validationErr
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
		output, err = yaml.MarshalWithOptions(resource, yaml.UseLiteralStyleIfMultiline(true))
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

// getValues configures the project and returns the effective context values and loaded schema without
// running full initialization. It merges schema defaults with values.yaml overrides, providing the
// complete set of configuration values available for use in blueprint processing. No files are written
// or terraform modules processed. Returns values, schema (may be nil), and any error.
func getValues(cmd *cobra.Command) (map[string]any, map[string]any, error) {
	proj, err := configureProject(cmd)
	if err != nil {
		return nil, nil, err
	}

	values, err := proj.Runtime.ConfigHandler.GetContextValues()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get context values: %w", err)
	}

	schema := proj.Runtime.ConfigHandler.GetSchema()
	return values, schema, nil
}

// buildFluxKustomization converts a blueprint Kustomization to a Flux Kustomization resource,
// including blueprint-level ConfigMaps (e.g. values-common) in postBuild.substituteFrom to
// match what the kubernetes manager applies to the cluster.
func buildFluxKustomization(blueprint *blueprintv1alpha1.Blueprint, kustomization *blueprintv1alpha1.Kustomization) kustomizev1.Kustomization {
	defaultSourceName := blueprint.Metadata.Name
	namespace := constants.DefaultFluxSystemNamespace
	return kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources, blueprint.ConfigMaps)
}
