package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// These variables will be set at build time
var (
	version   = "dev"
	commitSHA = "none"
)

// Goos returns the operating system, can be mocked for testing
var Goos = runtime.GOOS

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display the current version",
	Long:  "Display the current version of the application",
	Run: func(cmd *cobra.Command, args []string) {
		platform := fmt.Sprintf("%s/%s", Goos, runtime.GOARCH)
		cmd.Printf("Version: %s\nCommit SHA: %s\nPlatform: %s\n", version, commitSHA, platform)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
