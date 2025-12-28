package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/runtime"
)

var (
	nodeHealthTimeout      time.Duration
	nodeHealthNodes        []string
	nodeHealthVersion      string
	k8sEndpoint            string
	checkNodeReady         bool
	nodeHealthSkipServices []string
)

var checkCmd = &cobra.Command{
	Use:          "check",
	Short:        "Check the tool versions",
	Long:         "Check the tool versions required by the project",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt, err := runtime.NewRuntime(rtOpts...)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return fmt.Errorf("Nothing to check. Have you run \033[1mwindsor init\033[0m?")
		}

		if err := rt.CheckTools(); err != nil {
			return err
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

		if len(nodeHealthNodes) == 0 && k8sEndpoint == "" {
			return fmt.Errorf("No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
		}

		if !cmd.Flags().Changed("timeout") {
			nodeHealthTimeout = constants.DefaultNodeHealthCheckTimeout
		}

		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt, err := runtime.NewRuntime(rtOpts...)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if !rt.ConfigHandler.IsLoaded() {
			return fmt.Errorf("Nothing to check. Have you run \033[1mwindsor init\033[0m?")
		}

		comp := composer.NewComposer(rt)
		prov := provisioner.NewProvisioner(rt, comp.BlueprintHandler)

		outputFunc := func(output string) {
			fmt.Fprintln(cmd.OutOrStdout(), output)
		}

		k8sEndpointStr := k8sEndpoint
		if k8sEndpointStr == "" && checkNodeReady {
			k8sEndpointStr = "true"
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               nodeHealthNodes,
			Timeout:             nodeHealthTimeout,
			Version:             nodeHealthVersion,
			K8SEndpoint:         k8sEndpointStr,
			K8SEndpointProvided: k8sEndpoint != "" || checkNodeReady,
			CheckNodeReady:      checkNodeReady,
			SkipServices:        nodeHealthSkipServices,
		}

		if err := prov.CheckNodeHealth(cmd.Context(), options, outputFunc); err != nil {
			return fmt.Errorf("error checking node health: %w", err)
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
	checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthSkipServices, "skip-services", []string{}, "Service names to ignore during health checks (e.g., dashboard)")
}
