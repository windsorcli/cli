# Quick Start

### Install Dependencies

```bash
aqua install
```

### Set the context to local

```sh
windsor init --vm-driver colima local
windsor context set local
```
This command initializes the application by setting up necessary configurations and the contextual environment.

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
