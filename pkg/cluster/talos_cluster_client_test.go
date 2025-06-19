package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupShims initializes and returns shims for tests
func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := NewShims()

	shims.TalosConfigOpen = func(configPath string) (*clientconfig.Config, error) {
		return &clientconfig.Config{}, nil
	}

	shims.TalosNewClient = func(ctx context.Context, opts ...talosclient.OptionFunc) (*talosclient.Client, error) {
		return &talosclient.Client{}, nil
	}

	shims.TalosWithNodes = func(ctx context.Context, nodes ...string) context.Context {
		return ctx
	}

	shims.TalosServiceList = func(ctx context.Context, client *talosclient.Client) (*machine.ServiceListResponse, error) {
		return &machine.ServiceListResponse{
			Messages: []*machine.ServiceList{
				{
					Services: []*machine.ServiceInfo{
						{
							Id:    "apid",
							State: "Running",
							Health: &machine.ServiceHealth{
								Healthy: true,
							},
						},
						{
							Id:    "machined",
							State: "Running",
							Health: &machine.ServiceHealth{
								Healthy: true,
							},
						},
					},
				},
			},
		}, nil
	}

	shims.TalosVersion = func(ctx context.Context, client *talosclient.Client) (*machine.VersionResponse, error) {
		return &machine.VersionResponse{
			Messages: []*machine.Version{
				{
					Version: &machine.VersionInfo{
						Tag: "v1.0.0",
					},
				},
			},
		}, nil
	}

	return shims
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTalosClusterClient(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		client := NewTalosClusterClient(di.NewMockInjector())

		if client == nil {
			t.Error("Expected non-nil TalosClusterClient")
		}
		if client.BaseClusterClient == nil {
			t.Error("Expected non-nil BaseClusterClient")
		}
		if client.injector == nil {
			t.Error("Expected injector to be set")
		}
		if client.shims == nil {
			t.Error("Expected shims to be initialized")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTalosClusterClient_WaitForNodesHealthy(t *testing.T) {
	setup := func(t *testing.T) *TalosClusterClient {
		t.Helper()
		client := NewTalosClusterClient(di.NewMockInjector())
		client.shims = setupShims(t)
		client.healthCheckTimeout = 100 * time.Millisecond
		client.healthCheckPollInterval = 10 * time.Millisecond
		return client
	}

	t.Run("Success", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1", "10.0.0.2"}
		expectedVersion := "1.0.0"

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, expectedVersion)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithoutVersion", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1"}

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, "")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("EnsureClientError", func(t *testing.T) {
		client := setup(t)

		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1"}

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, "")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "TALOSCONFIG environment variable not set") {
			t.Errorf("Expected TALOSCONFIG error, got %v", err)
		}
	})

	t.Run("HealthCheckError", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		client.shims.TalosServiceList = func(ctx context.Context, client *talosclient.Client) (*machine.ServiceListResponse, error) {
			return nil, fmt.Errorf("service list error")
		}

		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1"}

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, "")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes") {
			t.Errorf("Expected timeout error, got %v", err)
		}
	})

	t.Run("UnhealthyServices", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		client.shims.TalosServiceList = func(ctx context.Context, client *talosclient.Client) (*machine.ServiceListResponse, error) {
			return &machine.ServiceListResponse{
				Messages: []*machine.ServiceList{
					{
						Services: []*machine.ServiceInfo{
							{
								Id:    "apid",
								State: "Stopped",
								Health: &machine.ServiceHealth{
									Healthy: false,
								},
							},
						},
					},
				},
			}, nil
		}

		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1"}

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, "")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unhealthy nodes") {
			t.Errorf("Expected unhealthy nodes error, got %v", err)
		}
	})

	t.Run("VersionMismatch", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		client.shims.TalosVersion = func(ctx context.Context, client *talosclient.Client) (*machine.VersionResponse, error) {
			return &machine.VersionResponse{
				Messages: []*machine.Version{
					{
						Version: &machine.VersionInfo{
							Tag: "v1.1.0",
						},
					},
				},
			}, nil
		}

		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1"}
		expectedVersion := "1.0.0"

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, expectedVersion)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "version mismatch nodes") {
			t.Errorf("Expected version mismatch error, got %v", err)
		}
	})

	t.Run("VersionError", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		client.shims.TalosVersion = func(ctx context.Context, client *talosclient.Client) (*machine.VersionResponse, error) {
			return nil, fmt.Errorf("version error")
		}

		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1"}
		expectedVersion := "1.0.0"

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, expectedVersion)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes") {
			t.Errorf("Expected timeout error, got %v", err)
		}
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		nodeAddresses := []string{"10.0.0.1"}

		err := client.WaitForNodesHealthy(ctx, nodeAddresses, "")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to be ready") {
			t.Errorf("Expected timeout error, got %v", err)
		}
	})
}

