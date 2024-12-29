# Quick Start

### Install Dependencies

```bash
aqua install
```

### Initialize the environment

Initialize the contextual environment setting up necessary configurations and naming the context.

```sh
windsor init local
```

### Confirm context

Confirm the default 'local' context was selected.

```sh
windsor context get
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
