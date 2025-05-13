@test "windsor version" {
  run windsor version
  [ "$status" -eq 0 ]
  [[ "$output" == *"Version:"* ]]
  [[ "$output" == *"Commit SHA:"* ]]
  [[ "$output" == *"Platform:"* ]]
}

@test "windsor init local" {
  run windsor init local
  if [ "$status" -ne 0 ]; then
    echo "=== TEST FAILED: windsor init local ==="
    echo "$output"
    return 1
  fi
  [ "$output" == "Initialization successful" ]
}

@test "windsor env" {
  run windsor env

  [ "$status" -eq 0 ]

  [[ "$output" =~ export\ KUBECONFIG=\".*\" ]]
  [[ "$output" =~ export\ KUBE_CONFIG_PATH=\".*\" ]]
  # DOCKER_CONFIG may not be present in all environments
  # [[ "$output" =~ export\ DOCKER_CONFIG=\".*\" ]]
  [[ "$output" =~ export\ DOCKER_HOST=\".*\" ]]
  [[ "$output" =~ export\ OMNICONFIG=\".*\" ]]
  [[ "$output" =~ export\ TALOSCONFIG=\".*\" ]]
  [[ "$output" =~ export\ WINDSOR_CONTEXT=\".*\" ]]
  [[ "$output" =~ export\ WINDSOR_PROJECT_ROOT=\".*\" ]]
}

@test "windsor context get" {
  run windsor context get

  [ "$status" -eq 0 ]
  [[ "$output" == "local" ]]
}

@test "windsor env from terraform/cluster" {
  mkdir -p terraform/cluster
  cd terraform/cluster || exit 1
  touch main.tf

  run windsor env

  if [ "$status" -ne 0 ]; then
    echo "=== TEST FAILED: windsor env command ==="
    echo "$output"
    return 1
  fi

  # Print the output for debugging
  echo "=== windsor env command output ==="
  echo "$output"

  [[ "$output" =~ export\ OMNICONFIG=\".*\" ]]
  [[ "$output" =~ export\ TALOSCONFIG=\".*\" ]]
  [[ "$output" =~ export\ WINDSOR_CONTEXT=\".*\" ]]
  [[ "$output" =~ export\ WINDSOR_PROJECT_ROOT=\".*\" ]]
  [[ "$output" =~ export\ KUBECONFIG=\".*\" ]]
  [[ "$output" =~ export\ KUBE_CONFIG_PATH=\".*\" ]]
  # DOCKER_CONFIG may not be present in all environments
  # [[ "$output" =~ export\ DOCKER_CONFIG=\".*\" ]]
  [[ "$output" =~ export\ DOCKER_HOST=\".*\" ]]
  [[ "$output" =~ export\ TF_CLI_ARGS_apply=\".*\" ]]
  [[ "$output" =~ unset\ TF_CLI_ARGS_destroy ]]
  [[ "$output" =~ unset\ TF_CLI_ARGS_import ]]
  [[ "$output" =~ export\ TF_CLI_ARGS_init=\".*\" ]]
  [[ "$output" =~ export\ TF_CLI_ARGS_plan=\".*\" ]]
  [[ "$output" =~ export\ TF_DATA_DIR=\".*\" ]]
  [[ "$output" =~ export\ TF_VAR_context_path=\".*\" ]]
  [[ "$output" =~ export\ TF_VAR_os_type=\".*\" ]]
}
