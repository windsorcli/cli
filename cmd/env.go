package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
)

var envCmd = &cobra.Command{
	Use:          "env",
	Short:        "Output commands to set environment variables",
	Long:         "Output commands to set environment variables for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		hook, _ := cmd.Flags().GetBool("hook")
		decrypt, _ := cmd.Flags().GetBool("decrypt")
		verbose, _ := cmd.Flags().GetBool("verbose")

		if !hook && os.Getenv("NO_CACHE") == "" {
			if err := os.Setenv("NO_CACHE", "true"); err != nil {
				return fmt.Errorf("failed to set NO_CACHE environment variable: %w", err)
			}
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
			if hook {
				return nil
			}
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := rt.HandleSessionReset(); err != nil {
			return err
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if err := rt.LoadEnvironment(decrypt); err != nil {
			if hook || !verbose {
				return nil
			}
			return fmt.Errorf("failed to load environment: %w", err)
		}

		outputFunc := func(output string) {
			if output != "" {
				fmt.Fprint(cmd.OutOrStdout(), output)
			}
		}

		if hook {
			if rt.Shell != nil && len(rt.GetEnvVars()) > 0 {
				outputFunc(rt.Shell.RenderEnvVars(rt.GetEnvVars(), true))
			}
			if rt.Shell != nil && len(rt.GetAliases()) > 0 {
				outputFunc(rt.Shell.RenderAliases(rt.GetAliases()))
			}
		} else {
			if rt.Shell != nil && len(rt.GetEnvVars()) > 0 {
				outputFunc(rt.Shell.RenderEnvVars(rt.GetEnvVars(), false))
			}
		}

		return nil
	},
}

func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before setting environment variables")
	envCmd.Flags().Bool("hook", false, "Flag that indicates the command is being executed by the hook")
	envCmd.Flags().Bool("verbose", false, "Show verbose error output")
	rootCmd.AddCommand(envCmd)
}
