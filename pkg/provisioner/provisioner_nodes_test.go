package provisioner_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_UpgradeNode(t *testing.T) {
	t.Run("UpgradesNodeAndWaitsForRebootAndAPIReady", func(t *testing.T) {
		// Given a cluster client that accepts the upgrade and reports the node healthy
		mocks := setupProvisionerMocks(t)

		upgradeCalled := false
		mocks.ClusterClient.UpgradeNodesFunc = func(ctx context.Context, nodeAddresses []string, image string, powercycle bool) error {
			upgradeCalled = true
			if len(nodeAddresses) != 1 || nodeAddresses[0] != "10.0.0.1" {
				t.Errorf("Expected node 10.0.0.1, got %v", nodeAddresses)
			}
			return nil
		}
		rebootCalled := false
		var capturedVersion string
		mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
			rebootCalled = true
			capturedVersion = expectedVersion
			return nil
		}
		apiReadyCalled := false
		var capturedAPIReadyNode string
		mocks.ClusterClient.WaitForControlPlaneAPIReadyFunc = func(ctx context.Context, nodeAddress string, outputFunc func(string)) error {
			apiReadyCalled = true
			capturedAPIReadyNode = nodeAddress
			return nil
		}
		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When UpgradeNode runs for one node with a v1.7.0 installer image
		var output []string
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "ghcr.io/siderolabs/installer:v1.7.0", 0, false, false, func(msg string) {
			output = append(output, msg)
		})

		// Then the upgrade, reboot wait, and API readiness check run with the expected values
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !upgradeCalled {
			t.Error("Expected UpgradeNodes to be called")
		}
		if !rebootCalled {
			t.Error("Expected WaitForNodesReboot to be called")
		}
		if !apiReadyCalled {
			t.Error("Expected WaitForControlPlaneAPIReady to be called")
		}
		if capturedAPIReadyNode != "10.0.0.1" {
			t.Errorf("Expected api-ready node 10.0.0.1, got %q", capturedAPIReadyNode)
		}
		if len(output) == 0 {
			t.Error("Expected output messages, got none")
		}
		if capturedVersion != "1.7.0" {
			t.Errorf("Expected version 1.7.0 passed to WaitForNodesReboot, got: %q", capturedVersion)
		}
	})

	t.Run("DeletesStaleNodeAfterUpgrade", func(t *testing.T) {
		// Given a node that had a Kubernetes node name before the upgrade
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetNodeNamesByAddressFunc = func(ctx context.Context, address string) ([]string, error) {
			return []string{"worker-1-abc"}, nil
		}
		var gotAddress string
		var gotPrevious []string
		mocks.KubernetesManager.RemoveStaleNodesFunc = func(ctx context.Context, address string, previousNames []string) ([]string, error) {
			gotAddress, gotPrevious = address, previousNames
			return []string{"worker-1-abc"}, nil
		}
		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient, KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When UpgradeNode runs
		var output []string
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "img:v1.7.0", 0, false, false, func(msg string) { output = append(output, msg) })

		// Then the stale node cleanup runs with the address and the old names, and reports the deletion
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if gotAddress != "10.0.0.1" || len(gotPrevious) != 1 || gotPrevious[0] != "worker-1-abc" {
			t.Errorf("Unexpected cleanup arguments: %q %v", gotAddress, gotPrevious)
		}
		if !strings.Contains(strings.Join(output, "\n"), "Deleted stale Kubernetes node worker-1-abc") {
			t.Errorf("Expected a deletion message, got %v", output)
		}
	})

	t.Run("StaleNodeCleanupFailureDoesNotFailUpgrade", func(t *testing.T) {
		// Given a cleanup that fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetNodeNamesByAddressFunc = func(ctx context.Context, address string) ([]string, error) {
			return []string{"worker-1-abc"}, nil
		}
		mocks.KubernetesManager.RemoveStaleNodesFunc = func(ctx context.Context, address string, previousNames []string) ([]string, error) {
			return nil, fmt.Errorf("forbidden")
		}
		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient, KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When UpgradeNode runs
		var output []string
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "img:v1.7.0", 0, false, false, func(msg string) { output = append(output, msg) })

		// Then the upgrade succeeds and the failure is reported
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(strings.Join(output, "\n"), "Could not delete the stale Kubernetes node") {
			t.Errorf("Expected a cleanup warning, got %v", output)
		}
	})

	t.Run("SkipsCleanupWhenNodeHadNoKubernetesName", func(t *testing.T) {
		// Given a node with no Node object before the upgrade
		mocks := setupProvisionerMocks(t)
		called := false
		mocks.KubernetesManager.RemoveStaleNodesFunc = func(ctx context.Context, address string, previousNames []string) ([]string, error) {
			called = true
			return nil, nil
		}
		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient, KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When UpgradeNode runs
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "img:v1.7.0", 0, false, false, nil)

		// Then the cleanup does not run
		if err != nil || called {
			t.Errorf("Expected no cleanup call, got called=%v err=%v", called, err)
		}
	})

	t.Run("APIReadyFails", func(t *testing.T) {
		// Given a cluster client whose control plane API readiness check fails
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForControlPlaneAPIReadyFunc = func(ctx context.Context, nodeAddress string, outputFunc func(string)) error {
			return fmt.Errorf("connection refused")
		}

		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When UpgradeNode runs
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, false, nil)

		// Then a kube-apiserver readiness error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kube-apiserver readiness check failed") {
			t.Errorf("Expected apiserver error, got: %v", err)
		}
	})

	t.Run("UpgradeRequestFails", func(t *testing.T) {
		// Given a cluster client that rejects the upgrade request
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.UpgradeNodesFunc = func(ctx context.Context, nodeAddresses []string, image string, powercycle bool) error {
			return fmt.Errorf("upgrade request rejected")
		}

		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When UpgradeNode runs
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, false, nil)

		// Then the upgrade request error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "upgrade request rejected") {
			t.Errorf("Expected upgrade request error, got: %v", err)
		}
	})

	t.Run("RebootWaitFails", func(t *testing.T) {
		// Given a cluster client whose reboot wait times out
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
			return fmt.Errorf("timeout waiting for nodes to go offline")
		}

		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When UpgradeNode runs
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, false, nil)

		// Then a node reboot wait error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "node reboot wait failed") {
			t.Errorf("Expected reboot wait error, got: %v", err)
		}
	})

	t.Run("NoClusterClient", func(t *testing.T) {
		// Given a provisioner with no cluster driver and no cluster client
		mocks := setupProvisionerMocks(t)
		mockCfg := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCfg.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
		mockCfg.GetContextValuesFunc = func() (map[string]any, error) { return nil, nil }
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{})

		// When UpgradeNode runs
		err := prov.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, false, nil)

		// Then a missing cluster client error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no cluster client found") {
			t.Errorf("Expected no cluster client error, got: %v", err)
		}
	})
}

