---
title: "Local Workstation"
description: "The Windsor CLI configures your local workstation to closely mimic production environments."
---
# Local Workstation
A significant feature of the Windsor CLI is its ability to configure your local workstation. Every attempt is made to configure this workstation to closely mimic true production workloads. While working with your local workstation, you can expect to have DNS, Docker registries, Kubernetes clusters, an AWS emulator (Localstack), and a local reflection of your repository available via a local git server.

## Prerequisites
To fully follow this guide, you should already have a local Windsor environment running, having followed through the [quick start tutorial](../quick-start.md). With the Windsor CLI installed, you can create your local environment by running:

```
windsor init local
windsor up
```

More information about the local workstation follows.

## Virtualization
In order to simulate a disposable local environment across multiple OS's, a virtual machine is required. The Windsor project will continue to add support for common virtualization drivers.

### Colima
Presently, MacOS and Linux users may use [Colima](https://github.com/abiosoft/colima), which is itself a wrapper around [Lima](https://github.com/lima-vm/lima). This provides a friendly interface to native virtualization technologies such as Apple's hypervisor framework. With a full virtualization, you will be able to closely emulate a full production environment.

### Docker Desktop
Developers on Linux, MacOS, or Windows may use [Docker Desktop](https://docs.docker.com/desktop/). Not all services or features are supported on a lightweight containerized virtualization platform.

### Feature Comparison

| Feature                          | Full Virtualization (e.g., Colima) | Lightweight Container Virtualization (e.g., Docker Desktop) |
|----------------------------------|------------------------------------|-------------------------------------------------------------|
| **DNS**                    | DNS routes to service IPs    | DNS routes to localhost                              |
| **Docker Registries**                  | Full support      | Full support              |
| **Local Git**                  | Full support      | Full support              |
| **Kubernetes Cluster**                  | Supported as containers & VMs (soon)      | Supported as containers              |
| **Device Emulation**                   | Filesystem and block devices         | Filesystem only                          |
| **Network Emulation**                   | Fully addressable IP range and Layer2 load balancing | Localhost with port-forwarding and nodeport     |

## AWS
If you use Amazon Web Services (AWS) as your cloud provider, Windsor can set up an instance of [Localstack](https://github.com/localstack/localstack), allowing emulation of many AWS services. To enable this, modify your `windsor.yaml` to include:

```
aws:
  enabled: true
  localstack:
    enabled: true
```

This will enable Localstack, which will launch on running `windsor up`. The AWS API endpoint may then be reached at http://aws.test:4566.

## DNS
The Windsor CLI configures your local DNS resolver to route requests to services running on the local cloud. The CLI configures the resolver for a reserved local domain (defaults to `test`) at a local CoreDNS container. This setup allows routing to development services (http://aws.test, http://git.test, etc) as well as services running in the local cluster.

The `.test` domain is reserved by the Internet Assigned Numbers Authority (IANA) for testing purposes. It is ideal for a local development or CI/CD pipeline. If you would like to change it, you may do so in the `windsor.yaml` file:

```
dns:
  enabled: true
  name: local # used to be "test"
```

### Testing DNS Configuration

To verify your DNS setup, follow the instructions for your operating system:

=== "Windows"
    1. Open Command Prompt.
    2. Run the following command:
       ```powershell
       nslookup registry.test dns.test
       ```
    3. You should see an output similar to:
       ```plaintext
       Name:    registry.test
       Address: 127.0.0.1
       ```

=== "Linux"
    1. Open Terminal.
    2. Run the following command:
       ```bash
       dig @dns.test registry.test
       ```
    3. If running natively on Linux or with a full virtualization, expect:
       ```plaintext
       ;; ANSWER SECTION:
       registry.test.          3600    IN      A       10.5.0.3
       ```

=== "MacOS"
    1. Open Terminal.
    2. Run the following command:
       ```bash
       dig @dns.test registry.test
       ```
    3. You should see an output similar to:
       ```plaintext
       ;; ANSWER SECTION:
       registry.test.          3600    IN      A       127.0.0.1
       ```

Note: The IP address `127.0.0.1` is expected when using Docker Desktop, while `10.5.0.3` is expected for full virtualization setups.

## Registries
The local workstation provides several Docker registries run as local containerized services. The following table outlines these and their purpose:

| Registry               | Local Endpoint                     | Purpose                                      |
|------------------------|------------------------------------|----------------------------------------------|
| Google Container Registry (gcr.io) | http://gcr.test:5000        | Mirror for Google Container Registry         |
| GitHub Container Registry (ghcr.io) | http://ghcr.test:5000       | Mirror for GitHub Container Registry         |
| Quay Container Registry (quay.io) | http://quay.test:5000       | Mirror for Quay Container Registry           |
| Docker Hub (registry-1.docker.io) | http://registry-1.docker.test:5000 | Mirror for Docker Hub                        |
| Kubernetes Official Registry (registry.k8s.io) | http://registry.k8s.test:5000 | Mirror for Kubernetes official registry      |
| Local Generic Registry (registry.test) | http://registry.test:5000 | Generic local registry                       |

To configure a mirror, add the following configuration:

```
docker:
  registries:
    1234567890.dkr.ecr.us-east-1.amazonaws.com:
      remote: https://1234567890.dkr.ecr.us-east-1.amazonaws.com
```

The registry base URL is always available via the `REGISTRY_URL` environment variable.

**Note:** When running Docker Desktop, only http://registry.test:5002 is available.

These registries store their cache in the `.windsor/.docker-cache` folder. This cache is persisted, allowing faster load times as well as acting as a genuine registry mirror. The local registry is also accessible from the local Kubernetes cluster, allowing you to easily load locally developed images in to your Kubernetes environment.

### Pushing to your local registry
You can push images to your local registry. Assuming you have a Dockerfile in place,

1. Build your Docker image:
```bash
docker build -t my-image:latest .
```
2. Tag the image for the local registry:
```bash
docker tag my-image:latest ${REGISTRY_URL}/my-image:latest
```
3. Push the image to the local registry:
```bash
docker push ${REGISTRY_URL}/my-image:latest
```

## Build ID Management

Windsor provides a `build-id` command for managing build identifiers used for artifact tagging in local development environments. Build IDs are stored persistently in the `.windsor/.build-id` file and are available as the `BUILD_ID` environment variable and postBuild variable in Kustomizations.

### Usage

```bash
windsor build-id                    # Output current build ID
windsor build-id --new              # Generate and output new build ID
```

### Build ID Format

Build IDs follow the format `YYMMDD.RANDOM.#` where:
- `YYMMDD` is the current date (year, month, day)
- `RANDOM` is a random three-digit number for collision prevention
- `#` is a sequential counter incremented for each build on the same day

### Examples

```bash
# Get the current build ID
windsor build-id

# Generate a new build ID and use it for Docker tagging
BUILD_ID=$(windsor build-id --new) && docker build -t myapp:$BUILD_ID .

# Use build ID with local registry
BUILD_ID=$(windsor build-id --new)
docker build -t ${REGISTRY_URL}/myapp:$BUILD_ID .
docker push ${REGISTRY_URL}/myapp:$BUILD_ID
```

The `BUILD_ID` is automatically included in:
- Environment variables available to all processes
- Post-build variables in Kustomizations for Flux variable substitution
- Cluster variables accessible in Kubernetes manifests

## Local GitOps
A local GitOps workflow is provided by [git-livereload](https://github.com/windsorcli/git-livereload). When you save your files locally, they are updated within this container, and reflected as a new commit via [http://git.test](http://git.test). This feature is utilized by the internal gitops tooling ([Flux](https://github.com/fluxcd/flux2)), allowing you to persistently reflect your local Kubernetes manifests on to your local cluster.

Flux supports a [webhook](https://fluxcd.io/flux/guides/webhook-receivers/) that allows for fast cluster state reconciliation. This mechanism is configured for you in your local environment. It can be triggered via a worker NodePort as follows:
 
 ```bash
 curl -X POST http://worker-1.test:30292/hook/5dc88e45e809fb0872b749c0969067e2c1fd142e17aed07573fad20553cc0c59
 ```

 This is triggered on each refresh by the git-livereload service.

Read more about configuring git livereload in the [reference](../reference/configuration.md#git) section.

### Testing the local git repository
For those running on a full virtualization option (Colima), the git server is accessible. You can test the local git repository as follows. It is assumed you ran `windsor up` in a folder called `my-project`. In a new folder, run:

```
git clone http://local@git.test/git/my-project
```

This should pull down the contents of your repository in to a new folder.

## Kubernetes Cluster
A container based Kubernetes cluster is run locally. Currently, Windsor supports clusters running [Sidero Talos](https://github.com/siderolabs/talos).

You can configure the cluster's controlplanes and workers in the windsor.yaml as follows:

```yaml
cluster:
  enabled: true
  driver: talos
  controlplanes:
    count: 1
    cpu: 2
    memory: 2
  workers:
    count: 1
    cpu: 4
    memory: 4
    hostports:
    - 80:30080/tcp
    - 443:30443/tcp
    - 9292:30292/tcp
    - 8053:30053/udp
    volumes:
    - ${WINDSOR_PROJECT_ROOT}/.volumes:/var/local
```

### Listing cluster nodes
When Windsor bootstraps the local cluster, it places a kube config file at `contexts/local/.kube/config`. It also configures the `KUBECONFIG` path environment variable for you. To test that everything is configured as expected, list the Kubernetes nodes by running:

```
kubectl get nodes
```

You should see something like:

```
NAME             STATUS   ROLES           AGE     VERSION
controlplane-1   Ready    control-plane   1m      v1.31.4
worker-1         Ready    <none>          1m      v1.31.4
```

### Validating web ports
By default, your initial environment comes installed with Istio's [BookInfo application](https://istio.io/latest/docs/examples/bookinfo/). You can check on the status of this deployment by running:

```
kubectl get all -n demo-bookinfo
```

In a web browser, visit both http://bookinfo.test:8080 and https://bookinfo.test:8443. If you're using a full virtualization option or have configured your `hostports` to use traditional web port mappings, you do not need the port designations and can connect over `:80` and `:443`.

### Validating volumes
Across all environments, you should see a set of common storage classes. These should include a `local` and a `single` storage class. You can see these by running:

```
kubectl get storageclass
``` 
In your local development environment, these are both provided by [OpenEBS's dynamic-localpv-provisioner](https://github.com/openebs/dynamic-localpv-provisioner). To further validate, you should run through the [Hello World](../tutorial/hello-world.md) example and verify that you can see `.volumes/pvc-*`  folders mounted in to your project folder.

<div>
  {{ footer('Contexts', '../contexts/index.html', 'Environment Injection', '../environment-injection/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../contexts/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../environment-injection/index.html'; 
  });
</script>
