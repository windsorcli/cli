package cmd

import (
	"fmt"
	"runtime"

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
		cmd.Printf("Version: %s\nCommit SHA: %s\nPlatform: %s\n", constants.Version, constants.CommitSHA, platform)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
