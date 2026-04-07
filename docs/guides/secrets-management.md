---
title: "Secrets Management"
description: "Unified secrets management using the Windsor CLI with support for 1Password and other providers."
---

# Secrets Management
The Windsor CLI provides tools for managing secrets within your projects. This guide covers the setup and usage of the secrets management functionality, including integration with [SOPS](https://github.com/getsops/sops) and 1Password.

## Overview
The secrets management system in Windsor CLI is designed to securely handle sensitive information such as API keys, passwords, and other confidential data. It supports multiple backends, allowing you to choose the most suitable provider for your needs.

Windsor supports multiple secrets providers simultaneously. Once a provider is configured, you may inject secrets in to your environment by setting these values on your context's `environment` key in `windsor.yaml`:

```yaml
version: v1alpha1
contexts:
  local:
    environment:
      CRITERION_PASSWORD: {% raw %}${{ op.personal["The Criterion Channel"]["password"] }}{% endraw %}
...
```

Where `op` represents the `onepassword` secrets provider, and `personal` is a reference to the 1Password vault where you have stored your secrets.

## Supported Providers

### SOPS
The Windsor CLI integrates with [SOPS (Secrets OPerationS)](https://github.com/getsops/sops). SOPS is a tool that encrypts and decrypts secrets to a file, allowing you to commit sensitive values to a repository securely. If you would like to use SOPS in your project, it is expected that you visit their documentation and configure it correctly with an appropriate `sops.yaml` file.

To use SOPS with Windsor, run `sops contexts/<context-name>/secrets.enc.yaml` to encrypt your secrets file. As long as this file exists and is a valid SOPS-encrypted file, these values are available to use in your `environment` configuration as follows:

```yaml
version: v1alpha1
contexts:
  local:
    environment:
      CRITERION_PASSWORD: {% raw %}${{ sops.streaming.criterion.password }}{% endraw %}
...
```

### 1Password CLI
The Windsor CLI integrates with the [1Password CLI](https://developer.1password.com/docs/cli/). It can import secrets from multiple accounts and vaults. To configure a 1Password vault, add the following to your `windsor.yaml`:

```yaml
...
version: v1alpha1
contexts:
  local:
    ...
    secrets:
      onepassword:
        vaults:
          personal:
            url: my.1password.com
            vault: "Personal"
          development:
            url: my-company.1password.com
            vault: "Development"
...
```

With this configuration in place, you can reference these secrets in your environment configuration as follows:

```yaml
version: v1alpha1
contexts:
  local:
    environment:
      LOCALSTACK_API_KEY: {% raw %}${{ op.personal.localstack.api_key }}{% endraw %}
      STRIPE_API_KEY: {% raw %}${{ op.development.stripe.api_key }}{% endraw %}
```

When you have configured 1Password in your environment, you will likely be prompted to authenticate with 1Password. This creates a session, and you will not be prompted again until that session has been expired, lasting typically 30 minutes.

## Caching
Secrets from remote providers are cached in-memory in your environment to improve performance and reduce unnecessary service calls or re-authentication. If a secret has already been defined in your environment, it will not be retrieved again. If you would like to trigger a refresh of your secrets, you may either:

=== "Bash"
    1. Start a new shell session, or
    2. Set the `NO_CACHE` environment variable to `true`:
    ```bash
    NO_CACHE=true windsor init
    ```

=== "PowerShell"
    1. Start a new PowerShell session, or
    2. Set the `NO_CACHE` environment variable to `true`:
    ```powershell
    $env:NO_CACHE = "true"
    windsor init
    ```

## Troubleshooting
If you are having difficulty with your secrets, you may export your secret to the terminal and inspect it. If there has been an error, it will be included as the value of your environment variable, `e.g.`:

=== "Bash"
    ```bash
    $ env | grep '<ERROR'
    MY_SECRET=<ERROR: secret not found>
    ```

=== "PowerShell"
    ```powershell
    PS> Get-ChildItem Env: | Where-Object { $_.Value -like '*<ERROR*' }
    MY_SECRET=<ERROR: secret not found>
    ```

## Security Recommendations
For more details about Windsor's use of secrets along with our recommendations for securely using secrets in your environment, see the section on [Securing Secrets](../security/secrets.md).

<!-- Footer Start -->

<div>
  {{ footer('Terraform', '../terraform/index.html', 'Blueprint Templates', '../templates/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../terraform/index.html';
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../templates/index.html';
  });
</script>

<!-- Footer End -->
