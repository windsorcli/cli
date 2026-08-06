// The TalosClusterClient is a Talos-specific implementation of the ClusterClient interface.
// It provides cluster operations and health checks using the Talos API and gRPC.
// The TalosClusterClient acts as the primary interface for Talos cluster management.
// It coordinates node health checks, API operations, and connection lifecycle.

package cluster

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Types
// =============================================================================

// TalosClusterClient implements ClusterClient for Talos clusters
type TalosClusterClient struct {
	*BaseClusterClient
	shims  *Shims
	config *clientconfig.Config
	client *client.Client
}

// =============================================================================
// Constructor
// =============================================================================

// NewTalosClusterClient creates a new TalosClusterClient instance with default configuration
func NewTalosClusterClient() *TalosClusterClient {
	return &TalosClusterClient{
		BaseClusterClient: NewBaseClusterClient(),
		shims:             NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// WaitForNodesHealthy waits for nodes to be healthy and optionally match a specific version.
// It polls each node continuously, checking service health and version status until all nodes
// meet the criteria or timeout occurs. For each node, it validates that all critical services
// are running and healthy, and if expectedVersion is provided, verifies the node is running
// that specific version. Services listed in skipServices are ignored during health checks.
// The method provides detailed status output for each node during polling, showing healthy/unhealthy
// services and version information. Returns an error with specific details about which nodes
// failed health checks or version validation if timeout is reached.
func (c *TalosClusterClient) WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
	if err := c.ensureClient(); err != nil {
		return fmt.Errorf("failed to initialize Talos client: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.healthCheckTimeout)
	}

	var unhealthyNodes []string
	var versionMismatchNodes []string

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for nodes to be ready")
		default:
			allReady := true
			unhealthyNodes = nil
			versionMismatchNodes = nil

			for _, nodeAddress := range nodeAddresses {
				healthy, healthyServices, unhealthyServices, err := c.getNodeHealthDetails(ctx, nodeAddress, skipServices)
				if err != nil {
					fmt.Printf("Node %s: ERROR - %v\n", nodeAddress, err)
					allReady = false
					continue
				}

				var versionStatus string
				var versionOK bool = true
				if expectedVersion != "" {
					version, err := c.getNodeVersion(ctx, nodeAddress)
					if err != nil {
						versionStatus = fmt.Sprintf("version error: %v", err)
						versionOK = false
					} else if version != expectedVersion {
						versionStatus = fmt.Sprintf("version %s (expected %s)", version, expectedVersion)
						versionOK = false
						versionMismatchNodes = append(versionMismatchNodes, nodeAddress)
					} else {
						versionStatus = fmt.Sprintf("version %s", version)
					}
				}

				var statusParts []string

				if healthy {
					statusParts = append(statusParts, "HEALTHY")
				} else {
					statusParts = append(statusParts, "UNHEALTHY")
					unhealthyNodes = append(unhealthyNodes, nodeAddress)
					allReady = false
				}

				if len(healthyServices) > 0 {
					statusParts = append(statusParts, fmt.Sprintf("healthy services: %s", strings.Join(healthyServices, ", ")))
				}

				if len(unhealthyServices) > 0 {
					statusParts = append(statusParts, fmt.Sprintf("unhealthy services: %s", strings.Join(unhealthyServices, ", ")))
				}

				if versionStatus != "" {
					statusParts = append(statusParts, versionStatus)
				}

				fmt.Printf("Node %s: %s\n", nodeAddress, strings.Join(statusParts, " | "))

				if !healthy || !versionOK {
					allReady = false
				}
			}

			if allReady {
				return nil
			}

			time.Sleep(c.healthCheckPollInterval)
		}
	}

	var errorParts []string

	if len(unhealthyNodes) > 0 {
		errorParts = append(errorParts, fmt.Sprintf("unhealthy nodes: %s", strings.Join(unhealthyNodes, ", ")))
	}

	if len(versionMismatchNodes) > 0 {
		errorParts = append(errorParts, fmt.Sprintf("version mismatch nodes: %s", strings.Join(versionMismatchNodes, ", ")))
	}

	if len(errorParts) > 0 {
		return fmt.Errorf("timeout waiting for nodes (%s)", strings.Join(errorParts, "; "))
	}

	return fmt.Errorf("timeout waiting for nodes to be ready")
}

