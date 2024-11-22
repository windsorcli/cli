<!-- Add this to the top of your Markdown file -->
<!-- <div class="draft-watermark"></div> -->


Ensure you have **[Go](https://golang.org/doc/install)** and **[Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)** installed on your system

| Installation Method          | Description         |
|------------------------------|---------------------|
| [Source Code Installation](./install/source-code-installation.md) | Clone and build        |
| [Go Installation](./install/go-installation.md)   | Go installation     |


## [Shell Integration](#shell-integration)

To automatically load Windsor CLI environment variables in your shell, add the following to your `precmd()` function in your shell configuration file (e.g., `.zshrc` for Zsh or `.bashrc` for Bash):

```bash
precmd() {
  if command -v windsor >/dev/null 2>&1; then
    eval "$(windsor env)"
  fi
}
```

<div>
{{ footer('Home', '../index.html', 'Quick Start', '../tutorial/quick-start/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = 'index.html'; 
  });

  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../tutorial/quick-start/index.html'; 
  });
</script>
