// The shell hooks are the scripts that run windsor env on each prompt or directory change.
// It provides a hook script for zsh, bash, fish, tcsh, elvish, and PowerShell.
// InstallHook renders the hook for one shell with the path of the Windsor binary.
// The user evaluates the output in the shell profile to install the hook.

package shell

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

var shellHooks = map[string]string{
	"zsh": `
		_windsor_hook() {
			trap -- '' SIGINT;
			eval "$("{{.SelfPath}}" env --decrypt --hook)";
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
	"bash": `
		_windsor_hook() {
			local previous_exit_status=$?;
			trap -- '' SIGINT;
			eval "$("{{.SelfPath}}" env --decrypt --hook)";
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
	"fish": `
		function _windsor_hook --on-event fish_prompt
			eval (eval "$("{{.SelfPath}}" env --decrypt --hook)"; export fish | source)
		end
		`,
	"tcsh": `
		alias precmd '_windsor_hook';
		alias _windsor_hook 'eval eval "$("{{.SelfPath}}" env --decrypt --hook)"; export tcsh"
		`,
	"elvish": `
		eval (eval "$("{{.SelfPath}}" env --decrypt --hook)"; export elvish | slurp)
		`,
	"powershell": `
		$originalPromptFunction = Get-Item function:\prompt -ErrorAction SilentlyContinue
		if ($originalPromptFunction) {
				$originalPromptBlock = $originalPromptFunction.ScriptBlock
		} else {
				$originalPromptBlock = $null
		}
		function prompt {
				$windsorEnvScript = & "{{.SelfPath}}" env --decrypt --hook | Out-String
				if ($windsorEnvScript) {
						Invoke-Expression $windsorEnvScript
				}
				if ($originalPromptBlock) {
						& $originalPromptBlock
				} else {
						"PS $($pwd)> "
				}
		}
		`,
}

// =============================================================================
// Types
// =============================================================================

// HookContext are the variables available during hook template evaluation
type HookContext struct {
	// SelfPath is the unescaped absolute path to the Windsor binary
	SelfPath string
}

// =============================================================================
// Public Methods
// =============================================================================

// InstallHook sets up a shell hook for a specified shell using a template with the Windsor path.
// It returns an error if the shell is unsupported. For PowerShell, it formats the script into a single line.
func (s *DefaultShell) InstallHook(shellName string) error {
	hookCommand, ok := shellHooks[shellName]
	if !ok {
		return werror.New(fmt.Sprintf("Windsor has no hook for the shell %q.", shellName),
			"Use one of these shells: bash, zsh, fish, tcsh, elvish, or powershell.", nil)
	}

	selfPath, err := s.shims.Executable()
	if err != nil {
		return err
	}

	ctx := HookContext{SelfPath: selfPath}

	hookTemplate := s.shims.NewTemplate("hook")
	if hookTemplate == nil {
		return fmt.Errorf("failed to create new template")
	}

	hookTemplate, err = s.shims.TemplateParse(hookTemplate, hookCommand)
	if err != nil {
		return werror.Wrap(err, "failed to parse hook template")
	}

	var buf bytes.Buffer
	if err := s.shims.TemplateExecute(hookTemplate, &buf, ctx); err != nil {
		return werror.Wrap(err, "failed to execute hook template")
	}

	hookScript := buf.String()

	if shellName == "powershell" {
		lines := strings.Split(hookScript, "\n")
		var cleaned []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				cleaned = append(cleaned, line)
			}
		}
		hookScript = strings.Join(cleaned, "; ")
	}

	_, err = os.Stdout.WriteString(hookScript)
	return err
}