// UpgradeNodes upgrades the specified nodes to the specified image.
// It iterates through each node address and initiates an upgrade using the Talos Upgrade API.
// powercycle requests a full ACPI reboot instead of the default kexec, needed on platforms
// (e.g. nested virtualization) where kexec doesn't reliably register as an offline transition.
// Returns an error if any node upgrade fails or if the Talos client cannot be initialized.
func (c *TalosClusterClient) UpgradeNodes(ctx context.Context, nodeAddresses []string, image string, powercycle bool) error {
	if err := c.ensureClient(); err != nil {
		return fmt.Errorf("failed to initialize Talos client: %w", err)
	}

	for _, nodeAddress := range nodeAddresses {
		fmt.Printf("upgrading node %s\n", nodeAddress)

		nodeCtx := c.shims.TalosWithNodes(ctx, nodeAddress)
		err := c.shims.TalosUpgrade(nodeCtx, c.client, image, powercycle)

		if err != nil {
			return fmt.Errorf("failed to upgrade node %s: %w", nodeAddress, err)
		}
	}

	return nil
}

// bootIDPath is the standard Linux kernel pseudo-file exposing a unique ID
// generated fresh on every boot, used to confirm a genuine reboot occurred.
const bootIDPath = "/proc/sys/kernel/random/boot_id"

// CaptureNodeBootIDs reads each node's current kernel boot ID, dialing its endpoint
// directly. A node whose boot ID can't be read (permission denied, unreachable) is
// omitted from the result rather than failing the capture outright.
func (c *TalosClusterClient) CaptureNodeBootIDs(ctx context.Context, nodeAddresses []string) map[string]string {
	bootIDs := make(map[string]string, len(nodeAddresses))

	for _, nodeAddress := range nodeAddresses {
		bootID, err := c.getNodeBootID(ctx, nodeAddress)
		if err != nil {
			continue
		}

		bootIDs[nodeAddress] = bootID
	}

	return bootIDs
}

// getNodeBootID reads a single node's kernel boot ID, dialing its endpoint directly.
func (c *TalosClusterClient) getNodeBootID(ctx context.Context, nodeAddress string) (string, error) {
	nodeClient, err := c.shims.TalosNewClient(ctx, client.WithConfig(c.config), client.WithEndpoints(nodeAddress))
	if err != nil {
		return "", err
	}
	defer c.shims.TalosClose(nodeClient)

	reader, err := c.shims.TalosRead(ctx, nodeClient, bootIDPath)
	if err != nil {
		return "", err
	}
	defer reader.Close() //nolint:errcheck

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

// WaitForNodesReboot waits for nodes to reboot then come back online healthy. Phase 1
// polls each node's kernel boot ID until it differs from preActionBootIDs, confirming a
// genuine reboot occurred; unlike watching for the node to become unreachable, this
// can't miss a reboot that completes between polls, and isn't fooled by transient
// connectivity blips that were never a real reboot. A node missing from
// preActionBootIDs (its boot ID couldn't be captured beforehand) is treated as already
// satisfied. offlineTimeout caps phase 1; zero uses the context deadline. Phase 2
// re-initializes the client and delegates to WaitForNodesHealthy to wait for all nodes
// to return to a healthy state within the remaining context deadline. Returns an error
// if either phase times out or if the client cannot be initialized.
func (c *TalosClusterClient) WaitForNodesReboot(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
	if err := c.ensureClient(); err != nil {
		return fmt.Errorf("failed to initialize Talos client: %w", err)
	}

	overallDeadline, ok := ctx.Deadline()
	if !ok {
		overallDeadline = time.Now().Add(c.healthCheckTimeout)
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, overallDeadline)
		defer cancel()
	}

	// Phase 1: poll each node's boot ID until it differs from before the upgrade.
	// Use offlineTimeout as a sub-deadline so we don't burn the full timeout waiting.
	rebootDeadline := overallDeadline
	if offlineTimeout > 0 {
		candidate := time.Now().Add(offlineTimeout)
		if candidate.Before(rebootDeadline) {
			rebootDeadline = candidate
		}
	}

	fmt.Printf("Waiting for nodes to reboot...\n")
	rebootConfirmed := false
	for !rebootConfirmed && time.Now().Before(rebootDeadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for nodes to reboot")
		default:
		}

		allRebooted := true
		for _, nodeAddress := range nodeAddresses {
			preBootID, tracked := preActionBootIDs[nodeAddress]
			if !tracked {
				continue
			}

			currentBootID, err := c.getNodeBootID(ctx, nodeAddress)
			if err != nil || currentBootID == preBootID {
				fmt.Printf("Node %s: waiting for reboot\n", nodeAddress)
				allRebooted = false
				continue
			}

			fmt.Printf("Node %s: rebooted\n", nodeAddress)
		}

		if allRebooted {
			rebootConfirmed = true
		} else {
			time.Sleep(c.healthCheckPollInterval)
		}
	}

	if !rebootConfirmed {
		return fmt.Errorf("timeout waiting for nodes to reboot")
	}

	fmt.Printf("All nodes rebooted, waiting for reboot to complete...\n")

	// Phase 2: reset client and wait for nodes to come back healthy.
	c.Close()
	c.client = nil
	return c.WaitForNodesHealthy(ctx, nodeAddresses, expectedVersion, skipServices)
}

