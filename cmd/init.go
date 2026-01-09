package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Init Command
// =============================================================================

var (
	initReset          bool
	initBackend        string
	initAwsProfile     string
	initAwsEndpointURL string
	initVmDriver       string
	initCpu            int
	initDisk           int
	initMemory         int
	initArch           string
	initDocker         bool
	initGitLivereload  bool
	initProvider       string
	initPlatform       string
	initBlueprint      string
	initEndpoint       string
	initSetFlags       []string
)

var initCmd = &cobra.Command{
	Use:          "init [context]",
	Short:        "Initialize the application environment",
	Long:         "Initialize the application environment with the specified context configuration",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		contextName := "local"
		changingContext := len(args) > 0
		if changingContext {
			contextName = args[0]

			tempRt := runtime.NewRuntime(rtOpts...)

			if _, err := tempRt.Shell.WriteResetToken(); err != nil {
				return fmt.Errorf("failed to write reset token: %w", err)
			}

			if err := tempRt.ConfigHandler.SetContext(contextName); err != nil {
				return fmt.Errorf("failed to set context: %w", err)
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.AddCurrentDirToTrustedFile(); err != nil {
			return fmt.Errorf("failed to add current directory to trusted file: %w", err)
		}

		if !changingContext {
			currentContext := rt.ConfigHandler.GetContext()
			if currentContext != "" && currentContext != "local" {
				contextName = currentContext
			}
		}

		if initPlatform != "" {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: The --platform flag is deprecated and will be removed in a future version. Please use --provider instead.\033[0m\n")
			initProvider = initPlatform
		}

		// Build flag overrides map
		flagOverrides := make(map[string]any)
		if initProvider != "" {
			flagOverrides["provider"] = initProvider
		}
		if initBackend != "" {
			flagOverrides["terraform.backend.type"] = initBackend
		}
		if initAwsProfile != "" {
			flagOverrides["aws.profile"] = initAwsProfile
		}
		if initAwsEndpointURL != "" {
			flagOverrides["aws.endpoint_url"] = initAwsEndpointURL
		}
		if initVmDriver != "" {
			flagOverrides["vm.driver"] = initVmDriver
		}
		if initCpu > 0 {
			flagOverrides["vm.cpu"] = initCpu
		}
		if initDisk > 0 {
			flagOverrides["vm.disk"] = initDisk
		}
		if initMemory > 0 {
			flagOverrides["vm.memory"] = initMemory
		}
		if initArch != "" {
			flagOverrides["vm.arch"] = initArch
		}
		if initDocker {
			flagOverrides["docker.enabled"] = true
		}
		if initGitLivereload {
			flagOverrides["git.livereload.enabled"] = true
		}

		for _, setFlag := range initSetFlags {
			parts := strings.SplitN(setFlag, "=", 2)
			if len(parts) == 2 {
				flagOverrides[parts[0]] = parts[1]
			}
		}

		var projectOpts *project.Project
		if composerOverrideVal := cmd.Context().Value(composerOverridesKey); composerOverrideVal != nil {
			compOverride := composerOverrideVal.(*composer.Composer)
			projectOpts = &project.Project{
				Runtime:  rt,
				Composer: compOverride,
			}
		} else {
			projectOpts = &project.Project{
				Runtime: rt,
			}
		}

		proj := project.NewProject(contextName, projectOpts)

		if err := proj.Configure(flagOverrides); err != nil {
			return err
		}

		if !changingContext {
			if err := rt.HandleSessionReset(); err != nil {
				return fmt.Errorf("failed to handle session reset: %w", err)
			}
		}

		var blueprintURL []string
		if initBlueprint != "" {
			blueprintURL = []string{initBlueprint}
		}
		if err := proj.Initialize(initReset, blueprintURL...); err != nil {
			return err
		}

		hasSetFlags := len(initSetFlags) > 0
		if err := proj.Runtime.ConfigHandler.SaveConfig(hasSetFlags); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		fmt.Fprintln(os.Stderr, "Initialization successful")

		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initReset, "reset", false, "Reset/overwrite existing files and clean .terraform directory")
	initCmd.Flags().StringVar(&initBackend, "backend", "", "Specify the backend to use")
	initCmd.Flags().StringVar(&initAwsProfile, "aws-profile", "", "Specify the AWS profile to use")
	initCmd.Flags().StringVar(&initAwsEndpointURL, "aws-endpoint-url", "", "Specify the AWS endpoint URL to use")
	initCmd.Flags().StringVar(&initVmDriver, "vm-driver", "", "Specify the VM driver. Only Colima is supported for now.")
	initCmd.Flags().IntVar(&initCpu, "vm-cpu", 0, "Specify the number of CPUs for Colima")
	initCmd.Flags().IntVar(&initDisk, "vm-disk", 0, "Specify the disk size for Colima")
	initCmd.Flags().IntVar(&initMemory, "vm-memory", 0, "Specify the memory size for Colima")
	initCmd.Flags().StringVar(&initArch, "vm-arch", "", "Specify the architecture for Colima")
	initCmd.Flags().BoolVar(&initDocker, "docker", false, "Enable Docker")
	initCmd.Flags().BoolVar(&initGitLivereload, "git-livereload", false, "Enable Git Livereload")
	initCmd.Flags().StringVar(&initProvider, "provider", "", "Specify the provider to use [none|generic|aws|azure|gcp]")
	initCmd.Flags().StringVar(&initPlatform, "platform", "", "Deprecated: use --provider instead")
	initCmd.Flags().StringVar(&initBlueprint, "blueprint", "", "Specify the blueprint to use")
	initCmd.Flags().StringVar(&initEndpoint, "endpoint", "", "Specify the kubernetes API endpoint")
	initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false --set cluster.endpoint=https://localhost:6443")

	rootCmd.AddCommand(initCmd)
}
