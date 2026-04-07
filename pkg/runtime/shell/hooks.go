package shell

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