// WaitForControlPlaneAPIReady waits for the kube-apiserver on a control-plane node
// to accept TCP connections on port 6443. It first queries the Talos ServiceList to
// determine the node's role: nodes that do not run the etcd service are treated as
// workers and the method returns nil immediately. For control-plane nodes, it polls
// a TCP dial to the apiserver port until a connection succeeds or the context deadline
// is reached. The per-dial timeout is half the health check poll interval so a slow
// dial does not starve polling. outputFunc, when non-nil, receives a status message
// on each failed poll attempt. Returns an error if the role cannot be determined or
// the apiserver does not become reachable in time.
func (c *TalosClusterClient) WaitForControlPlaneAPIReady(ctx context.Context, nodeAddress string, outputFunc func(string)) error {
	if err := c.ensureClient(); err != nil {
		return fmt.Errorf("failed to initialize Talos client: %w", err)
	}

	isControlPlane, err := c.isControlPlaneNode(ctx, nodeAddress)
	if err != nil {
		return fmt.Errorf("failed to determine node role for %s: %w", nodeAddress, err)
	}
	if !isControlPlane {
		return nil
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(constants.DefaultAPIServerReadyTimeout)
	}

	dialTimeout := c.healthCheckPollInterval / 2
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}

	address := net.JoinHostPort(nodeAddress, fmt.Sprintf("%d", constants.DefaultAPIServerPort))

	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for kube-apiserver on %s: %w", address, ctx.Err())
		default:
		}

		conn, dialErr := c.shims.NetDialTimeout("tcp", address, dialTimeout)
		if dialErr == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = dialErr
		if outputFunc != nil {
			outputFunc(fmt.Sprintf("kube-apiserver on %s not ready, retrying...", address))
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for kube-apiserver on %s: %w", address, ctx.Err())
		case <-time.After(c.healthCheckPollInterval):
		}
	}

	if lastErr != nil {
		return fmt.Errorf("timeout waiting for kube-apiserver on %s: %w", address, lastErr)
	}
	return fmt.Errorf("timeout waiting for kube-apiserver on %s", address)
}

