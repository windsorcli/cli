// The node methods of the Provisioner upgrade cluster nodes and check their health.
// It provides UpgradeNode, UpgradeNodes, and CheckNodeHealth, and the NodeHealthCheckOptions type.
// UpgradeNode also deletes the stale Kubernetes Node object of a node that restarted under a new name.

package provisioner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// noHealthChecksHint is the remediation for a node health check with no checks to run.
const noHealthChecksHint = "Use --nodes, --k8s-endpoint, or both to choose the health checks."

// =============================================================================
// Types
// =============================================================================

// NodeHealthCheckOptions contains options for node health checking.
type NodeHealthCheckOptions struct {
	Nodes               []string
	Timeout             time.Duration
	Version             string
	K8SEndpoint         string
	K8SEndpointProvided bool
	CheckNodeReady      bool
	SkipServices        []string
	WaitForReboot       bool
	OfflineTimeout      time.Duration
}

// =============================================================================
// Public Methods
// =============================================================================

// UpgradeNode upgrades one node to image. It records the boot ID and the Kubernetes node names, sends the upgrade request, waits
// for the boot ID to change within offlineTimeout, and then waits for the node and its API server.
// If the node re-registered under a new name, it then deletes the old Node object when it is not Ready.
// A failure to read or delete Node objects is reported through outputFunc and does not fail the upgrade.
// powercycle requests a full reboot instead of kexec, for platforms where kexec does not show as
// offline. When skipIfCurrent is true, it first reads the running version of the node. If that
// version equals the image tag, it reports this through outputFunc and returns nil without an
// upgrade request or reboot. If the version cannot be read or the image has no tag, the upgrade
// proceeds. outputFunc receives status messages. It returns an error when a step fails or times out.
func (i *Provisioner) UpgradeNode(ctx context.Context, node string, image string, offlineTimeout time.Duration, powercycle bool, skipIfCurrent bool, outputFunc func(string)) error {
	if err := i.ensureClusterClient(); err != nil {
		return err
	}
	defer i.ClusterClient.Close()

	if skipIfCurrent && i.nodeIsCurrent(ctx, node, image, outputFunc) {
		return nil
	}

	return i.rollNode(ctx, node, image, offlineTimeout, powercycle, outputFunc)
}

// UpgradeNodes upgrades the named cluster nodes to image, one at a time. It reads the role of each
// node, then upgrades the control planes first and the workers after them, each group in the order
// named. Each node runs the same steps as UpgradeNode, so the next node starts only after the
// previous one is healthy. It stops at the first failure. This keeps etcd quorum and leaves
// workloads on the other nodes running. powercycle requests a full ACPI reboot instead of the
// default kexec. nodeTimeout limits the role read and the whole upgrade of each node, and zero sets no limit.
// offlineTimeout limits the reboot wait for each node. When skipIfCurrent is true, a node that
// already runs the image version is skipped, as in UpgradeNode. outputFunc receives status
// messages. It returns an error when the client cannot start, a role cannot be read, or a step fails.
func (i *Provisioner) UpgradeNodes(ctx context.Context, nodes []string, image string, nodeTimeout time.Duration, offlineTimeout time.Duration, powercycle bool, skipIfCurrent bool, outputFunc func(string)) error {
	if err := i.ensureClusterClient(); err != nil {
		return err
	}
	defer i.ClusterClient.Close()

	var controlPlanes, workers []string
	for _, node := range nodes {
		roleCtx, cancel := ctx, context.CancelFunc(func() {})
		if nodeTimeout > 0 {
			roleCtx, cancel = context.WithTimeout(ctx, nodeTimeout)
		}
		isControlPlane, err := i.ClusterClient.IsControlPlaneNode(roleCtx, node)
		cancel()
		if err != nil {
			return werror.Wrap(err, "failed to read the role of node %s", node)
		}
		if isControlPlane {
			controlPlanes = append(controlPlanes, node)
		} else {
			workers = append(workers, node)
		}
	}

	for _, node := range append(controlPlanes, workers...) {
		if err := i.upgradeOneOfMany(ctx, node, image, nodeTimeout, offlineTimeout, powercycle, skipIfCurrent, outputFunc); err != nil {
			return werror.Wrap(err, "node %s failed to upgrade; upgrades stopped", node)
		}
	}
	return nil
}

