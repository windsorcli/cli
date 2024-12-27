package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shellHooks = map[string][]string{
	"zsh": {
		`
		_direnv_hook() {
			trap -- '' SIGINT
			eval "$("{{.SelfPath}}" export zsh)"
			trap - SIGINT
		}
		typeset -ag precmd_functions
		if (( ! ${precmd_functions[(I)_direnv_hook]} )); then
			precmd_functions=(_direnv_hook $precmd_functions)
		fi
		typeset -ag chpwd_functions
		if (( ! ${chpwd_functions[(I)_direnv_hook]} )); then
			chpwd_functions=(_direnv_hook $chpwd_functions)
		fi
		`,
	},
	"bash": {
		`
		_direnv_hook() {
			local previous_exit_status=$?;
			trap -- '' SIGINT;
			eval "$("{{.SelfPath}}" export bash)";
			trap - SIGINT;
			return $previous_exit_status;
		};
		if [[ ";${PROMPT_COMMAND[*]:-};" != *";_direnv_hook;"* ]]; then
			if [[ "$(declare -p PROMPT_COMMAND 2>&1)" == "declare -a"* ]]; then
				PROMPT_COMMAND=(_direnv_hook "${PROMPT_COMMAND[@]}")
			else
				PROMPT_COMMAND="_direnv_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
			fi
		fi
		`,
	},
	"fish": {
		`
		function _direnv_hook --on-event fish_prompt
			eval ("{{.SelfPath}}" export fish | source)
		end
		`,
	},
	"tcsh": {
		`
		alias precmd '_direnv_hook'
		alias _direnv_hook 'eval "{{.SelfPath}}" export tcsh"
		`,
	},
	"elvish": {
		`
		eval ("{{.SelfPath}}" export elvish | slurp)
		`,
	},
	"powershell": {
		`
		$DirenvPrompt = {
			Invoke-Expression (& "{{.SelfPath}}" export powershell)
		}
		If ($global:Prompt -is [scriptblock]) {
			$oldPrompt = $global:Prompt
			$global:Prompt = {
				& $DirenvPrompt
				& $oldPrompt
			}
		} Else {
			$global:Prompt = $DirenvPrompt
		}
		`,
	},
}

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out windsor setup command for each platform.",
	Long:         "Prints out windsor setup command for each platform.",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {

		// Check if the correct number of arguments is provided
		if len(args) != 1 {
			return fmt.Errorf("Please provide exactly one argument specifying the shell name")
		}

		// Get the shell name from the arguments
		shellName := args[0]

		// Retrieve the hook command for the specified shell
		hookCommand, exists := shellHooks[shellName]
		if !exists {
			return fmt.Errorf("Unsupported shell: %s", shellName)
		}

		// Print the hook command
		fmt.Println(hookCommand[0])
		return nil

	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