func TestTalosClusterClient_Close(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		client := NewTalosClusterClient(di.NewMockInjector())

		// Test that Close doesn't panic when client is nil
		client.client = nil

		client.Close()

		if client.client != nil {
			t.Error("Expected client to remain nil after Close")
		}
	})

	t.Run("SuccessWithClient", func(t *testing.T) {
		client := NewTalosClusterClient(di.NewMockInjector())
		client.shims = setupShims(t)

		// Set up a mock client (we'll use a non-nil pointer to simulate having a client)
		mockClient := &talosclient.Client{}
		client.client = mockClient

		// Mock the TalosClose function to track if it was called
		closeCalled := false
		client.shims.TalosClose = func(c *talosclient.Client) {
			closeCalled = true
		}

		client.Close()

		// Verify TalosClose was called
		if !closeCalled {
			t.Error("Expected TalosClose to be called")
		}

		// Verify client is set to nil
		if client.client != nil {
			t.Error("Expected client to be nil after Close")
		}
	})

	t.Run("NoClient", func(t *testing.T) {
		client := NewTalosClusterClient(di.NewMockInjector())

		client.Close()
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTalosClusterClient_ensureClient(t *testing.T) {
	setup := func(t *testing.T) *TalosClusterClient {
		t.Helper()
		client := NewTalosClusterClient(di.NewMockInjector())
		client.shims = setupShims(t)
		return client
	}

	t.Run("Success", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		err := client.ensureClient()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if client.client == nil {
			t.Error("Expected client to be set")
		}
		if client.config == nil {
			t.Error("Expected config to be set")
		}
	})

	t.Run("ClientAlreadyExists", func(t *testing.T) {
		client := setup(t)
		client.client = &talosclient.Client{}

		err := client.ensureClient()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("MissingTalosConfig", func(t *testing.T) {
		client := setup(t)

		err := client.ensureClient()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "TALOSCONFIG environment variable not set") {
			t.Errorf("Expected TALOSCONFIG error, got %v", err)
		}
	})

	t.Run("ConfigOpenError", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		client.shims.TalosConfigOpen = func(configPath string) (*clientconfig.Config, error) {
			return nil, fmt.Errorf("config open error")
		}

		err := client.ensureClient()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error loading Talos config") {
			t.Errorf("Expected config loading error, got %v", err)
		}
	})

	t.Run("ClientCreationError", func(t *testing.T) {
		client := setup(t)
		os.Setenv("TALOSCONFIG", "/tmp/talosconfig")
		defer os.Unsetenv("TALOSCONFIG")

		client.shims.TalosNewClient = func(ctx context.Context, opts ...talosclient.OptionFunc) (*talosclient.Client, error) {
			return nil, fmt.Errorf("client creation error")
		}

		err := client.ensureClient()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error creating Talos client") {
			t.Errorf("Expected client creation error, got %v", err)
		}
	})
}

