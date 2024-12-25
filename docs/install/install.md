# Install Aqua
https://aquaproj.github.io/docs/install

# Install Pipx
https://pipx.pypa.io/stable/installation/

# Install Poetry
https://python-poetry.org/docs/#installing-with-pipx

poetry env info --path


# Setup and Installation

## [1. Confirm all prerequisites have been met](#prerequisites)
Ensure you have **[Go](https://golang.org/doc/install)** and **[Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)** installed on your system

## [2. Install the application ](#installation) 

### Go Installation Method
Follow these steps to install Windsor CLI using Go:

#### Step 1: Go install
```bash
go install github.com/windsorcli/cli/cmd/windsor@latest
```

### Source Code Installation Method
Follow these steps to install Windsor CLI from the source code:

#### Step 1: Clone the Repository
```bash
git clone https://github.com/windsorcli/cli.git
```

#### Step 2: Build the Application

```bash
cd cli;mkdir -p dist;go build -o dist/windsor cmd/windsor/main.go;cd ..
```

#### Step 3: Put application in system PATH

```bash
cp cli/dist/windsor /usr/local/bin/windsor
```

## [3. Setup Shell Integration](#shell-integration)

Add this `precmd()` definition to your shell configuration file (e.g., `.zshrc` for Zsh or `.bashrc` for Bash).

This is required for the Windsor CLI to load environment variables automatically in the shell:

```bash
precmd() {
  if command -v windsor >/dev/null 2>&1; then
    eval "$(windsor env)"
  fi
}
```

## [4. Test windsor command](#test-windsor)

Check the version using this command,

```bash
windsor version
```

Dump the windsor environment variables,

```bash
windsor env
```

<div>
{{ footer('Home', '../../index.html', 'Quick Start', '../../tutorial/quick-start/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/quick-start/index.html'; 
  });
</script>
