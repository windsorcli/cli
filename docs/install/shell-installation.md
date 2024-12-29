# Shell Installation

## DESCRIPTION

`windsor` is an environment variable manager for your shell. It knows how to
hook into bash, zsh and fish shell to load or unload environment variables
depending on your current directory. 

Because windsor is compiled into a single static executable it is fast enough
to be unnoticeable on each prompt. It is also language agnostic and can be
used to build solutions similar to rbenv, pyenv, phpenv, ...


## SETUP

For windsor to work properly it needs to be hooked into the shell. Each shell has its own extension mechanism:

### BASH

Add the following line at the end of the `~/.bashrc` file:

```sh
eval "$(windsor hook bash)"
```

Make sure it appears even after rvm, git-prompt and other shell extensions
that manipulate the prompt.

### ZSH

Add the following line at the end of the `~/.zshrc` file:

```sh
eval "$(windsor hook zsh)"
```

### FISH

Add the following line at the end of the `$XDG_CONFIG_HOME/fish/config.fish` file:

```fish
windsor hook fish | source
```

Fish supports 3 modes you can set with with the global environment variable `windsor_fish_mode`:

```fish
set -g windsor_fish_mode eval_on_arrow    # trigger windsor at prompt, and on every arrow-based directory change (default)
set -g windsor_fish_mode eval_after_arrow # trigger windsor at prompt, and only after arrow-based directory changes before executing command
set -g windsor_fish_mode disable_arrow    # trigger windsor at prompt only, this is similar functionality to the original behavior
```


### TCSH

Add the following line at the end of the `~/.cshrc` file:

```sh
eval `windsor hook tcsh`
```

### Elvish

Run:

```
~> mkdir -p ~/.config/elvish/lib
~> windsor hook elvish > ~/.config/elvish/lib/windsor.elv
```

and add the following line to your `~/.config/elvish/rc.elv` file:

```
use windsor
```

### PowerShell

Add the following line to your `$PROFILE`:

```powershell
Invoke-Expression "$(windsor hook pwsh)"
```
