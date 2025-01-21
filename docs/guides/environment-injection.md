# Environment Injection
A key feature of the Windsor CLI is its ability to contextually manage your environment while working within a Windsor project. You may have encountered similar tools, in particular [direnv](https://github.com/direnv/direnv), that make it possible to dynamically configure environment variables while you're inside a project folder. The `windsor hook` & `windsor env` system does the same, while continuing to manage your environment variables as you move throughout the subfolders within your project.

## About the Mechanism
You should have set up the hook during [installation](../install.md). If configured correctly, the hook causes `windsor env` to inject into your environment prior to loading each prompt. The `windsor env` command will only be activated while you are in a [trusted Windsor project](../security/trusted-folders.md). By managing your environment variables dynamically, the Windsor CLI is able to manage the behavior and context configuration of the tools in your toolchain as you switch contexts or move amongst your project folders.

## Supported Tools
Several tools are presently supported by Windsor's environment management system. The following outlines what to expect for each of these tools.

### AWS CLI
If you are using Amazon Web Services (AWS), it is assumed that your AWS config file is located in your project at `contexts/<context-name>/.aws/config`, and leverage non-sensitive, OIDC-based authentication. It's only necessary to configure a single `default` profile.

The following environment variables are set automatically, or can be configured in your `windsor.yaml` file:

| Variable         | Default Value                          | Configuration                                      |
|------------------|----------------------------------------|----------------------------------------------------|
| AWS_CONFIG_FILE  | `contexts/<context-name>/.aws/config`  |                                                    |
| AWS_PROFILE      | system default                         | `contexts.<context-name>.aws.aws_profile`          |
| AWS_ENDPOINT_URL | system default                         | `contexts.<context-name>.aws.aws_endpoint_url`     |
| S3_HOSTNAME      | system default                         | `contexts.<context-name>.aws.s3_hostname`          |
| MWAA_ENDPOINT    | system default                         | `contexts.<context-name>.aws.mwaa_endpoint`        |

### Docker
The Windsor CLI provides several functionalities to manage Docker environments effectively. It automatically sets the `DOCKER_HOST` environment variable based on the `vm.driver` configuration, ensuring compatibility with both Colima and Docker Desktop setups. The CLI also ensures the Docker configuration directory exists and writes necessary configuration files. Additionally, it adds the `DOCKER_CONFIG` environment variable pointing to the Docker configuration directory and manages aliases for Docker commands, such as `docker-compose`, if specific plugins are detected.

Below is a table summarizing the driver configurations:

| Driver                  | DOCKER_HOST Path                                                      |
|-------------------------|-----------------------------------------------------------------------|
| Colima                  | `unix://<home-directory>/.colima/windsor-<context-name>/docker.sock`  |
| Docker Desktop | `unix://<home-directory>/.docker/run/docker.sock`                              |
| Docker Desktop (Windows) | `npipe:////./pipe/docker_engine`                                     |

These features ensure that your Docker environment is configured correctly and consistently, regardless of the underlying virtualization driver you are using.

### Kubernetes
The following Kubernetes related environment variables are set to the following paths:

| Variable         | Configuration Path                                        |
|------------------|-----------------------------------------------------------|
| KUBECONFIG       | `contexts/<context-name>/.kube/config`                    |
| KUBE_CONFIG_PATH | `contexts/<context-name>/.kube/config`                    |
| TALOSCONFIG      | `contexts/<context-name>/.talos/config`                   |

### Terraform
Windsor configures your `TF_CLI_ARGS_*` variables when you change in to a project under the `terraform/` folder. You can read more in depth about how Windsor works with Terraform in the [Terraform guide](terraform.md).


<div>
  {{ footer('Contexts', '../../guides/contexts/index.html', 'Local Workstation', '../../guides/local-workstation/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../guides/contexts/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../guides/local-workstation/index.html'; 
  });
</script>
