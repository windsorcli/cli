package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// getCmd represents the get command group
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Display one or many resources",
	Long:  "Display one or many resources",
}

// getContextsCmd lists all available contexts
var getContextsCmd = &cobra.Command{
	Use:          "contexts",
	Short:        "List all available contexts",
	Long:         "List all available contexts in the project. The current context is marked with an asterisk (*).",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt, err := runtime.NewRuntime(rtOpts...)
		if err != nil {
			return fmt.Errorf("failed to initialize runtime: %w", err)
		}

		projectRoot, err := rt.Shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("failed to get project root: %w", err)
		}

		contextsDir := filepath.Join(projectRoot, "contexts")
		entries, err := os.ReadDir(contextsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(cmd.OutOrStdout(), "No contexts found")
				return nil
			}
			return fmt.Errorf("failed to read contexts directory: %w", err)
		}

		var contexts []string
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && entry.Name() != "_template" {
				contexts = append(contexts, entry.Name())
			}
		}

		if len(contexts) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No contexts found")
			return nil
		}

		sort.Strings(contexts)

		currentContext := rt.ConfigHandler.GetContext()

		type contextInfo struct {
			name     string
			provider string
			backend  string
			current  bool
		}

		var contextInfos []contextInfo
		for _, ctx := range contexts {
			provider := "<none>"
			backend := "<none>"
			ctxConfigHandler := config.NewConfigHandler(rt.Shell)
			ctxConfigHandler.SetContext(ctx)
			if err := ctxConfigHandler.LoadConfig(); err == nil {
				if p := ctxConfigHandler.GetString("provider"); p != "" {
					provider = p
				}
				if b := ctxConfigHandler.GetString("terraform.backend.type"); b != "" {
					backend = b
				}
			}

			contextInfos = append(contextInfos, contextInfo{
				name:     ctx,
				provider: provider,
				backend:  backend,
				current:  ctx == currentContext,
			})
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tPROVIDER\tBACKEND\tCURRENT")
		for _, info := range contextInfos {
			current := ""
			if info.current {
				current = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", info.name, info.provider, info.backend, current)
		}
		w.Flush()

		return nil
	},
}

// getContextCmd gets the current context
var getContextCmd = &cobra.Command{
	Use:          "context",
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
			return fmt.Errorf("failed to initialize runtime: %w", err)
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		contextName := rt.ConfigHandler.GetContext()
		fmt.Fprintln(cmd.OutOrStdout(), contextName)

		return nil
	},
}

func init() {
	getCmd.AddCommand(getContextsCmd)
	getCmd.AddCommand(getContextCmd)
	rootCmd.AddCommand(getCmd)
}
