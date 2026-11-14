// The migrate command converts a v1alpha1 project to the v1alpha2 layout.
// It also holds the notice that the other commands print when they load a v1alpha1 file.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/migrate"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Migrate Command
// =============================================================================

var migrateDryRun bool

// migrateCmd converts every v1alpha1 source of the project into the v1alpha2 form.
// It writes nothing when any context has a conflict, and then exits with an error.
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Convert a v1alpha1 project to v1alpha2.",
	Long: `Move the per-context settings from the root windsor.yaml and from contexts/<name>/windsor.yaml into contexts/<name>/values.yaml, and rewrite the root windsor.yaml to version v1alpha2. The command maps provider to platform and copies a root terraform.driver into each context.

If a key has different values in a source and in values.yaml, the command writes nothing, lists each conflict, and exits with an error. Resolve the conflicts and run the command again. A project with nothing to migrate exits with code 0.`,
	Example: `# List the files the command would write and delete
windsor migrate --dry-run

# Convert the project
windsor migrate`,
	Annotations: map[string]string{
		"docs.seealso": "[`init`](init.md), [`upgrade`](upgrade.md)",
		"docs.source":  "cmd/migrate.go",
	},
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := migrateProjectRoot(cmd)
		if err != nil {
			return err
		}

		migrator := migrate.NewMigrator(nil)
		plan, err := migrator.Plan(projectRoot)
		if err != nil {
			return err
		}
		if len(plan.Conflicts) > 0 {
			fmt.Fprint(cmd.ErrOrStderr(), plan.Format(migrateDryRun))
			return werror.New(fmt.Sprintf("The project has %d conflict(s).", len(plan.Conflicts)),
				"Resolve the conflicts, then run 'windsor migrate' again.", nil)
		}
		if !migrateDryRun {
			if err := migrator.Apply(plan); err != nil {
				return err
			}
		}
		fmt.Fprint(cmd.OutOrStdout(), plan.Format(migrateDryRun))
		return nil
	},
}

// migrateProjectRoot returns the project root, from the runtime override in tests or from the shell.
func migrateProjectRoot(cmd *cobra.Command) (string, error) {
	var rtOpts []*runtime.Runtime
	if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
		rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
	}
	rt := runtime.NewRuntime(rtOpts...)
	projectRoot, err := rt.Shell.GetProjectRoot()
	if err != nil {
		return "", werror.Wrap(err, "failed to find the project root")
	}
	return projectRoot, nil
}

// requireMigratedProject returns an error that lists the v1alpha1 files of a project.
// It returns nil when the project holds none.
func requireMigratedProject(projectRoot string) error {
	files := migrate.NewMigrator(nil).LegacyFiles(projectRoot)
	if len(files) == 0 {
		return nil
	}
	return werror.New(
		"The project has v1alpha1 files:\n  "+strings.Join(files, "\n  "),
		"Run 'windsor migrate' first, then run the command again.", nil)
}

// =============================================================================
// Migration Notice
// =============================================================================

// sessionTokenPattern matches a session token that is safe to use in a file name.
var sessionTokenPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// noticeSkipCommands lists the commands that never print the migration notice.
var noticeSkipCommands = map[string]bool{
	"migrate": true, "init": true, "version": true, "hook": true,
	"help": true, "completion": true, "__complete": true, "__completeNoDesc": true,
}

// printMigrationNotice prints one notice to stderr when the project holds v1alpha1 files.
// The notice names each file and says to run 'windsor migrate'. The shell hook runs 'env --hook'
// on every prompt, so that command prints the notice once per shell session, tracked by a marker file
// named after WINDSOR_SESSION_TOKEN in .windsor. It ignores every error, so a notice never fails a command.
func printMigrationNotice(cmd *cobra.Command) {
	if noticeSkipCommands[cmd.Name()] {
		return
	}
	var projectRoot string
	if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
		rt := overridesVal.(*runtime.Runtime)
		if rt.Shell == nil {
			return
		}
		root, err := rt.Shell.GetProjectRoot()
		if err != nil {
			return
		}
		projectRoot = root
	} else {
		root, err := shell.NewDefaultShell().GetProjectRoot()
		if err != nil {
			return
		}
		projectRoot = root
	}

	files := migrate.NewMigrator(nil).LegacyFiles(projectRoot)
	if len(files) == 0 {
		return
	}

	if hookFlag := cmd.Flags().Lookup("hook"); cmd.Name() == "env" && hookFlag != nil && hookFlag.Value.String() == "true" {
		if token := os.Getenv("WINDSOR_SESSION_TOKEN"); sessionTokenPattern.MatchString(token) {
			marker := filepath.Join(projectRoot, ".windsor", ".migrate-notice."+token)
			if _, err := os.Stat(marker); err == nil { // #nosec G703 -- the token matches [A-Za-z0-9]+, so it cannot leave .windsor
				return
			}
			if err := os.MkdirAll(filepath.Dir(marker), 0750); err == nil { // #nosec G703 -- same token check
				_ = os.WriteFile(marker, nil, 0600) // #nosec G703 -- same token check
			}
		}
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Notice: this project has v1alpha1 files. Run 'windsor migrate' to convert them.\n  %s\n",
		strings.Join(files, "\n  "))
}

// init registers the migrate command and its --dry-run flag.
func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Print the planned changes and the conflicts. Write nothing.")
	rootCmd.AddCommand(migrateCmd)
}
