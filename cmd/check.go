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
	k8sEndpoint       string
	checkNodeReady    bool
)

var checkCmd = &cobra.Command{
	Use:          "check",
	Short:        "Check the tool versions",
	Long:         "Check the tool versions required by the project",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create output function
		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		// Create execution context with operation and output function
		ctx := context.WithValue(cmd.Context(), "operation", "tools")
		ctx = context.WithValue(ctx, "output", outputFunc)

		// Set up the check pipeline
		pipeline, err := pipelines.WithPipeline(injector, ctx, "checkPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up check pipeline: %w", err)
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

		// Require at least one health check type to be specified
		if len(nodeHealthNodes) == 0 && k8sEndpoint == "" {
			return fmt.Errorf("No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
		}

		// If timeout is not set via flag, use default
		if !cmd.Flags().Changed("timeout") {
			nodeHealthTimeout = constants.DefaultNodeHealthCheckTimeout
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
		ctx = context.WithValue(ctx, "k8s-endpoint", k8sEndpoint)
		ctx = context.WithValue(ctx, "k8s-endpoint-provided", k8sEndpoint != "" || checkNodeReady)
		ctx = context.WithValue(ctx, "check-node-ready", checkNodeReady)
		ctx = context.WithValue(ctx, "output", outputFunc)

		// Set up the check pipeline
		pipeline, err := pipelines.WithPipeline(injector, ctx, "checkPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up check pipeline: %w", err)
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
	checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthNodes, "nodes", []string{}, "Nodes to check (optional)")
	checkNodeHealthCmd.Flags().StringVar(&nodeHealthVersion, "version", "", "Expected version to check against (optional)")
	checkNodeHealthCmd.Flags().StringVar(&k8sEndpoint, "k8s-endpoint", "", "Perform Kubernetes API health check (use --k8s-endpoint or --k8s-endpoint=https://endpoint:6443)")
	checkNodeHealthCmd.Flags().Lookup("k8s-endpoint").NoOptDefVal = "true"
	checkNodeHealthCmd.Flags().BoolVar(&checkNodeReady, "ready", false, "Check Kubernetes node readiness status")
}
