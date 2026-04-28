---
title: "Windsor CLI"
description: "Compose blueprints, run terraform, deploy via Flux, and manage local workstation environments."
---
<div align="center">
  <h1>Windsor Command Line Interface</h1>

  <p>
    <img src="img/windsor-logo.png" alt="Windsor CLI Logo" style="width: 50%; height: auto;">
  </p>

  <p>
    <img src="https://img.shields.io/github/v/release/windsorcli/cli" alt="GitHub release">
    <img src="https://img.shields.io/github/actions/workflow/status/windsorcli/cli/ci.yaml" alt="CI status">
  </p>

  <hr>
</div>

## What it is

Windsor is a CLI for cloud-native development workflows. It runs on Linux, macOS, and Windows, written in Go.

A Windsor project is a directory of contexts. Each context describes one deployment target — `local`, `staging`, `prod`, anything you want — and pairs a blueprint (terraform components + Flux kustomizations) with the configuration values that drive it. Windsor composes the blueprint, runs the right tools in the right order, and gets out of the way.

## The core loop

| Phase | Command |
|-------|---------|
| Scaffold a context | [`windsor init`](reference/commands/init.md) |
| Bring up a workstation | [`windsor up`](reference/commands/up.md) |
| Apply infrastructure | [`windsor apply`](reference/commands/apply.md) |
| Inspect | [`windsor plan`](reference/commands/plan.md) / [`show`](reference/commands/show.md) / [`explain`](reference/commands/explain.md) |
| Tear down | [`windsor destroy`](reference/commands/destroy.md) / [`down`](reference/commands/down.md) |

See the [Lifecycle guide](guides/lifecycle.md) for how the phases fit together.

## Get started

- [Installation](install.md)
- [Quick Start](quick-start.md) — local cluster in ~10 minutes
- [Hello, World!](tutorial/hello-world.md) — deploy a real app

## Tools Windsor drives

Docker · Kubernetes · Terraform · FluxCD · Talos Linux · Colima · AWS · SOPS · 1Password · Localstack

## Contributing

Fork the repo, create a branch, open a PR. Code must follow the project style and include tests. Issues and questions are welcome on [GitHub](https://github.com/windsorcli/cli).

## License

Mozilla Public License 2.0. See [LICENSE](LICENSE).

<div>
  {{ next_footer('Installation', './install/index.html') }}
</div>

<script>
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = './install/index.html';
  });
</script>
