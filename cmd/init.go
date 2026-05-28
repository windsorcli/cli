package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
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
	initPlatform       string
	initBlueprint      string
	initEndpoint       string
	initSetFlags       []string
)

var initCmd = &cobra.Command{
	Use:   "init [context]",
	Short: "Scaffold or re-initialize a Windsor context.",
	Long: `Scaffold a Windsor project. Writes windsor.yaml at the project root if missing, creates contexts/<context>/, and adds the current directory to the trusted-folders list. Re-running on an existing context updates configuration; pass --reset to overwrite generated files and clean .terraform.

If no context is given, the current context is used; if none is set, 'local' is used.

The directory must be a git repository — init refuses to run in an empty or non-git directory to prevent silently scaffolding against $HOME.`,
	Example: `# Scaffold a local context with the docker VM driver
windsor init local --vm-driver=docker

# Re-initialize and overwrite generated files
windsor init local --reset

# Initialize an AWS staging context
windsor init staging --platform=aws --aws-profile=staging`,
	Annotations: map[string]string{
		"docs.seealso": "[Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle), [Contexts reference](../contexts.md)\n" +
			"[`up`](up.md), [`apply`](apply.md), [`bootstrap`](bootstrap.md)",
		"docs.source": "cmd/init.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		// Ensure the current directory is a project before resolving the runtime.
		// Without this, an `init` run in an empty directory would fall back to
		// global mode (via GetProjectRoot) and silently operate against $HOME.
		if len(rtOpts) == 0 {
			if err := shell.EnsureGitRepository(); err != nil {
				return err
			}
			if err := shell.EnsureProjectAnchor(); err != nil {
				return fmt.Errorf("failed to initialize project: %w", err)
			}
		}

		contextName := "local"
		changingContext := len(args) > 0
		tempRt := runtime.NewRuntime(rtOpts...)
		if _, err := tempRt.Shell.WriteResetToken(); err != nil {
			return fmt.Errorf("failed to write reset token: %w", err)
		}
		if changingContext {
			contextName = args[0]

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
			if currentContext != "" {
				contextName = currentContext
			}
		}

		// Build flag overrides map
		flagOverrides := make(map[string]any)
		applyWorkstationFlagOverrides(flagOverrides, initVmDriver, initPlatform)
		if initBackend != "" {
			flagOverrides["terraform.backend.type"] = initBackend
		}
		if initAwsProfile != "" {
			flagOverrides["aws.profile"] = initAwsProfile
		}
		if initAwsEndpointURL != "" {
			flagOverrides["aws.endpoint_url"] = initAwsEndpointURL
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

		if err := proj.Runtime.ConfigHandler.ValidateContextValues(); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		blueprintURL, err := resolveBlueprintURL(initBlueprint, initPlatform, contextName, rt.TemplateRoot, true)
		if err != nil {
			return err
		}
		// `windsor init` writes config files and generates infrastructure stubs locally; it
		// does not run terraform, start a workstation, or talk to a cluster — so docker /
		// colima / terraform / kubelogin are deliberately NOT requested here, letting a
		// fresh-machine init succeed before those tools are installed. Secrets is requested
		// because Project.Initialize always calls LoadEnvironment, which shells out to
		// sops / op when the context has those backends enabled (e.g. `init --reset` against
		// an existing sops-enabled context); without this, the operator would get a raw
		// "sops: command not found" instead of the registry-formatted hint. The config gates
		// inside CheckRequirements ensure contexts that haven't enabled either backend still
		// skip the actual binary check.
		proj.SetToolRequirements(tools.Requirements{Secrets: true})
		if err := proj.Initialize(initReset, blueprintURL...); err != nil {
			return err
		}

		hasSetFlags := len(initSetFlags) > 0
		if err := proj.Runtime.SaveConfig(hasSetFlags); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		fmt.Fprintln(os.Stderr, "Initialization successful")

		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initReset, "reset", false, "Overwrite existing files and clean .terraform.")
	initCmd.Flags().StringVar(&initBackend, "backend", "", "Terraform backend type.")
	initCmd.Flags().StringVar(&initAwsProfile, "aws-profile", "", "AWS profile name.")
	initCmd.Flags().StringVar(&initAwsEndpointURL, "aws-endpoint-url", "", "AWS endpoint URL.")
	initCmd.Flags().StringVar(&initVmDriver, "vm-driver", "", "VM driver: colima, colima-incus, docker-desktop, docker.")
	initCmd.Flags().IntVar(&initCpu, "vm-cpu", 0, "CPU count for the workstation VM.")
	initCmd.Flags().IntVar(&initDisk, "vm-disk", 0, "Disk size for the workstation VM (GB).")
	initCmd.Flags().IntVar(&initMemory, "vm-memory", 0, "Memory for the workstation VM (GB).")
	initCmd.Flags().StringVar(&initArch, "vm-arch", "", "CPU architecture for the workstation VM.")
	initCmd.Flags().BoolVar(&initDocker, "docker", false, "Enable Docker.")
	initCmd.Flags().BoolVar(&initGitLivereload, "git-livereload", false, "Enable git livereload.")
	initCmd.Flags().StringVar(&initPlatform, "platform", "", "Target platform: none, metal, docker, aws, azure, gcp, hyperv.")
	initCmd.Flags().StringVar(&initBlueprint, "blueprint", "", "Blueprint OCI reference or local path.")
	initCmd.Flags().StringVar(&initEndpoint, "endpoint", "", "Kubernetes API endpoint.")
	initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override config values, e.g. --set dns.enabled=false. May be repeated.")

	rootCmd.AddCommand(initCmd)
}