// CheckNodeHealth checks the health of cluster nodes and of the Kubernetes API endpoint.
// It checks nodes through the cluster client for Talos and Omni clusters. It also checks the version
// and the readiness of each node. The node check and the Kubernetes check each wait up to options.Timeout.
// It returns an error if a health check fails. Options with no health checks, or --ready without --nodes,
// return an error with a remediation hint.
func (i *Provisioner) CheckNodeHealth(ctx context.Context, options NodeHealthCheckOptions, outputFunc func(string)) error {
	hasNodeCheck := len(options.Nodes) > 0
	hasK8sCheck := options.K8SEndpointProvided

	if !hasNodeCheck && !hasK8sCheck {
		return werror.New("no health checks specified", noHealthChecksHint, nil)
	}

	if hasNodeCheck {
		_ = i.ensureClusterClient() // best-effort; nil client is tolerated when hasK8sCheck

		if i.ClusterClient == nil {
			if !hasK8sCheck {
				return werror.New("no health checks specified", noHealthChecksHint, nil)
			}
		}

		if i.ClusterClient != nil {
			defer i.ClusterClient.Close()

			var checkCtx context.Context
			var cancel context.CancelFunc
			if options.Timeout > 0 {
				checkCtx, cancel = context.WithTimeout(ctx, options.Timeout)
			} else {
				checkCtx, cancel = context.WithCancel(ctx)
			}
			defer cancel()

			var clusterErr error
			if options.WaitForReboot {
				preActionBootIDs := i.ClusterClient.CaptureNodeBootIDs(checkCtx, options.Nodes)
				clusterErr = i.ClusterClient.WaitForNodesReboot(checkCtx, options.Nodes, preActionBootIDs, options.Version, options.SkipServices, options.OfflineTimeout)
			} else {
				clusterErr = i.ClusterClient.WaitForNodesHealthy(checkCtx, options.Nodes, options.Version, options.SkipServices)
			}
			if err := clusterErr; err != nil {
				if hasK8sCheck {
					if outputFunc != nil {
						outputFunc(fmt.Sprintf("Warning: Cluster client failed (%v), continuing with Kubernetes checks\n", err))
					}
				} else {
					return werror.Wrap(err, "nodes failed health check")
				}
			} else {
				if outputFunc != nil {
					message := fmt.Sprintf("All %d nodes are healthy", len(options.Nodes))
					if options.Version != "" {
						message += fmt.Sprintf(" and running version %s", options.Version)
					}
					outputFunc(message)
				}
			}
		}
	}

	if hasK8sCheck {
		if i.KubernetesManager == nil {
			return fmt.Errorf("no kubernetes manager found")
		}

		if options.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, options.Timeout)
			defer cancel()
		}

		k8sEndpointStr := options.K8SEndpoint
		if k8sEndpointStr == "true" {
			k8sEndpointStr = ""
		}

		var nodeNames []string
		if options.CheckNodeReady {
			if hasNodeCheck {
				nodeNames = options.Nodes
			} else {
				return werror.New("--ready flag requires --nodes to be specified", "Add --nodes with the nodes to check.", nil)
			}
		}

		if len(nodeNames) > 0 && outputFunc != nil {
			outputFunc(fmt.Sprintf("Waiting for %d nodes to be Ready...", len(nodeNames)))
		}

		if err := i.KubernetesManager.WaitForKubernetesHealthy(ctx, k8sEndpointStr, outputFunc, nodeNames...); err != nil {
			return werror.Wrap(err, "kubernetes health check failed")
		}

		if outputFunc != nil {
			if len(nodeNames) > 0 {
				readyStatus, err := i.KubernetesManager.GetNodeReadyStatus(ctx, nodeNames)
				allFoundAndReady := err == nil && len(readyStatus) == len(nodeNames)
				for _, ready := range readyStatus {
					if !ready {
						allFoundAndReady = false
						break
					}
				}

				if allFoundAndReady {
					if k8sEndpointStr != "" {
						outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy and all nodes are Ready", k8sEndpointStr))
					} else {
						outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy and all nodes are Ready")
					}
				} else {
					if k8sEndpointStr != "" {
						outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
					} else {
						outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy")
					}
				}
			} else {
				if k8sEndpointStr != "" {
					outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
				} else {
					outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy")
				}
			}
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// upgradeOneOfMany upgrades one node of a larger run. It applies nodeTimeout to this node only,
// so a slow node does not shorten the time left for the next one. When skipIfCurrent is true and
// the node already runs the image version, it returns nil without a request or reboot.
func (i *Provisioner) upgradeOneOfMany(ctx context.Context, node string, image string, nodeTimeout time.Duration, offlineTimeout time.Duration, powercycle bool, skipIfCurrent bool, outputFunc func(string)) error {
	if nodeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, nodeTimeout)
		defer cancel()
	}
	if skipIfCurrent && i.nodeIsCurrent(ctx, node, image, outputFunc) {
		return nil
	}
	return i.rollNode(ctx, node, image, offlineTimeout, powercycle, outputFunc)
}

