// The kubernetes_manager_health file holds the cluster health checks of the BaseKubernetesManager.
// It provides a wait until the Kubernetes API is healthy and stays healthy.
// It can also wait until the named nodes report Ready.
// It reports the last failure when the wait times out.

package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// Public Methods
// =============================================================================

// WaitForKubernetesHealthy waits for the Kubernetes API to become healthy within the context deadline.
// If nodeNames are provided, verifies all specified nodes reach Ready state before returning.
// A machine config apply (e.g. Talos resource reservations touching the apiServer/controllerManager/
// scheduler static pods) is accepted synchronously but reconciled asynchronously, so the very next
// health check can still observe the pre-change apiserver and return healthy moments before it's
// recreated. To catch that race, a success only counts once the API (and, if requested, node
// readiness) has held continuously for healthCheckSettleDuration; any failure during that window
// resets the clock. healthCheckSettleDuration is bounded by the context's own remaining deadline
// (minus one poll interval of margin) so a caller with a short overall timeout — e.g. the 30s
// reachability check windsor destroy runs before invoking terraform — still has a reachable window
// in which to succeed, rather than the settle requirement alone guaranteeing a timeout regardless of
// cluster health. Returns an error if the API is unreachable or any specified nodes are not Ready
// within the deadline.
func (k *BaseKubernetesManager) WaitForKubernetesHealthy(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
	if k.client == nil {
		return fmt.Errorf("kubernetes client not initialized")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = k.shims.TimeNow().Add(5 * time.Minute)
	}

	pollInterval := k.healthCheckPollInterval
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	settleDuration := k.healthCheckSettleDuration
	if settleDuration == 0 {
		settleDuration = 30 * time.Second
	}
	if remaining := deadline.Sub(k.shims.TimeNow()) - pollInterval; settleDuration > remaining {
		if remaining < 0 {
			remaining = 0
		}
		settleDuration = remaining
	}

	var lastErr error
	var settleSince time.Time
	for k.shims.TimeNow().Before(deadline) {
		select {
		case <-ctx.Done():
			return healthyTimeoutError(lastErr)
		default:
			if err := k.client.CheckHealth(ctx, endpoint); err != nil {
				lastErr = fmt.Errorf("health check for API endpoint %s failed: %w", endpoint, err)
				settleSince = time.Time{}
				select {
				case <-ctx.Done():
					return healthyTimeoutError(lastErr)
				case <-time.After(pollInterval):
					continue
				}
			}

			if len(nodeNames) > 0 {
				if err := k.waitForNodesReady(ctx, nodeNames, outputFunc); err != nil {
					lastErr = err
					settleSince = time.Time{}
					select {
					case <-ctx.Done():
						return healthyTimeoutError(lastErr)
					case <-time.After(pollInterval):
						continue
					}
				}
			}

			if settleSince.IsZero() {
				settleSince = k.shims.TimeNow()
			}
			if k.shims.TimeNow().Sub(settleSince) < settleDuration {
				select {
				case <-ctx.Done():
					return healthyTimeoutError(lastErr)
				case <-time.After(pollInterval):
					continue
				}
			}

			return nil
		}
	}

	return healthyTimeoutError(lastErr)
}

// GetNodeReadyStatus returns a map of node names to their Ready condition status.
// Returns a map of node names to Ready status (true if Ready, false if NotReady), or an error if listing fails.
func (k *BaseKubernetesManager) GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error) {
	if k.client == nil {
		return nil, fmt.Errorf("kubernetes client not initialized")
	}
	return k.client.GetNodeReadyStatus(ctx, nodeNames)
}

// =============================================================================
// Private Methods
// =============================================================================

// waitForNodesReady blocks until all specified nodes exist and are in Ready state or the context deadline is reached.
// It periodically queries node status, invokes outputFunc on status changes, and returns an error if any nodes are missing or not Ready within the deadline.
// If the context is cancelled, returns an error immediately.
func (k *BaseKubernetesManager) waitForNodesReady(ctx context.Context, nodeNames []string, outputFunc func(string)) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Minute)
	}

	pollInterval := k.nodeReadyPollInterval
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}
	lastStatus := make(map[string]string)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for nodes to be ready")
		default:
			readyStatus, err := k.client.GetNodeReadyStatus(ctx, nodeNames)
			if err != nil {
				select {
				case <-ctx.Done():
					return fmt.Errorf("context cancelled while waiting for nodes to be ready")
				case <-time.After(pollInterval):
					continue
				}
			}

			var missingNodes []string
			var notReadyNodes []string

			for _, nodeName := range nodeNames {
				if ready, exists := readyStatus[nodeName]; !exists {
					missingNodes = append(missingNodes, nodeName)
				} else if !ready {
					notReadyNodes = append(notReadyNodes, nodeName)
				}
			}

			if outputFunc != nil {
				for _, nodeName := range nodeNames {
					var currentStatus string
					if ready, exists := readyStatus[nodeName]; !exists {
						currentStatus = "NOT FOUND"
					} else if ready {
						currentStatus = "READY"
					} else {
						currentStatus = "NOT READY"
					}

					if lastStatus[nodeName] != currentStatus {
						outputFunc(fmt.Sprintf("Node %s: %s", nodeName, currentStatus))
						lastStatus[nodeName] = currentStatus
					}
				}
			}

			if len(missingNodes) == 0 && len(notReadyNodes) == 0 {
				return nil
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while waiting for nodes to be ready")
			case <-time.After(pollInterval):
				continue
			}
		}
	}

	readyStatus, err := k.client.GetNodeReadyStatus(ctx, nodeNames)
	if err != nil {
		return fmt.Errorf("timeout waiting for nodes to be ready: failed to get final status: %w", err)
	}

	var missingNodes []string
	var notReadyNodes []string

	for _, nodeName := range nodeNames {
		if ready, exists := readyStatus[nodeName]; !exists {
			missingNodes = append(missingNodes, nodeName)
		} else if !ready {
			notReadyNodes = append(notReadyNodes, nodeName)
		}
	}

	if len(missingNodes) > 0 {
		return fmt.Errorf("timeout waiting for nodes to appear: %s", strings.Join(missingNodes, ", "))
	}

	if len(notReadyNodes) > 0 {
		return fmt.Errorf("timeout waiting for nodes to be ready: %s", strings.Join(notReadyNodes, ", "))
	}

	return fmt.Errorf("timeout waiting for nodes to be ready")
}

// =============================================================================
// Helpers
// =============================================================================

// healthyTimeoutError builds the WaitForKubernetesHealthy timeout error. When a
// health-check or node-readiness failure was seen on the final attempt it is
// appended so the operator learns why the wait gave up (a failed API health
// check, or specific nodes that never reached Ready) rather than only that it
// did. Falls back to the bare message when no underlying cause was recorded.
func healthyTimeoutError(lastErr error) error {
	if lastErr == nil {
		return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
	}
	return fmt.Errorf("timeout waiting for Kubernetes API to be healthy: %w", lastErr)
}