func TestProvisioner_CheckNodeHealth(t *testing.T) {
	t.Run("SuccessWithNodeCheckOnly", func(t *testing.T) {
		// Given a cluster client that reports two nodes healthy
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		opts := &provisioner.Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpointProvided: false,
		}

		// When CheckNodeHealth runs with only nodes
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then one message reports all nodes healthy
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "All 2 nodes are healthy") {
			t.Errorf("Expected output about healthy nodes, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithNodeCheckAndVersion", func(t *testing.T) {
		// Given a cluster client that expects version v1.5.0
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			if expectedVersion != "v1.5.0" {
				t.Errorf("Expected version 'v1.5.0', got: %q", expectedVersion)
			}
			return nil
		}
		opts := &provisioner.Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Version:             "v1.5.0",
			K8SEndpointProvided: false,
		}

		// When CheckNodeHealth runs with a node and a version
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then one message names the version
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "v1.5.0") {
			t.Errorf("Expected output about version, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithKubernetesCheckOnly", func(t *testing.T) {
		// Given a kubernetes manager that reports the endpoint healthy
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		// When CheckNodeHealth runs with only a Kubernetes endpoint
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then one message reports the endpoint healthy
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "healthy") {
			t.Errorf("Expected output about healthy endpoint, got: %q", outputMessages[0])
		}
	})

	t.Run("KubernetesCheckUsesTimeout", func(t *testing.T) {
		// Given a kubernetes manager that records the deadline of its context
		mocks := setupProvisionerMocks(t)
		var hasDeadline bool
		var remaining time.Duration
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			var deadline time.Time
			deadline, hasDeadline = ctx.Deadline()
			remaining = time.Until(deadline)
			return nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpoint:         "true",
			K8SEndpointProvided: true,
			Timeout:             2 * time.Second,
		}

		// When CheckNodeHealth runs with a timeout
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then the Kubernetes check gets a deadline no later than the timeout
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !hasDeadline {
			t.Fatal("Expected the Kubernetes check context to have a deadline")
		}
		if remaining <= 0 || remaining > 2*time.Second {
			t.Errorf("Expected the deadline within 2s, got %v", remaining)
		}
	})

	t.Run("SuccessWithBothChecks", func(t *testing.T) {
		// Given a cluster client and a kubernetes manager that both report healthy
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
			ClusterClient:     mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		// When CheckNodeHealth runs with nodes and a Kubernetes endpoint
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithNodeReadinessCheck", func(t *testing.T) {
		// Given a kubernetes manager that reports node 10.0.0.1 ready
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if len(nodeNames) != 1 || nodeNames[0] != "10.0.0.1" {
				t.Errorf("Expected node name '10.0.0.1', got: %v", nodeNames)
			}
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		// When CheckNodeHealth runs with the node readiness check
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNoHealthChecksSpecified", func(t *testing.T) {
		// Given a provisioner with no nodes and no Kubernetes endpoint
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpointProvided: false,
		}

		// When CheckNodeHealth runs
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then the error names the missing checks and suggests --k8s-endpoint
		if err == nil {
			t.Error("Expected error when no health checks specified")
		}

		if !strings.Contains(err.Error(), "no health checks specified") {
			t.Errorf("Expected error about no health checks, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "--k8s-endpoint") {
			t.Errorf("Expected a remediation that names --k8s-endpoint, got: %v", err)
		}
	})

	t.Run("ErrorClusterClientWaitForNodesHealthy", func(t *testing.T) {
		// Given a cluster client whose node health check fails
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return fmt.Errorf("cluster health check failed")
		}
		opts := &provisioner.Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpointProvided: false,
		}

		// When CheckNodeHealth runs with only nodes
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then a nodes health check error is returned
		if err == nil {
			t.Error("Expected error when cluster health check fails")
		}

		if !strings.Contains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected error about nodes failed health check, got: %v", err)
		}
	})

	t.Run("WarningClusterClientFailureWithK8sCheck", func(t *testing.T) {
		// Given a failing cluster client and a healthy kubernetes manager
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return fmt.Errorf("cluster health check failed")
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
			ClusterClient:     mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		// When CheckNodeHealth runs with nodes and a Kubernetes endpoint
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then no error is returned and a cluster client warning is written
		if err != nil {
			t.Errorf("Expected no error (cluster failure should be warning), got: %v", err)
		}

		warningFound := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "Warning") && strings.Contains(msg, "Cluster client failed") {
				warningFound = true
				break
			}
		}

		if !warningFound {
			t.Error("Expected warning message about cluster client failure")
		}
	})

	t.Run("ErrorKubernetesManagerWaitForKubernetesHealthy", func(t *testing.T) {
		// Given a kubernetes manager whose health check fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("kubernetes health check failed")
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		// When CheckNodeHealth runs with a Kubernetes endpoint
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then the Kubernetes health check error is returned
		if err == nil {
			t.Error("Expected error when Kubernetes health check fails")
		}

		if !strings.Contains(err.Error(), "kubernetes health check failed") {
			t.Errorf("Expected error about kubernetes health check, got: %v", err)
		}
	})

	t.Run("ErrorCheckNodeReadyRequiresNodes", func(t *testing.T) {
		// Given a node readiness check request without nodes
		mocks := setupProvisionerMocks(t)
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		// When CheckNodeHealth runs
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then the error says --ready requires --nodes and suggests adding them
		if err == nil {
			t.Error("Expected error when --ready flag used without --nodes")
		}

		if !strings.Contains(err.Error(), "--ready flag requires --nodes") {
			t.Errorf("Expected error about --ready requiring --nodes, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "Add --nodes") {
			t.Errorf("Expected a remediation that names Add --nodes, got: %v", err)
		}
	})

	t.Run("ErrorNoKubernetesManager", func(t *testing.T) {
		// Given a provisioner with no kubernetes manager
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		prov.KubernetesManager = nil

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		// When CheckNodeHealth runs with a Kubernetes endpoint
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then a missing kubernetes manager error is returned
		if err == nil {
			t.Error("Expected error when Kubernetes manager is nil")
		}

		if !strings.Contains(err.Error(), "no kubernetes manager found") {
			t.Errorf("Expected error about no kubernetes manager, got: %v", err)
		}
	})

	t.Run("SuccessWithDefaultTimeout", func(t *testing.T) {
		// Given a cluster client that requires a context deadline
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("Expected context to have deadline")
			}
			if deadline.IsZero() {
				t.Error("Expected non-zero deadline")
			}
			return nil
		}
		opts := &provisioner.Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Timeout:             5 * time.Minute,
			K8SEndpointProvided: false,
		}

		// When CheckNodeHealth runs with a five minute timeout
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithZeroTimeout", func(t *testing.T) {
		// Given a cluster client that reports nodes healthy
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		opts := &provisioner.Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Timeout:             0,
			K8SEndpointProvided: false,
		}

		// When CheckNodeHealth runs with a zero timeout
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithK8SEndpointTrue", func(t *testing.T) {
		// Given a kubernetes manager that expects an empty endpoint
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if endpoint != "" {
				t.Errorf("Expected empty endpoint when K8SEndpoint is 'true', got: %q", endpoint)
			}
			return nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpoint:         "true",
			K8SEndpointProvided: true,
		}

		// When CheckNodeHealth runs with the endpoint value true
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then one message reports the kubeconfig default endpoint
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "kubeconfig default") {
			t.Errorf("Expected output about kubeconfig default, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithNodeReadinessCheckAllReady", func(t *testing.T) {
		// Given a kubernetes manager that reports both nodes ready
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true, "10.0.0.2": true}, nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		// When CheckNodeHealth runs with the node readiness check
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then a message reports that all nodes are Ready
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundReadyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "all nodes are Ready") {
				foundReadyMessage = true
				break
			}
		}

		if !foundReadyMessage {
			t.Error("Expected output message about all nodes being Ready")
		}
	})

	t.Run("SuccessWithNodeReadinessCheckNotAllReady", func(t *testing.T) {
		// Given a kubernetes manager that reports the node not ready
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": false}, nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		// When CheckNodeHealth runs with the node readiness check
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then a message reports the endpoint healthy without all nodes Ready
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundHealthyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "is healthy") && !strings.Contains(msg, "all nodes are Ready") {
				foundHealthyMessage = true
				break
			}
		}

		if !foundHealthyMessage {
			t.Error("Expected output message about endpoint being healthy without all nodes Ready")
		}
	})

	t.Run("SuccessWithNodeReadinessCheckGetNodeReadyStatusError", func(t *testing.T) {
		// Given a kubernetes manager whose node ready status lookup fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return nil, fmt.Errorf("get node ready status failed")
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		// When CheckNodeHealth runs with the node readiness check
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then a message reports the endpoint healthy without all nodes Ready
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundHealthyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "is healthy") && !strings.Contains(msg, "all nodes are Ready") {
				foundHealthyMessage = true
				break
			}
		}

		if !foundHealthyMessage {
			t.Error("Expected output message about endpoint being healthy when GetNodeReadyStatus fails")
		}
	})

	t.Run("SuccessWithNodeReadinessCheckPartialReadyStatus", func(t *testing.T) {
		// Given a kubernetes manager that reports one of two nodes ready
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		// When CheckNodeHealth runs with the node readiness check
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then a message reports the endpoint healthy without all nodes Ready
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundHealthyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "is healthy") && !strings.Contains(msg, "all nodes are Ready") {
				foundHealthyMessage = true
				break
			}
		}

		if !foundHealthyMessage {
			t.Error("Expected output message about endpoint being healthy when not all nodes are ready")
		}
	})

	t.Run("ErrorNoHealthChecksWhenNodesProvidedButNoClusterClient", func(t *testing.T) {
		// Given no cluster driver and no cluster client, so node checks are unavailable
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		prov.ClusterClient = nil

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpointProvided: false,
		}

		// When CheckNodeHealth runs with only nodes
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then a no health checks error is returned
		if err == nil {
			t.Error("Expected error when nodes provided but no cluster client")
		}

		if !strings.Contains(err.Error(), "no health checks specified") {
			t.Errorf("Expected error about no health checks, got: %v", err)
		}
	})

	t.Run("SuccessWithK8SEndpointEmptyString", func(t *testing.T) {
		// Given a kubernetes manager that expects an empty endpoint
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if endpoint != "" {
				t.Errorf("Expected empty endpoint, got: %q", endpoint)
			}
			return nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			K8SEndpoint:         "",
			K8SEndpointProvided: true,
		}

		// When CheckNodeHealth runs with an empty Kubernetes endpoint
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then one message reports the kubeconfig default endpoint
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if len(outputMessages) != 1 {
			t.Errorf("Expected 1 output message, got: %d", len(outputMessages))
		}

		if !strings.Contains(outputMessages[0], "kubeconfig default") {
			t.Errorf("Expected output about kubeconfig default, got: %q", outputMessages[0])
		}
	})

	t.Run("SuccessWithK8SEndpointAndNodeReadinessCheckDefaultEndpoint", func(t *testing.T) {
		// Given a kubernetes manager that reports the node ready
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &provisioner.Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		// When CheckNodeHealth runs with an empty endpoint and the node readiness check
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then a message names the kubeconfig default and reports all nodes Ready
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		foundReadyMessage := false
		for _, msg := range outputMessages {
			if strings.Contains(msg, "kubeconfig default") && strings.Contains(msg, "all nodes are Ready") {
				foundReadyMessage = true
				break
			}
		}

		if !foundReadyMessage {
			t.Error("Expected output message about kubeconfig default and all nodes Ready")
		}
	})

	t.Run("SuccessWithWaitForReboot", func(t *testing.T) {
		// Given a cluster client that records reboot and health wait calls
		mocks := setupProvisionerMocks(t)
		rebootCalled := false
		mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
			rebootCalled = true
			return nil
		}
		healthyCalled := false
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			healthyCalled = true
			return nil
		}
		opts := &provisioner.Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := provisioner.NodeHealthCheckOptions{
			Nodes:         []string{"10.0.0.1"},
			WaitForReboot: true,
		}

		// When CheckNodeHealth runs with the reboot wait option
		err := prov.CheckNodeHealth(context.Background(), options, outputFunc)

		// Then only the reboot wait runs
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !rebootCalled {
			t.Error("Expected WaitForNodesReboot to be called")
		}
		if healthyCalled {
			t.Error("Expected WaitForNodesHealthy NOT to be called")
		}
	})

	t.Run("WaitForRebootError", func(t *testing.T) {
		// Given a cluster client whose reboot wait times out
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
			return fmt.Errorf("reboot timed out")
		}
		opts := &provisioner.Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := provisioner.NodeHealthCheckOptions{
			Nodes:         []string{"10.0.0.1"},
			WaitForReboot: true,
		}

		// When CheckNodeHealth runs with the reboot wait option
		err := prov.CheckNodeHealth(context.Background(), options, nil)

		// Then the reboot error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "reboot timed out") {
			t.Errorf("Expected reboot error, got: %v", err)
		}
	})
}

