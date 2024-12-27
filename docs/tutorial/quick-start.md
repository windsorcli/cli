# Quick Start
After [installation](../install/install.md), you can use the \`windsor\` command to interact with the CLI. Here are some common commands:

### Install Dependencies

Upon the meticulous configuration of your `aqua.yaml` file, execute the following command to install the specified tools:

```bash
aqua install
```

### Set the context to local

```sh
windsor init --vm-driver colima local
windsor context set local
```
This command initializes the application by setting up necessary configurations and environment.

### Start Talos kubernetets

```sh
windsor up
```

<div>
{{ previous_footer('Installation', '../../install/install/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../install/install/index.html'; 
  });
</script>


