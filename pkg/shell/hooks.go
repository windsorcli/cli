package shell

var shellHooks = map[string]string{
	"zsh": `
		_windsor_hook() {
			trap -- '' SIGINT;
			eval "$("{{.SelfPath}}" env)";
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
			eval "$("{{.SelfPath}}" env)";
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
			eval (eval "$("{{.SelfPath}}" env)"; export fish | source)
		end
		`,
	"tcsh": `
		alias precmd '_windsor_hook';
		alias _windsor_hook 'eval eval "$("{{.SelfPath}}" env)"; export tcsh"
		`,
	"elvish": `
		eval (eval "$("{{.SelfPath}}" env)"; export elvish | slurp)
		`,
	"powershell": `
		$DirenvPrompt = {
			Invoke-Expression (& eval "$("{{.SelfPath}}" env)"; export powershell)
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
}