func TestProvisioner_UpgradeNode_SkipIfCurrent(t *testing.T) {
	const node = "10.0.0.1"
	const image = "ghcr.io/siderolabs/installer:v1.7.0"

	run := func(t *testing.T, skip bool, version string, versionErr error) (upgraded bool, output []string, err error) {
		t.Helper()
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.GetNodeVersionFunc = func(ctx context.Context, nodeAddress string) (string, error) {
			if nodeAddress != node {
				t.Errorf("Expected node %s, got %s", node, nodeAddress)
			}
			return version, versionErr
		}
		mocks.ClusterClient.UpgradeNodesFunc = func(ctx context.Context, nodeAddresses []string, image string, powercycle bool) error {
			upgraded = true
			return nil
		}
		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		err = prov.UpgradeNode(context.Background(), node, image, 0, false, skip, func(msg string) {
			output = append(output, msg)
		})
		return upgraded, output, err
	}

	t.Run("SkipsWhenVersionsMatch", func(t *testing.T) {
		// Given the flag is set and the node runs the image version
		// When UpgradeNode runs
		upgraded, output, err := run(t, true, "1.7.0", nil)

		// Then no upgrade request is sent and the output says the node is current
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if upgraded {
			t.Error("Expected no upgrade request")
		}
		if len(output) != 1 || !strings.Contains(output[0], "already at version 1.7.0") {
			t.Errorf("Expected an already-at-version message, got %v", output)
		}
	})

	t.Run("UpgradesWhenVersionsDiffer", func(t *testing.T) {
		// Given the flag is set and the node runs an older version
		// When UpgradeNode runs
		upgraded, _, err := run(t, true, "1.6.0", nil)

		// Then the upgrade request is sent
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !upgraded {
			t.Error("Expected an upgrade request")
		}
	})

	t.Run("UpgradesWhenVersionReadFails", func(t *testing.T) {
		// Given the flag is set and the version read fails
		// When UpgradeNode runs
		upgraded, _, err := run(t, true, "", fmt.Errorf("unreachable"))

		// Then the normal upgrade runs
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !upgraded {
			t.Error("Expected an upgrade request")
		}
	})

	t.Run("UpgradesWhenFlagUnset", func(t *testing.T) {
		// Given the flag is unset and the versions match
		// When UpgradeNode runs
		upgraded, _, err := run(t, false, "1.7.0", nil)

		// Then the upgrade request is still sent
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !upgraded {
			t.Error("Expected an upgrade request")
		}
	})
}

