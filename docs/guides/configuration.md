Windsor CLI uses configuration files to manage settings. The configuration files are typically located in the following paths:

- **CLI Configuration**: `~/.config/windsor/config.yaml`
- **Project Configuration**: `./windsor.yaml` or `./windsor.yml`

You can customize these configurations to suit your needs.

### Example Configuration

Here is an example of a CLI configuration file:

```yaml
context: local
contexts:
  local:
    aws:
      aws_endpoint_url: ""
      aws_profile: local
      localstack:
        services:
        - iam
        - sts
        - kms
        - s3
        - dynamodb
      mwaa_endpoint: http://mwaa.local.aws.test:4566
      s3_hostname: http://s3.local.aws.test:4566
    cluster:
      controlplanes:
        count: 1
        cpu: 2
        memory: 2
      driver: talos
      workers:
        count: 1
        cpu: 4
        memory: 4
    docker:
      enabled: true
      registries:
      - local: ""
        name: registry.test
        remote: ""
      - local: https://docker.io
        name: registry-1.docker.test
        remote: https://registry-1.docker.io
      - local: ""
        name: registry.k8s.test
        remote: https://registry.k8s.io
      - local: ""
        name: gcr.test
        remote: https://gcr.io
      - local: ""
        name: ghcr.test
        remote: https://ghcr.io
      - local: ""
        name: quay.test
        remote: https://quay.io
    git:
      livereload:
        image: ghcr.io/windsor-hotel/git-livereload-server:v0.2.1
        password: local
        rsync_exclude: .docker-cache,.terraform,data,.venv
        rsync_protect: flux-system
        username: local
        verify_ssl: false
        webhook_url: http://flux-webhook.private.test
    terraform:
      backend: local
    vm:
      arch: aarch64
      cpu: 4
      disk: 60
      driver: colima
      memory: 8
```

<div>
{{ previous_footer('Home', '../../index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../index.html'; 
  });

</script>

