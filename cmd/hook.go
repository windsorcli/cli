package cmd

import (
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// HookContext are the variables available during hook template evaluation
type HookContext struct {
	// SelfPath is the unescaped absolute path to windsor
	SelfPath string
}

// /usr/local/bin/windsor env

var shellHooks = map[string][]string{
	"zsh": {
		`
		_windsor_hook() {
			trap -- '' SIGINT;
 			eval "$("{{.SelfPath}"/windsor env export zsh)"		
			trap - SIGINT;
		};
		typeset -ag precmd_functions;
		if (( ! ${precmd_functions[(I)_windsor_hook]} )); then
			precmd_functions=(_windsor_hook $precmd_functions)
		fi;
		typeset -ag chpwd_functions;
		if (( ! ${chpwd_functions[(I)_windsor_hook]} )); then
			chpwd_functions=(_windsor_hook $chpwd_functions)
		fi;
		`,
	},
	"bash": {
		`
		_windsor_hook() {
			local previous_exit_status=$?;
			trap -- '' SIGINT;
			eval "$(windsor env)";
			trap - SIGINT;
			return $previous_exit_status;
		};
		if [[ ";${PROMPT_COMMAND[*]:-};" != *";_windsor_hook;"* ]]; then
			if [[ "$(declare -p PROMPT_COMMAND 2>&1)" == "declare -a"* ]]; then
				PROMPT_COMMAND=(_windsor_hook "${PROMPT_COMMAND[@]}")
			else
				PROMPT_COMMAND="_windsor_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
			fi
		fi
		`,
	},
	"fish": {
		`
		function _windsor_hook --on-event fish_prompt
			eval (eval "$(windsor env)"; export fish | source)
		end
		`,
	},
	"tcsh": {
		`
		alias precmd '_windsor_hook';
		alias _windsor_hook 'eval eval "$(windsor env)"; export tcsh"
		`,
	},
	"elvish": {
		`
		eval (eval "$(windsor env)"; export elvish | slurp)
		`,
	},
	"powershell": {
		`
		$DirenvPrompt = {
			Invoke-Expression (& eval "$(windsor env)"; export powershell)
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
	Short:        "Prints out shell hook information per platform.",
	Long:         "Prints out shell hook information for each platform (zsh,bash,fish,tcsh, elvish,powershell).",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {

		// Get the shell name from the arguments
		shellName := args[0]

		// Retrieve the hook command for the specified shell
		hookCommand, exists := shellHooks[shellName]
		if !exists {
			return fmt.Errorf("Unsupported shell: %s", shellName)
		}

		selfPath, err := os.Executable()
		if err != nil {
			return err
		}

		// Convert Windows path if needed
		selfPath = strings.Replace(selfPath, "\\", "/", -1)
		ctx := HookContext{selfPath}

		hookTemplate, err := template.New("hook").Parse(hookCommand[0])
		if err != nil {
			return err
		}

		err = hookTemplate.Execute(os.Stdout, ctx)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