func TestProvisioner_UpgradeNode_ExpectedVersion(t *testing.T) {
	cases := []struct {
		image    string
		expected string
	}{
		{"ghcr.io/siderolabs/talos:v1.9.0", "1.9.0"},
		{"factory.talos.dev/installer/abc123:v1.9.0", "1.9.0"},
		{"registry:5000/image:v1.9.0", "1.9.0"},
		{"ghcr.io/siderolabs/talos:v1.9.0-beta.0", "1.9.0-beta.0"},
		{"image@sha256:abc123", ""},
		{"ghcr.io/siderolabs/talos", ""},
		{"registry:5000/image", ""},
	}
	for _, c := range cases {
		t.Run(c.image, func(t *testing.T) {
			// Given a cluster client that records the expected version of the reboot wait
			mocks := setupProvisionerMocks(t)
			var got string
			mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
				got = expectedVersion
				return nil
			}
			prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{ClusterClient: mocks.ClusterClient})

			// When UpgradeNode runs with the image
			if err := prov.UpgradeNode(context.Background(), "10.0.0.1", c.image, 0, false, false, nil); err != nil {
				t.Fatalf("UpgradeNode failed: %v", err)
			}

			// Then the reboot wait expects the version from the image tag
			if got != c.expected {
				t.Errorf("expected version %q, got %q", c.expected, got)
			}
		})
	}
}

