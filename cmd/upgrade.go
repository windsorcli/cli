package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	upgradeNodes []string
	upgradeImage string
)

var upgradeCmd = &cobra.Command{
	Use:          "upgrade",
	Short:        "Upgrade cluster resources",
	Long:         "Upgrade cluster nodes and other resources",
	SilenceUsage: true,
}

var upgradeClusterCmd = &cobra.Command{
	Use:          "cluster",
	Short:        "Upgrade cluster nodes",
	Long:         "Upgrade specified cluster nodes to a new version",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			ConfigLoaded: true,
			Cluster:      true,
			CommandName:  cmd.Name(),
			Flags: map[string]bool{
				"verbose": cmd.Flags().Changed("verbose"),
			},
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Check if projectName is set in the configuration
		configHandler := controller.ResolveConfigHandler()
		if !configHandler.IsLoaded() {
			return fmt.Errorf("Nothing to upgrade. Have you run \033[1mwindsor init\033[0m?")
		}

		// Get the cluster client
		clusterClient := controller.ResolveClusterClient()
		if clusterClient == nil {
			return fmt.Errorf("No cluster client found")
		}
		defer clusterClient.Close()

		// Require nodes to be specified
		if len(upgradeNodes) == 0 {
			return fmt.Errorf("No nodes specified. Use --nodes flag to specify nodes to upgrade")
		}

		// For Talos clusters, require --image flag
		if upgradeImage == "" {
			return fmt.Errorf("--image flag is required for cluster upgrades")
		}

		// Perform the upgrade
		if err := clusterClient.UpgradeNodes(cmd.Context(), upgradeNodes, upgradeImage); err != nil {
			return fmt.Errorf("node upgrade failed: %w", err)
		}

		// Success message
		fmt.Fprintf(cmd.OutOrStdout(), "Successfully initiated upgrade for %d nodes to image %s\n", len(upgradeNodes), upgradeImage)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.AddCommand(upgradeClusterCmd)

	// Add flags for cluster upgrade
	upgradeClusterCmd.Flags().StringSliceVar(&upgradeNodes, "nodes", []string{}, "Nodes to upgrade (required)")
	upgradeClusterCmd.Flags().StringVar(&upgradeImage, "image", "", "Image to upgrade to (required for Talos)")
	_ = upgradeClusterCmd.MarkFlagRequired("nodes")
	_ = upgradeClusterCmd.MarkFlagRequired("image")
}
