// The docker_virt_test package is a test suite for the DockerVirt ContainerRuntime implementation.
// It provides test coverage for Docker-native lifecycle behavior (Up, WriteConfig, Down).
// It verifies public method behavior only in a TDD fashion.

package virt

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupDockerVirt(t *testing.T, opts ...func(*VirtTestMocks)) (*VirtTestMocks, *DockerVirt) {
	t.Helper()
	mocks := setupVirtMocks(t, opts...)
	mocks.Runtime.ContextName = "mock-context"
	if err := mocks.ConfigHandler.SetContext("mock-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}
	dockerVirt := NewDockerVirt(mocks.Runtime)
	return mocks, dockerVirt
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewDockerVirt(t *testing.T) {
	t.Run("CreatesDockerVirtWithRuntime", func(t *testing.T) {
		// Given virt mocks
		mocks := setupVirtMocks(t)
		mocks.Runtime.ContextName = "mock-context"

		// When creating DockerVirt
		dockerVirt := NewDockerVirt(mocks.Runtime)

		// Then DockerVirt should be non-nil with BaseVirt set
		if dockerVirt == nil {
			t.Fatal("Expected non-nil DockerVirt")
		}
		if dockerVirt.BaseVirt == nil {
			t.Fatal("Expected non-nil BaseVirt")
		}
	})

	t.Run("PanicsWhenRuntimeNil", func(t *testing.T) {
		// Given no runtime (nil)

		// When/Then NewDockerVirt panics
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when runtime is nil")
			}
		}()
		NewDockerVirt(nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDockerVirt_Up(t *testing.T) {
	t.Run("SucceedsWithNoError", func(t *testing.T) {
		// Given a DockerVirt
		_, dockerVirt := setupDockerVirt(t)

		// When calling Up
		err := dockerVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestDockerVirt_WriteConfig(t *testing.T) {
	t.Run("SucceedsWithNoError", func(t *testing.T) {
		// Given a DockerVirt
		_, dockerVirt := setupDockerVirt(t)

		// When calling WriteConfig
		err := dockerVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestDockerVirt_Down(t *testing.T) {
	t.Run("WhenNetworkListFailsReturnsNil", func(t *testing.T) {
		// Given a DockerVirt with shell that fails on docker network ls
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "", fmt.Errorf("docker unavailable")
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then Down returns nil (best-effort)
		if err != nil {
			t.Errorf("Expected nil (best-effort), got %v", err)
		}
	})

	t.Run("WhenNoWindsorNetworksReturnsNil", func(t *testing.T) {
		// Given a DockerVirt with shell that returns non-Windsor networks only
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\nhost\nother_net\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then no error (no Windsor networks to clean)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("WhenWindsorNetworksExistCleansNetworksAndContainers", func(t *testing.T) {
		// Given a DockerVirt with shell that returns Windsor networks and inspect output
		var execCalls []string
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			call := command + " " + strings.Join(args, " ")
			execCalls = append(execCalls, call)
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\nwindsor-mock-context\n", nil
			}
			if command == "docker" && len(args) >= 3 && args[0] == "network" && args[1] == "inspect" {
				return "/c1 /c2 ", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And network inspect, stop, rm, network rm were invoked
		hasInspect := false
		hasStop := false
		hasRm := false
		hasNetworkRm := false
		for _, c := range execCalls {
			if strings.Contains(c, "network inspect") && strings.Contains(c, "windsor-mock-context") {
				hasInspect = true
			}
			if strings.Contains(c, "docker stop") {
				hasStop = true
			}
			if strings.Contains(c, "docker rm") && strings.Contains(c, "-f") && strings.Contains(c, "-v") {
				hasRm = true
			}
			if strings.Contains(c, "network rm") {
				hasNetworkRm = true
			}
		}
		if !hasInspect {
			t.Error("Expected docker network inspect to be called for Windsor network")
		}
		if !hasStop {
			t.Error("Expected docker stop to be called for containers")
		}
		if !hasRm {
			t.Error("Expected docker rm -f to be called for containers")
		}
		if !hasNetworkRm {
			t.Error("Expected docker network rm to be called")
		}
	})

	t.Run("WhenNetworkInspectFailsReturnsNil", func(t *testing.T) {
		// Given a DockerVirt with Windsor network but inspect fails
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "windsor-mock-context\n", nil
			}
			if command == "docker" && len(args) >= 3 && args[0] == "network" && args[1] == "inspect" {
				return "", fmt.Errorf("inspect failed")
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then Down returns nil (best-effort; inspect failure is logged, cleanup skipped for that network)
		if err != nil {
			t.Errorf("Expected nil (best-effort), got %v", err)
		}
	})
}
