package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
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
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create check pipeline
		pipeline := pipelines.NewCheckPipeline()

		// Create output function
		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		// Create execution context with operation and output function
		ctx := context.WithValue(cmd.Context(), "operation", "tools")
		ctx = context.WithValue(ctx, "output", outputFunc)

		// Initialize the pipeline
		if err := pipeline.Initialize(injector, ctx); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Execute the pipeline
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing check pipeline: %w", err)
		}

		return nil
	},
}

var checkNodeHealthCmd = &cobra.Command{
	Use:          "node-health",
	Short:        "Check the health of cluster nodes",
	Long:         "Check the health status of specified cluster nodes",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create check pipeline
		pipeline := pipelines.NewCheckPipeline()

		// Require nodes to be specified
		if len(nodeHealthNodes) == 0 {
			return fmt.Errorf("No nodes specified. Use --nodes flag to specify nodes to check")
		}

		// If timeout is not set via flag, use default
		if !cmd.Flags().Changed("timeout") {
			nodeHealthTimeout = constants.DEFAULT_NODE_HEALTH_CHECK_TIMEOUT
		}

		// Create output function
		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		// Create execution context with operation, nodes, timeout, version, and output function
		ctx := context.WithValue(cmd.Context(), "operation", "node-health")
		ctx = context.WithValue(ctx, "nodes", nodeHealthNodes)
		ctx = context.WithValue(ctx, "timeout", nodeHealthTimeout)
		ctx = context.WithValue(ctx, "version", nodeHealthVersion)
		ctx = context.WithValue(ctx, "output", outputFunc)

		// Initialize the pipeline
		if err := pipeline.Initialize(injector, ctx); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Execute the pipeline
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing check pipeline: %w", err)
		}

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