// Close releases resources held by the TalosClusterClient.
// It safely closes the underlying Talos gRPC client connection if one exists and sets
// the client reference to nil to prevent further use. This method is safe to call
// multiple times and handles the case where no client connection was established.
func (c *TalosClusterClient) Close() {
	if c.client != nil {
		c.shims.TalosClose(c.client)
		c.client = nil
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureClient lazily initializes the Talos client if not already set.
// It checks if a client already exists and returns early if so. Otherwise, it reads the
// TALOSCONFIG environment variable to locate the configuration file, loads and parses
// the Talos configuration using the shim layer, then creates a new Talos gRPC client
// with the loaded configuration. Returns an error if the environment variable is not
// set, the configuration file cannot be loaded, or the client creation fails.
func (c *TalosClusterClient) ensureClient() error {
	if c.client != nil {
		return nil
	}

	configPath := os.Getenv("TALOSCONFIG")
	if configPath == "" {
		return fmt.Errorf("TALOSCONFIG environment variable not set")
	}

	var err error
	c.config, err = c.shims.TalosConfigOpen(configPath)
	if err != nil {
		return fmt.Errorf("error loading Talos config: %w", err)
	}

	c.client, err = c.shims.TalosNewClient(context.Background(),
		client.WithConfig(c.config),
	)
	if err != nil {
		return fmt.Errorf("error creating Talos client: %w", err)
	}

	return nil
}

// getNodeHealthDetails gets detailed health information for a single node.
// It creates a node-specific context targeting the given node address, then queries
// the Talos ServiceList API to retrieve all services running on that node. For each
// service, it checks both the running state and health status to determine if the
// service is fully operational. Services listed in skipServices are excluded from
// health checks entirely. Only essential services (apid, machined, kubelet, etcd, trustd)
// determine overall node health — non-essential services are reported but do not affect
// the healthy result. Extension services that explicitly report Unknown=true (e.g. ext-iscsid)
// are treated as healthy when running. A nil health field is distinct from Unknown=true
// and is treated as unhealthy. Returns the overall node health status, lists of healthy
// and unhealthy service names, and any error encountered during the API call.
func (c *TalosClusterClient) getNodeHealthDetails(ctx context.Context, nodeAddress string, skipServices []string) (bool, []string, []string, error) {
	nodeCtx := c.shims.TalosWithNodes(ctx, nodeAddress)

	serviceResp, err := c.shims.TalosServiceList(nodeCtx, c.client)
	if err != nil {
		return false, nil, nil, err
	}

	skipMap := make(map[string]bool)
	for _, svc := range skipServices {
		skipMap[svc] = true
	}

	// Essential services that must be healthy for the node to be considered healthy.
	// Based on Talos machine status controller requirements.
	essentialServices := map[string]bool{
		"apid":     true,
		"machined": true,
		"kubelet":  true,
		"etcd":     true,
		"trustd":   true,
	}

	var healthyServices []string
	var unhealthyServices []string
	overallHealthy := true

	for _, serviceList := range serviceResp.GetMessages() {
		for _, service := range serviceList.GetServices() {
			serviceName := service.GetId()

			if skipMap[serviceName] {
				continue
			}

			state := service.GetState()
			health := service.GetHealth()

			isRunning := state == "Running"
			healthUnknown := health != nil && health.GetUnknown()
			isHealthy := isRunning && ((health != nil && health.GetHealthy()) || healthUnknown)

			if isHealthy {
				healthyServices = append(healthyServices, serviceName)
			} else {
				unhealthyServices = append(unhealthyServices, serviceName)
				if essentialServices[serviceName] {
					overallHealthy = false
				}
			}
		}
	}

	return overallHealthy, healthyServices, unhealthyServices, nil
}

// isControlPlaneNode determines whether a node is a control-plane node by checking
// whether the Talos ServiceList reports an etcd service. etcd runs only on control-plane
// nodes, so its presence is a reliable role signal. Returns an error only if the
// ServiceList API call fails.
func (c *TalosClusterClient) isControlPlaneNode(ctx context.Context, nodeAddress string) (bool, error) {
	nodeCtx := c.shims.TalosWithNodes(ctx, nodeAddress)
	serviceResp, err := c.shims.TalosServiceList(nodeCtx, c.client)
	if err != nil {
		return false, err
	}
	for _, serviceList := range serviceResp.GetMessages() {
		for _, service := range serviceList.GetServices() {
			if service.GetId() == "etcd" {
				return true, nil
			}
		}
	}
	return false, nil
}

// getNodeVersion gets the version of a single node, dialing its endpoint directly
// rather than proxying through the shared client, so an unreachable node fails
// promptly instead of the proxy masking it. Returns an error if the API call fails.
func (c *TalosClusterClient) getNodeVersion(ctx context.Context, nodeAddress string) (string, error) {
	nodeClient, err := c.shims.TalosNewClient(ctx, client.WithConfig(c.config), client.WithEndpoints(nodeAddress))
	if err != nil {
		return "", err
	}
	defer c.shims.TalosClose(nodeClient)

	version, err := c.shims.TalosVersion(ctx, nodeClient)
	if err != nil {
		return "", err
	}

	versionTag := version.Messages[0].Version.Tag
	return strings.TrimPrefix(versionTag, "v"), nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TalosClusterClient implements ClusterClient
var _ ClusterClient = (*TalosClusterClient)(nil)