func TestTalosClusterClient_getNodeHealthDetails(t *testing.T) {
	setup := func(t *testing.T) *TalosClusterClient {
		t.Helper()
		client := NewTalosClusterClient(di.NewMockInjector())
		client.shims = setupShims(t)
		return client
	}

	t.Run("Success", func(t *testing.T) {
		client := setup(t)
		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		healthy, healthyServices, unhealthyServices, err := client.getNodeHealthDetails(ctx, nodeAddress)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !healthy {
			t.Error("Expected node to be healthy")
		}
		if len(healthyServices) != 2 {
			t.Errorf("Expected 2 healthy services, got %d", len(healthyServices))
		}
		if len(unhealthyServices) != 0 {
			t.Errorf("Expected 0 unhealthy services, got %d", len(unhealthyServices))
		}
	})

	t.Run("ServiceListError", func(t *testing.T) {
		client := setup(t)
		client.shims.TalosServiceList = func(ctx context.Context, client *talosclient.Client) (*machine.ServiceListResponse, error) {
			return nil, fmt.Errorf("service list error")
		}

		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		_, _, _, err := client.getNodeHealthDetails(ctx, nodeAddress)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "service list error") {
			t.Errorf("Expected service list error, got %v", err)
		}
	})

	t.Run("UnhealthyServices", func(t *testing.T) {
		client := setup(t)
		client.shims.TalosServiceList = func(ctx context.Context, client *talosclient.Client) (*machine.ServiceListResponse, error) {
			return &machine.ServiceListResponse{
				Messages: []*machine.ServiceList{
					{
						Services: []*machine.ServiceInfo{
							{
								Id:    "apid",
								State: "Running",
								Health: &machine.ServiceHealth{
									Healthy: true,
								},
							},
							{
								Id:    "machined",
								State: "Stopped",
								Health: &machine.ServiceHealth{
									Healthy: false,
								},
							},
						},
					},
				},
			}, nil
		}

		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		healthy, healthyServices, unhealthyServices, err := client.getNodeHealthDetails(ctx, nodeAddress)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if healthy {
			t.Error("Expected node to be unhealthy")
		}
		if len(healthyServices) != 1 {
			t.Errorf("Expected 1 healthy service, got %d", len(healthyServices))
		}
		if len(unhealthyServices) != 1 {
			t.Errorf("Expected 1 unhealthy service, got %d", len(unhealthyServices))
		}
	})

	t.Run("ServiceWithNilHealth", func(t *testing.T) {
		client := setup(t)
		client.shims.TalosServiceList = func(ctx context.Context, client *talosclient.Client) (*machine.ServiceListResponse, error) {
			return &machine.ServiceListResponse{
				Messages: []*machine.ServiceList{
					{
						Services: []*machine.ServiceInfo{
							{
								Id:     "service-with-nil-health",
								State:  "Running",
								Health: nil, // nil health should be treated as unhealthy
							},
						},
					},
				},
			}, nil
		}

		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		healthy, healthyServices, unhealthyServices, err := client.getNodeHealthDetails(ctx, nodeAddress)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if healthy {
			t.Error("Expected node to be unhealthy")
		}
		if len(healthyServices) != 0 {
			t.Errorf("Expected 0 healthy services, got %d", len(healthyServices))
		}
		if len(unhealthyServices) != 1 {
			t.Errorf("Expected 1 unhealthy service, got %d", len(unhealthyServices))
		}
	})

	t.Run("ServiceNotRunning", func(t *testing.T) {
		client := setup(t)
		client.shims.TalosServiceList = func(ctx context.Context, client *talosclient.Client) (*machine.ServiceListResponse, error) {
			return &machine.ServiceListResponse{
				Messages: []*machine.ServiceList{
					{
						Services: []*machine.ServiceInfo{
							{
								Id:    "stopped-service",
								State: "Stopped",
								Health: &machine.ServiceHealth{
									Healthy: true, // healthy but not running should be unhealthy
								},
							},
						},
					},
				},
			}, nil
		}

		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		healthy, healthyServices, unhealthyServices, err := client.getNodeHealthDetails(ctx, nodeAddress)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if healthy {
			t.Error("Expected node to be unhealthy")
		}
		if len(healthyServices) != 0 {
			t.Errorf("Expected 0 healthy services, got %d", len(healthyServices))
		}
		if len(unhealthyServices) != 1 {
			t.Errorf("Expected 1 unhealthy service, got %d", len(unhealthyServices))
		}
	})
}

func TestTalosClusterClient_getNodeVersion(t *testing.T) {
	setup := func(t *testing.T) *TalosClusterClient {
		t.Helper()
		client := NewTalosClusterClient(di.NewMockInjector())
		client.shims = setupShims(t)
		return client
	}

	t.Run("Success", func(t *testing.T) {
		client := setup(t)
		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		version, err := client.getNodeVersion(ctx, nodeAddress)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if version != "1.0.0" {
			t.Errorf("Expected version '1.0.0', got '%s'", version)
		}
	})

	t.Run("VersionError", func(t *testing.T) {
		client := setup(t)
		client.shims.TalosVersion = func(ctx context.Context, client *talosclient.Client) (*machine.VersionResponse, error) {
			return nil, fmt.Errorf("version error")
		}

		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		_, err := client.getNodeVersion(ctx, nodeAddress)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "version error") {
			t.Errorf("Expected version error, got %v", err)
		}
	})

	t.Run("VersionWithoutPrefix", func(t *testing.T) {
		client := setup(t)
		client.shims.TalosVersion = func(ctx context.Context, client *talosclient.Client) (*machine.VersionResponse, error) {
			return &machine.VersionResponse{
				Messages: []*machine.Version{
					{
						Version: &machine.VersionInfo{
							Tag: "1.2.3",
						},
					},
				},
			}, nil
		}

		ctx := context.Background()
		nodeAddress := "10.0.0.1"

		version, err := client.getNodeVersion(ctx, nodeAddress)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if version != "1.2.3" {
			t.Errorf("Expected version '1.2.3', got '%s'", version)
		}
	})
}
