---
title: "Securing Secrets"
description: "Best practices and special features for managing secrets securely with Windsor CLI."
---

# Securing Secrets
The Windsor CLI offers features and best practices to ensure the secure management of secrets within your projects. This section highlights these features and provides recommendations for securely handling secrets. Read more about [secrets management](../guides/secrets-management.md) in the corresponding guide.

## Risks and Mitigations

### Secret Exposure Through the Environment
If you have configured a secret to be injected into your environment, this represents a potential vector for sensitive data exposure. It is recommended to only inject development secrets and avoid using this mechanism in your production environments. However, this mechanism may be valuable during production bootstrapping. Rotating your secrets once an appropriate production-grade secrets mechanism is in place is recommended.

Windsor supports the `windsor env --decrypt` option, allowing you to decrypt secrets only when necessary. This ensures that secrets remain encrypted by default and are only decrypted in memory when explicitly required by your workflow. The `windsor hook` that you installed in your shell always decrypts environment variables. However, if you run `windsor env` to inspect these variables, secrets are either not included in the output if they are cached or are obfuscated with asterisks,

=== "Bash"
    ```bash
    $ windsor env | grep MY_SECRET
    MY_SECRET=********
    ```

=== "PowerShell"
    ```powershell
    PS> Get-ChildItem Env: | Where-Object { $_.Name -eq "MY_SECRET" }
    MY_SECRET=********
    ```

## Best Practices

### Limit Environment Injection
Injecting secrets directly into your environment is generally discouraged outside of development environments. This practice can lead to unintentional exposure of sensitive information.

### Regularly Rotate Secrets
Regularly rotating your secrets is a critical practice for maintaining security. Using a service such as 1Password makes it simple to rotate secrets centrally.

### Avoid Extended Shell Sessions
To minimize the risk of secret exposure, limit your shell sessions to specific tasks related to your project. Once you have completed your tasks, promptly close the shell session to reduce the chance of sensitive data being compromised. Dispose of shell sessions when they are no longer needed to maintain security.

<div>
  {{ footer('Contexts', '../../reference/contexts/index.html', 'Secrets', '../trusted-folders/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../reference/contexts/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../trusted-folders/index.html'; 
  });
</script>
