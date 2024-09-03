package windsor

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Windsor CLI",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Windsor CLI v0.1.0")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
