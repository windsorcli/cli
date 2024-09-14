package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var exitFunc = os.Exit

// Wrapper function for os.UserHomeDir
var userHomeDir = os.UserHomeDir

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "windsor",
	Short: "A command line interface to assist in a context flow development environment",
	Long:  "A command line interface to assist in a context flow development environment",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Bind environment variable
		viper.BindEnv("windsorconfig", "WINDSORCONFIG")

		// Optionally, use the environment variable if set
		if configPath := viper.GetString("windsorconfig"); configPath != "" {
			viper.SetConfigFile(configPath)
		} else {
			// Initialize configuration
			viper.SetConfigName("config") // name of config file (without extension)
			viper.SetConfigType("yaml")   // REQUIRED if the config file does not have the extension in the name

			// Set default config path to $HOME/.config/windsor
			home, err := userHomeDir()
			if err != nil {
				fmt.Printf("Error finding home directory, %s\n", err)
				exitFunc(1)
			}
			defaultConfigPath := filepath.Join(home, ".config", "windsor")
			viper.AddConfigPath(defaultConfigPath)
		}

		if err := viper.ReadInConfig(); err != nil {
			fmt.Printf("Error reading config file, %s\n", err)
			exitFunc(1)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		exitFunc(1)
	}
}
