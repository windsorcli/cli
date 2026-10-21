package provisioner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_UpgradeNode(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
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
		opts := &Provisioner{ClusterClient: mocks.ClusterClient}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var output []string
		err := provisioner.UpgradeNode(context.Background(), "10.0.0.1", "ghcr.io/siderolabs/installer:v1.7.0", 0, false, func(msg string) {
			output = append(output, msg)
		})

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

	t.Run("APIReadyFails", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForControlPlaneAPIReadyFunc = func(ctx context.Context, nodeAddress string, outputFunc func(string)) error {
			return fmt.Errorf("connection refused")
		}

		opts := &Provisioner{ClusterClient: mocks.ClusterClient}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, nil)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kube-apiserver readiness check failed") {
			t.Errorf("Expected apiserver error, got: %v", err)
		}
	})

	t.Run("UpgradeRequestFails", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.UpgradeNodesFunc = func(ctx context.Context, nodeAddresses []string, image string, powercycle bool) error {
			return fmt.Errorf("upgrade request rejected")
		}

		opts := &Provisioner{ClusterClient: mocks.ClusterClient}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, nil)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "upgrade request rejected") {
			t.Errorf("Expected upgrade request error, got: %v", err)
		}
	})

	t.Run("RebootWaitFails", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
			return fmt.Errorf("timeout waiting for nodes to go offline")
		}

		opts := &Provisioner{ClusterClient: mocks.ClusterClient}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, nil)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "node reboot wait failed") {
			t.Errorf("Expected reboot wait error, got: %v", err)
		}
	})

	t.Run("NoClusterClient", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockCfg := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCfg.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
		mockCfg.GetContextValuesFunc = func() (map[string]any, error) { return nil, nil }
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{})

		err := provisioner.UpgradeNode(context.Background(), "10.0.0.1", "img", 0, false, nil)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no cluster client found") {
			t.Errorf("Expected no cluster client error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "cluster.driver") {
			t.Errorf("Expected a remediation that names cluster.driver, got: %v", err)
		}
	})
}

func TestProvisioner_CheckNodeHealth(t *testing.T) {
	t.Run("SuccessWithNodeCheckOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			if expectedVersion != "v1.5.0" {
				t.Errorf("Expected version 'v1.5.0', got: %q", expectedVersion)
			}
			return nil
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Version:             "v1.5.0",
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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

	t.Run("SuccessWithBothChecks", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
			ClusterClient:     mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithNodeReadinessCheck", func(t *testing.T) {
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
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNoHealthChecksSpecified", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		options := NodeHealthCheckOptions{
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

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
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return fmt.Errorf("cluster health check failed")
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when cluster health check fails")
		}

		if !strings.Contains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected error about nodes failed health check, got: %v", err)
		}
	})

	t.Run("WarningClusterClientFailureWithK8sCheck", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return fmt.Errorf("cluster health check failed")
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
			ClusterClient:     mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("kubernetes health check failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when Kubernetes health check fails")
		}

		if !strings.Contains(err.Error(), "kubernetes health check failed") {
			t.Errorf("Expected error about kubernetes health check, got: %v", err)
		}
	})

	t.Run("ErrorCheckNodeReadyRequiresNodes", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

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
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.KubernetesManager = nil

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when Kubernetes manager is nil")
		}

		if !strings.Contains(err.Error(), "no kubernetes manager found") {
			t.Errorf("Expected error about no kubernetes manager, got: %v", err)
		}
	})

	t.Run("SuccessWithDefaultTimeout", func(t *testing.T) {
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
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Timeout:             5 * time.Minute,
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithZeroTimeout", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
			return nil
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			Timeout:             0,
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessWithK8SEndpointTrue", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if endpoint != "" {
				t.Errorf("Expected empty endpoint when K8SEndpoint is 'true', got: %q", endpoint)
			}
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "true",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true, "10.0.0.2": true}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": false}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return nil, fmt.Errorf("get node ready status failed")
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1", "10.0.0.2"},
			K8SEndpoint:         "https://test:6443",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "" // No cluster driver set, so ClusterClient won't be created
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		provisioner.ClusterClient = nil

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpointProvided: false,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error when nodes provided but no cluster client")
		}

		if !strings.Contains(err.Error(), "no health checks specified") {
			t.Errorf("Expected error about no health checks, got: %v", err)
		}
	})

	t.Run("SuccessWithK8SEndpointEmptyString", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			if endpoint != "" {
				t.Errorf("Expected empty endpoint, got: %q", endpoint)
			}
			return nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			K8SEndpoint:         "",
			K8SEndpointProvided: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		mocks.KubernetesManager.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"10.0.0.1": true}, nil
		}
		opts := &Provisioner{
			KubernetesManager: mocks.KubernetesManager,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:               []string{"10.0.0.1"},
			K8SEndpoint:         "",
			K8SEndpointProvided: true,
			CheckNodeReady:      true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		var outputMessages []string
		outputFunc := func(msg string) {
			outputMessages = append(outputMessages, msg)
		}

		options := NodeHealthCheckOptions{
			Nodes:         []string{"10.0.0.1"},
			WaitForReboot: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, outputFunc)

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
		mocks := setupProvisionerMocks(t)
		mocks.ClusterClient.WaitForNodesRebootFunc = func(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
			return fmt.Errorf("reboot timed out")
		}
		opts := &Provisioner{
			ClusterClient: mocks.ClusterClient,
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		options := NodeHealthCheckOptions{
			Nodes:         []string{"10.0.0.1"},
			WaitForReboot: true,
		}

		err := provisioner.CheckNodeHealth(context.Background(), options, nil)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "reboot timed out") {
			t.Errorf("Expected reboot error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestVersionFromImage(t *testing.T) {
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
		got := versionFromImage(c.image)
		if got != c.expected {
			t.Errorf("versionFromImage(%q) = %q, want %q", c.image, got, c.expected)
		}
	}
}