func TestProvisioner_UpgradeNodes(t *testing.T) {
	type call struct {
		step  string
		nodes []string
	}

	setup := func(t *testing.T, controlPlanes ...string) (*provisioner.Provisioner, *[]call) {
		t.Helper()
		mocks := setupProvisionerMocks(t)
		calls := &[]call{}
		isCP := map[string]bool{}
		for _, n := range controlPlanes {
			isCP[n] = true
		}
		mocks.ClusterClient.IsControlPlaneNodeFunc = func(ctx context.Context, node string) (bool, error) {
			return isCP[node], nil
		}
		mocks.ClusterClient.UpgradeNodesFunc = func(ctx context.Context, nodes []string, image string, powercycle bool) error {
			*calls = append(*calls, call{"upgrade", nodes})
			return nil
		}
		mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodes []string, ids map[string]string, version string, skip []string, offline time.Duration) error {
			*calls = append(*calls, call{"reboot", nodes})
			return nil
		}
		mocks.ClusterClient.WaitForControlPlaneAPIReadyFunc = func(ctx context.Context, node string, out func(string)) error {
			*calls = append(*calls, call{"api", []string{node}})
			return nil
		}
		opts := &provisioner.Provisioner{ClusterClient: mocks.ClusterClient}
		return provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts), calls
	}

	format := func(calls []call) string {
		parts := make([]string, 0, len(calls))
		for _, c := range calls {
			parts = append(parts, c.step+":"+strings.Join(c.nodes, ","))
		}
		return strings.Join(parts, " ")
	}

	t.Run("RollsControlPlanesThenWorkersOneAtATime", func(t *testing.T) {
		// Given three control planes and one worker
		prov, calls := setup(t, "cp1", "cp2", "cp3")

		// When UpgradeNodes runs with the worker named first
		err := prov.UpgradeNodes(context.Background(), []string{"w1", "cp1", "cp2", "cp3"}, "img:v1.7.0", 0, time.Minute, false, false, nil)

		// Then each node finishes before the next starts and the worker rolls last
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		want := "upgrade:cp1 reboot:cp1 api:cp1 upgrade:cp2 reboot:cp2 api:cp2 upgrade:cp3 reboot:cp3 api:cp3 upgrade:w1 reboot:w1 api:w1"
		if got := format(*calls); got != want {
			t.Errorf("Expected calls %q, got %q", want, got)
		}
	})

	t.Run("StopsAtFirstControlPlaneFailure", func(t *testing.T) {
		// Given a second control plane whose reboot wait fails
		prov, calls := setup(t, "cp1", "cp2", "cp3")
		mocks := prov.ClusterClient.(*cluster.MockClusterClient)
		mocks.WaitForNodesRebootFunc = func(ctx context.Context, nodes []string, ids map[string]string, version string, skip []string, offline time.Duration) error {
			if nodes[0] == "cp2" {
				return fmt.Errorf("timeout")
			}
			return nil
		}

		// When UpgradeNodes runs
		err := prov.UpgradeNodes(context.Background(), []string{"cp1", "cp2", "cp3", "w1"}, "img", 0, time.Minute, false, false, nil)

		// Then the error names the node and no later node gets a request
		if err == nil || !strings.Contains(err.Error(), "cp2") {
			t.Fatalf("Expected an error naming cp2, got: %v", err)
		}
		for _, c := range *calls {
			if c.step == "upgrade" && (c.nodes[0] == "cp3" || c.nodes[0] == "w1") {
				t.Errorf("Expected no request to %v after the failure", c.nodes)
			}
		}
	})

	t.Run("RollsWorkersOneAtATime", func(t *testing.T) {
		// Given one control plane and two workers
		prov, calls := setup(t, "cp1")

		// When UpgradeNodes runs
		err := prov.UpgradeNodes(context.Background(), []string{"w1", "cp1", "w2"}, "img", 0, time.Minute, false, false, nil)

		// Then every node rolls alone, control plane first
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		want := "upgrade:cp1 reboot:cp1 api:cp1 upgrade:w1 reboot:w1 api:w1 upgrade:w2 reboot:w2 api:w2"
		if got := format(*calls); got != want {
			t.Errorf("Expected calls %q, got %q", want, got)
		}
	})

	t.Run("SkipsNodesAlreadyAtVersion", func(t *testing.T) {
		// Given two nodes where the first already runs the image version
		prov, calls := setup(t)
		mocks := prov.ClusterClient.(*cluster.MockClusterClient)
		mocks.GetNodeVersionFunc = func(ctx context.Context, node string) (string, error) {
			if node == "w1" {
				return "1.7.0", nil
			}
			return "1.6.0", nil
		}

		// When UpgradeNodes runs with skipIfCurrent
		var output []string
		err := prov.UpgradeNodes(context.Background(), []string{"w1", "w2"}, "img:v1.7.0", 0, time.Minute, false, true, func(msg string) {
			output = append(output, msg)
		})

		// Then only the older node rolls and the skip is reported
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if got := format(*calls); got != "upgrade:w2 reboot:w2 api:w2" {
			t.Errorf("Expected only w2 to roll, got %q", got)
		}
		if !strings.Contains(strings.Join(output, "\n"), "Node w1 is already at version 1.7.0") {
			t.Errorf("Expected a skip message, got %v", output)
		}
	})

	t.Run("AppliesNodeTimeoutToEachNode", func(t *testing.T) {
		// Given a per-node timeout
		prov, _ := setup(t)
		mocks := prov.ClusterClient.(*cluster.MockClusterClient)
		var deadlines int
		mocks.UpgradeNodesFunc = func(ctx context.Context, nodes []string, image string, powercycle bool) error {
			if _, ok := ctx.Deadline(); ok {
				deadlines++
			}
			return nil
		}

		// When UpgradeNodes runs for two nodes
		err := prov.UpgradeNodes(context.Background(), []string{"w1", "w2"}, "img", time.Minute, time.Second, false, false, nil)

		// Then every node request runs under its own deadline
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if deadlines != 2 {
			t.Errorf("Expected 2 requests with a deadline, got %d", deadlines)
		}
	})

	t.Run("RoleReadFails", func(t *testing.T) {
		// Given a node whose role cannot be read
		prov, calls := setup(t)
		mocks := prov.ClusterClient.(*cluster.MockClusterClient)
		mocks.IsControlPlaneNodeFunc = func(ctx context.Context, node string) (bool, error) {
			return false, fmt.Errorf("unreachable")
		}

		// When UpgradeNodes runs
		err := prov.UpgradeNodes(context.Background(), []string{"cp1", "cp2"}, "img", 0, time.Minute, false, false, nil)

		// Then it fails before any upgrade request
		if err == nil || !strings.Contains(err.Error(), "unreachable") {
			t.Fatalf("Expected a role error, got: %v", err)
		}
		if len(*calls) != 0 {
			t.Errorf("Expected no calls, got %v", *calls)
		}
	})
}
