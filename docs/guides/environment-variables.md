# Kubernetes and Talos Environment Variables
```bash
export TALOSCONFIG="*****/contexts/local/.talos/config"
export KUBECONFIG="*****/contexts/local/.kube/config"
export KUBE_CONFIG_PATH="*****/contexts/local/.kube/config"
```

# AWS Environment Variables
```bash
export AWS_CONFIG_FILE="*****/contexts/local/.aws/config"
export AWS_ENDPOINT_URL="http://aws.test:4566"
export AWS_PROFILE="local"
```

# Terraform Environment Variables
```bash
export TF_CLI_ARGS_apply="*****/contexts/local/.terraform/cluster/talos/terraform.tfplan"
export TF_CLI_ARGS_destroy="-var-file=*****/contexts/local/terraform/cluster/talos.tfvars \
  -var-file=*****/contexts/local/terraform/cluster/talos_generated.tfvars.json"
export TF_CLI_ARGS_import="-var-file=*****/contexts/local/terraform/cluster/talos.tfvars \
  -var-file=*****/contexts/local/terraform/cluster/talos_generated.tfvars.json"
export TF_CLI_ARGS_init="-backend=true -backend-config=path=*****/contexts/local/.tfstate/cluster/talos/terraform.tfstate"
export TF_CLI_ARGS_plan="-out=*****/contexts/local/.terraform/cluster/talos/terraform.tfplan \
  -var-file=*****/contexts/local/terraform/cluster/talos.tfvars \
  -var-file=*****/contexts/local/terraform/cluster/talos_generated.tfvars.json"
export TF_DATA_DIR="*****/contexts/local/.terraform/cluster/talos"
export TF_VAR_context_path="*****/contexts/local"
```
