package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/constants"
)

// Goos returns the operating system, can be mocked for testing
var Goos = runtime.GOOS

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:          "version",
	Short:        "Display the current version",
	Long:         "Display the current version of the application",
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		platform := fmt.Sprintf("%s/%s", Goos, runtime.GOARCH)
		cmd.Printf("Version: %s\nCommit SHA: %s\nBuild Date: %s\nGo: %s\nPlatform: %s\n",
			annotatedVersion(constants.Version),
			constants.CommitSHA,
			constants.BuildDate,
			runtime.Version(),
			platform,
		)
	},
}

// annotatedVersion appends a "(nightly build)" marker to goreleaser-snapshot
// versions so operators reading `windsor version` can tell at a glance that the
// binary is an unreleased main-branch build rather than a tagged release.
// Goreleaser emits version strings like "0.0.0-SNAPSHOT-abc1234" in --snapshot
// mode; tagged releases use clean semver and are returned unchanged.
func annotatedVersion(v string) string {
	if strings.Contains(strings.ToLower(v), "snapshot") {
		return v + " (nightly build)"
	}
	return v
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