// nodeIsCurrent reports whether the node already runs the version of image. It reports a skip
// through outputFunc. It returns false when the version cannot be read or the image has no tag.
func (i *Provisioner) nodeIsCurrent(ctx context.Context, node string, image string, outputFunc func(string)) bool {
	target := versionFromImage(image)
	current, err := i.ClusterClient.GetNodeVersion(ctx, node)
	if err != nil || target == "" || current != target {
		return false
	}
	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Node %s is already at version %s. Skipping upgrade.", node, current))
	}
	return true
}

// rollNode sends one upgrade request to node and waits for it to finish. It records the boot ID,
// sends the request, waits for the boot ID to change within offlineTimeout, and then waits for
// the node and, on a control plane, its kube-apiserver. outputFunc receives status messages.
func (i *Provisioner) rollNode(ctx context.Context, node string, image string, offlineTimeout time.Duration, powercycle bool, outputFunc func(string)) error {
	previousNodeNames := i.previousNodeNames(ctx, node)

	nodes := []string{node}
	preActionBootIDs := i.ClusterClient.CaptureNodeBootIDs(ctx, nodes)

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Sending upgrade request to node %s...", node))
	}
	if err := i.ClusterClient.UpgradeNodes(ctx, nodes, image, powercycle); err != nil {
		return werror.Wrap(err, "upgrade request failed")
	}

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Upgrade request sent to %s. Waiting for reboot...", node))
	}
	if err := i.ClusterClient.WaitForNodesReboot(ctx, nodes, preActionBootIDs, versionFromImage(image), nil, offlineTimeout); err != nil {
		return werror.Wrap(err, "node reboot wait failed")
	}

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Verifying kube-apiserver readiness on %s (skipped for workers)...", node))
	}
	if err := i.ClusterClient.WaitForControlPlaneAPIReady(ctx, node, outputFunc); err != nil {
		return werror.Wrap(err, "kube-apiserver readiness check failed")
	}

	i.removeStaleNodes(ctx, node, previousNodeNames, outputFunc)

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Node %s upgraded successfully.", node))
	}
	return nil
}

// previousNodeNames returns the names of the Kubernetes Node objects at address before an upgrade.
// It returns nil when no Kubernetes manager is set or the lookup fails, because the cleanup is optional.
func (i *Provisioner) previousNodeNames(ctx context.Context, address string) []string {
	if i.KubernetesManager == nil {
		return nil
	}
	names, err := i.KubernetesManager.GetNodeNamesByAddress(ctx, address)
	if err != nil {
		return nil
	}
	return names
}

// removeStaleNodes deletes the Node objects that the node had before it restarted under a new name.
// It reports each deletion and each failure through outputFunc and never returns an error.
func (i *Provisioner) removeStaleNodes(ctx context.Context, address string, previousNames []string, outputFunc func(string)) {
	if i.KubernetesManager == nil || len(previousNames) == 0 {
		return
	}
	deleted, err := i.KubernetesManager.RemoveStaleNodes(ctx, address, previousNames)
	if outputFunc == nil {
		return
	}
	for _, name := range deleted {
		outputFunc(fmt.Sprintf("Deleted stale Kubernetes node %s, which the upgraded node replaced.", name))
	}
	if err != nil {
		outputFunc(fmt.Sprintf("Could not delete the stale Kubernetes node for %s: %v", address, err))
	}
}

// =============================================================================
// Helpers
// =============================================================================

// versionFromImage extracts the Talos version from an image URI tag.
// It returns the tag with the leading "v" stripped to match the format returned
// by the Talos Version API. Returns an empty string if no tag is present or if
// the reference uses a digest instead of a tag.
func versionFromImage(image string) string {
	if strings.Contains(image, "@") {
		return ""
	}
	idx := strings.LastIndex(image, ":")
	if idx < 0 {
		return ""
	}
	tag := image[idx+1:]
	if strings.Contains(tag, "/") {
		return ""
	}
	return strings.TrimPrefix(tag, "v")
}
