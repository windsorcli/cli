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

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display the current version",
	Long:  "Display the current version of the application",
	Run: func(cmd *cobra.Command, args []string) {
		platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Version: %s\nCommit SHA: %s\nPlatform: %s\n", version, commitSHA, platform)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
