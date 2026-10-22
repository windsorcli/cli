// The init command creates a Windsor context or initializes it again.
// It changes its flags into config overrides and resolves the blueprint source.

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
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

// initCmd creates a context or initializes it again.
// It makes the current directory a project first, so that init does not fall back to global mode and change $HOME.
// It saves the config, with the platform, the --set values and the context ID, after validation and before blueprint composition.
// A failed facet requires check then leaves a values.yaml that the user can complete before the next run.
// The save comes after validation, because it removes the deprecated provider key.
// It requests only the secrets tools, so that init works on a new machine before docker, terraform or kubelogin are installed.
// The secrets tools are necessary, because Initialize loads the environment, which can run sops or op.
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
		"docs.seealso": "[`up`](up.md), [`apply`](apply.md), [`bootstrap`](bootstrap.md)",
		"docs.source":  "cmd/init.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		if len(rtOpts) == 0 {
			if err := shell.EnsureGitRepository(); err != nil {
				return err
			}
			anchorDir, err := shell.EnsureProjectAnchor()
			if err != nil {
				return werror.Wrap(err, "failed to initialize project")
			}
			if cwd, err := os.Getwd(); err == nil && anchorDir != cwd {
				fmt.Fprintf(os.Stderr, "Windsor uses the existing project at %s.\n", anchorDir)
			}
		}

		contextName := "local"
		changingContext := len(args) > 0
		tempRt := runtime.NewRuntime(rtOpts...)
		if _, err := tempRt.Shell.WriteResetToken(); err != nil {
			return werror.Wrap(err, "failed to write reset token")
		}
		if changingContext {
			contextName = args[0]

			if err := tempRt.ConfigHandler.SetContext(contextName); err != nil {
				return werror.Wrap(err, "failed to set context")
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.AddCurrentDirToTrustedFile(); err != nil {
			return werror.Wrap(err, "failed to add current directory to trusted file")
		}

		if !changingContext {
			currentContext := rt.ConfigHandler.GetContext()
			if currentContext != "" {
				contextName = currentContext
			}
		}

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
			return werror.Wrap(err, "invalid configuration")
		}

		hasSetFlags := len(initSetFlags) > 0
		if err := proj.Runtime.ConfigHandler.GenerateContextID(); err != nil {
			return werror.Wrap(err, "failed to generate context ID")
		}
		if err := proj.Runtime.SaveConfig(hasSetFlags); err != nil {
			return werror.Wrap(err, "failed to save configuration")
		}

		blueprintURL, err := resolveBlueprintURL(initBlueprint, initPlatform, contextName, rt.TemplateRoot, true, proj.Composer.ArtifactBuilder)
		if err != nil {
			return err
		}
		proj.SetToolRequirements(tools.Requirements{Secrets: true})
		if err := proj.Initialize(initReset, blueprintURL...); err != nil {
			return err
		}

		if err := proj.Runtime.SaveConfig(hasSetFlags); err != nil {
			return werror.Wrap(err, "failed to save configuration")
		}

		fmt.Fprintln(os.Stderr, "Initialization successful")

		return nil
	},
}

// init registers the init command and its flags with the root command.
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
	initCmd.Flags().StringVar(&initPlatform, "platform", "", "Target platform: none, docker, incus, metal, hetzner, aws, azure, gcp, hyperv, vsphere, proxmox.")
	initCmd.Flags().StringVar(&initBlueprint, "blueprint", "", "Blueprint OCI reference or local path.")
	initCmd.Flags().StringVar(&initEndpoint, "endpoint", "", "Kubernetes API endpoint.")
	initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override config values, e.g. --set dns.enabled=false. May be repeated.")

	rootCmd.AddCommand(initCmd)
}
