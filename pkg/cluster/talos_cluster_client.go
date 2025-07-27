// The TalosClusterClient is a Talos-specific implementation of the ClusterClient interface.
// It provides cluster operations and health checks using the Talos API and gRPC.
// The TalosClusterClient acts as the primary interface for Talos cluster management.
// It coordinates node health checks, API operations, and connection lifecycle.

package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/di"

	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
)

// =============================================================================
// Types
// =============================================================================

// TalosClusterClient implements ClusterClient for Talos clusters
type TalosClusterClient struct {
	*BaseClusterClient
	injector di.Injector
	shims    *Shims
	config   *clientconfig.Config
	client   *client.Client
}

// =============================================================================
// Constructor
// =============================================================================

// NewTalosClusterClient creates a new TalosClusterClient instance with default configuration
func NewTalosClusterClient(injector di.Injector) *TalosClusterClient {
	return &TalosClusterClient{
		BaseClusterClient: NewBaseClusterClient(),
		injector:          injector,
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
// that specific version. The method provides detailed status output for each node during polling,
// showing healthy/unhealthy services and version information. Returns an error with specific
// details about which nodes failed health checks or version validation if timeout is reached.
func (c *TalosClusterClient) WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
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
				healthy, healthyServices, unhealthyServices, err := c.getNodeHealthDetails(ctx, nodeAddress)
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
// service is fully operational. Returns the overall node health status, lists of
// healthy and unhealthy service names, and any error encountered during the API call.
func (c *TalosClusterClient) getNodeHealthDetails(ctx context.Context, nodeAddress string) (bool, []string, []string, error) {
	nodeCtx := c.shims.TalosWithNodes(ctx, nodeAddress)

	serviceResp, err := c.shims.TalosServiceList(nodeCtx, c.client)
	if err != nil {
		return false, nil, nil, err
	}

	var healthyServices []string
	var unhealthyServices []string
	overallHealthy := true

	for _, serviceList := range serviceResp.GetMessages() {
		for _, service := range serviceList.GetServices() {
			serviceName := service.GetId()

			state := service.GetState()
			health := service.GetHealth()

			isRunning := state == "Running"
			isHealthy := health != nil && health.GetHealthy()

			if isRunning && isHealthy {
				healthyServices = append(healthyServices, serviceName)
			} else {
				unhealthyServices = append(unhealthyServices, serviceName)
				overallHealthy = false
			}
		}
	}

	return overallHealthy, healthyServices, unhealthyServices, nil
}

// getNodeVersion gets the version of a single node.
// It creates a node-specific context targeting the given node address, then calls
// the Talos Version API to retrieve version information from that node. The method
// extracts the version tag from the API response and removes any leading 'v' prefix
// to return a clean version string. Returns an error if the API call fails or if
// the response format is unexpected.
func (c *TalosClusterClient) getNodeVersion(ctx context.Context, nodeAddress string) (string, error) {
	nodeCtx := c.shims.TalosWithNodes(ctx, nodeAddress)

	version, err := c.shims.TalosVersion(nodeCtx, c.client)
	if err != nil {
		return "", err
	}

	versionTag := version.Messages[0].Version.Tag
	return strings.TrimPrefix(versionTag, "v"), nil
}

// Ensure TalosClusterClient implements ClusterClient
var _ ClusterClient = (*TalosClusterClient)(nil)
