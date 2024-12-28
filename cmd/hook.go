package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out shell hook information per platform.",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh, elvish,powershell).",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) == 0 {
			return fmt.Errorf("No shell name provided")
		}

		return shell.InstallHook(args[0])

		// // // Retrieve the hook command for the specified shell
		// // hookCommand, exists := shellHooks[shellName]
		// // if !exists {
		// // 	return fmt.Errorf("Unsupported shell: %s", shellName)
		// // }

		// // selfPath, err := os.Executable()
		// // if err != nil {
		// // 	return err
		// // }

		// // // Convert Windows path if needed
		// // selfPath = strings.Replace(selfPath, "\\", "/", -1)
		// // ctx := HookContext{selfPath}

		// // hookTemplate, err := template.New("hook").Parse(hookCommand[0])
		// // if err != nil {
		// // 	return err
		// // }

		// // err = hookTemplate.Execute(os.Stdout, ctx)
		// // if err != nil {
		// // 	return err
		// // }

		// return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
