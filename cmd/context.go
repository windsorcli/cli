package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
)

// getContextCmd represents the get command
var getContextCmd = &cobra.Command{
	Use:          "get",
	Short:        "Get the current context",
	Long:         "Retrieve and display the current context from the configuration",
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

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		contextName := rt.ConfigHandler.GetContext()
		fmt.Fprintln(cmd.OutOrStdout(), contextName)

		return nil
	},
}

// setContextCmd represents the set command
var setContextCmd = &cobra.Command{
	Use:          "set [context]",
	Short:        "Set the current context",
	Long:         "Set the current context in the configuration and save it",
	Args:         cobra.ExactArgs(1),
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

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if _, err := rt.Shell.WriteResetToken(); err != nil {
			return fmt.Errorf("failed to write reset token: %w", err)
		}

		if err := rt.ConfigHandler.SetContext(args[0]); err != nil {
			return fmt.Errorf("failed to set context: %w", err)
		}

		return nil
	},
}

// getContextAliasCmd is an alias for the get command
var getContextAliasCmd = &cobra.Command{
	Use:          "get-context",
	Short:        "Alias for 'context get'",
	Long:         "Alias for 'context get'",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"context", "get"}, args...))
		return rootCmd.Execute()
	},
}

// setContextAliasCmd is an alias for the set command
var setContextAliasCmd = &cobra.Command{
	Use:          "set-context [context]",
	Short:        "Alias for 'context set'",
	SilenceUsage: true,
	Long:         "Alias for 'context set'",
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rootCmd.SetArgs(append([]string{"context", "set"}, args...))
		return rootCmd.Execute()
	},
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "context",
		Short: "Manage contexts",
		Long:  "Manage contexts for the application",
	})

	contextCmd := rootCmd.Commands()[len(rootCmd.Commands())-1]
	contextCmd.AddCommand(getContextCmd)
	contextCmd.AddCommand(setContextCmd)

	rootCmd.AddCommand(getContextAliasCmd)
	rootCmd.AddCommand(setContextAliasCmd)
}
