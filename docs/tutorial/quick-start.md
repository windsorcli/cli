# Quick Start

### Install Dependencies

```bash
aqua install
```

### Set the context to local

Initialize the contextual environment setting up necessary configurations and naming the context.

```sh
windsor init --vm-driver colima local
windsor context set local
```

### Start talos kubernetes cluster

Build the virtual environment and start the talos operating system.

```sh
windsor up
```

### List kubernetes nodes

```sh
kubectl get nodes
```

<div>
{{ previous_footer('Installation', '../../install/install/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../install/install/index.html'; 
  });
</script>
