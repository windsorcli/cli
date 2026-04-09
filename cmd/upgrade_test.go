package cmd

import (
	"bytes"
	stdcontext "context"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

func TestUpgradeNodeCmd(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(stdcontext.Background())
		upgradeNodeAddr = ""
		upgradeNodeImage = ""
		upgradeNodeTimeout = 0
	})

	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		upgradeNodeAddr = ""
		upgradeNodeImage = ""
		upgradeNodeTimeout = 0

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("MissingNodeFlag", func(t *testing.T) {
		setup(t)
		rootCmd.SetArgs([]string{"upgrade", "node", "--image", "img"})

		err := Execute()

		if err == nil {
			t.Error("Expected error for missing --node flag, got nil")
		}
	})

	t.Run("MissingImageFlag", func(t *testing.T) {
		setup(t)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1"})

		err := Execute()

		if err == nil {
			t.Error("Expected error for missing --image flag, got nil")
		}
	})

	t.Run("CheckTrustedDirectoryError", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return stdcontext.DeadlineExceeded
		}

		ctx := stdcontext.WithValue(stdcontext.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1", "--image", "img"})

		err := Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return false }
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		ctx := stdcontext.WithValue(stdcontext.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1", "--image", "img"})

		err := Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Nothing to upgrade") {
			t.Errorf("Expected not-loaded error, got: %v", err)
		}
	})

	t.Run("UpgradeNodeError", func(t *testing.T) {
		setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		ctx := stdcontext.WithValue(stdcontext.Background(), runtimeOverridesKey, mocks.Runtime)
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{"upgrade", "node", "--node", "10.0.0.1", "--image", "img"})

		err := Execute()

		// TALOSCONFIG not set — confirms upgrade node code path was reached
		if err == nil {
			t.Error("Expected error (no TALOSCONFIG), got nil")
		}
		if !strings.Contains(err.Error(), "node upgrade failed") {
			t.Errorf("Expected node upgrade error, got: %v", err)
		}
	})

}
