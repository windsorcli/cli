package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/constants"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	nodeHealthTimeout time.Duration
	nodeHealthNodes   []string
	nodeHealthVersion string
)

var checkCmd = &cobra.Command{
	Use:          "check",
	Short:        "Check the tool versions",
	Long:         "Check the tool versions required by the project",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			ConfigLoaded: true,
			Tools:        true,
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
			return fmt.Errorf("Nothing to check. Have you run \033[1mwindsor init\033[0m?")
		}

		// Check tools
		toolsManager := controller.ResolveToolsManager()
		if toolsManager == nil {
			return fmt.Errorf("No tools manager found")
		}
		if err := toolsManager.Check(); err != nil {
			return fmt.Errorf("Error checking tools: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "All tools are up to date.")
		return nil
	},
}

var checkNodeHealthCmd = &cobra.Command{
	Use:          "node-health",
	Short:        "Check the health of cluster nodes",
	Long:         "Check the health status of specified cluster nodes",
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
			return fmt.Errorf("Nothing to check. Have you run \033[1mwindsor init\033[0m?")
		}

		// Get the cluster client
		clusterClient := controller.ResolveClusterClient()
		if clusterClient == nil {
			return fmt.Errorf("No cluster client found")
		}
		defer clusterClient.Close()

		// Require nodes to be specified
		if len(nodeHealthNodes) == 0 {
			return fmt.Errorf("No nodes specified. Use --nodes flag to specify nodes to check")
		}

		// If timeout is not set via flag, use default
		if !cmd.Flags().Changed("timeout") {
			nodeHealthTimeout = constants.DEFAULT_NODE_HEALTH_CHECK_TIMEOUT
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(cmd.Context(), nodeHealthTimeout)
		defer cancel()

		// Wait for nodes to be healthy (and correct version if specified)
		if err := clusterClient.WaitForNodesHealthy(ctx, nodeHealthNodes, nodeHealthVersion); err != nil {
			return fmt.Errorf("nodes failed health check: %w", err)
		}

		// Success message
		fmt.Fprintf(cmd.OutOrStdout(), "All %d nodes are healthy", len(nodeHealthNodes))
		if nodeHealthVersion != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " and running version %s", nodeHealthVersion)
		}
		fmt.Fprintln(cmd.OutOrStdout())

		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.AddCommand(checkNodeHealthCmd)

	// Add flags for node health check
	checkNodeHealthCmd.Flags().DurationVar(&nodeHealthTimeout, "timeout", 0, "Maximum time to wait for nodes to be ready (default 5m)")
	checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthNodes, "nodes", []string{}, "Nodes to check (required)")
	checkNodeHealthCmd.Flags().StringVar(&nodeHealthVersion, "version", "", "Expected version to check against (optional)")
	_ = checkNodeHealthCmd.MarkFlagRequired("nodes")
}
