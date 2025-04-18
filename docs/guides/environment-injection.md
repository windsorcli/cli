---
title: "Environment Injection"
description: "Dynamically manage environment variables within a Windsor project using the Windsor CLI."
---
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


#### AWS Credentials Setup
  1. Place the aws config file under the context folder (contexts/local/.aws/config)

  Example AWS config file

  ```yaml
  [default]
  region=us-east-2
  cli_pager=

  [profile staging]
  aws_access_key_id = <AWS_ACCESS_KEY_ID>
  aws_secret_access_key = <AWS_SECRET_ACCES_KEY>
  region = us-east-1

  [profile public]
  sso_start_url = https://<ORGANIZATION_NAME>.awsapps.com/start
  sso_region = us-east-2
  sso_account_id = <SSO_ACCOUNT_ID>
  sso_role_name = AWSAdministratorAccess
  region = us-east-2
  cli_pager=
  ```
  2. Update windsor.yaml

  In this example AWS is enabled for the local context and the 'public' profile will be used

  ```yaml
  version: v1alpha1
  contexts:
    local:
      aws:
        enabled: true
        aws_profile: public
  ```

  3. Confirm AWS environment variables are present

  Run the windsor env command to confirm the AWS credentials are now part of the windsor environment
  
  ```bash
  windsor env | grep AWS

  export AWS_CONFIG_FILE="path-to/contexts/local/.aws/config"
  export AWS_PROFILE="public"
  ```

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

| Variable            | Configuration Path                          |
|---------------------|---------------------------------------------|
| KUBECONFIG          | `contexts/<context-name>/.kube/config`      |
| KUBE_CONFIG_PATH    | `contexts/<context-name>/.kube/config`      |
| TALOSCONFIG         | `contexts/<context-name>/.talos/config`     |
| PV_<NAMESPACE>_<NAME> | `.volumes/pvc-*`                          |

The `PV_<NAMESPACE>_<NAME>` environment variables point to local paths on your host that correspond to persistent volume claims in the cluster.

### Terraform
Windsor configures your `TF_CLI_ARGS_*` variables when you change in to a project under the `terraform/` folder. You can read more in depth about how Windsor works with Terraform in the [Terraform guide](terraform.md).


<div>
  {{ footer('Contexts', '../contexts/index.html', 'Kustomize', '../kustomize/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../contexts/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../kustomize/index.html'; 
  });
</script>
