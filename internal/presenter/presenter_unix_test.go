//go:build darwin || linux

package presenter_test

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/presenter"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestConsole_WatchInterrupts(t *testing.T) {
	t.Run("RestoresTheTerminalOnAGuardedInterrupt", func(t *testing.T) {
		// Given an interactive console with a running operation and a guarded interrupt watch
		term := &fakeTerminal{}
		c := presenter.NewConsole(&syncBuffer{}, presenter.ConsoleOptions{Interactive: true, Terminal: term})
		_, end := presenter.Begin(presenter.NewContext(context.Background(), c), "s", "applying")
		defer end(nil)
		stop := c.WatchInterrupts(func() bool { return true })
		defer stop()

		// When the process receives an interrupt
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			t.Fatalf("failed to send SIGINT: %v", err)
		}

		// Then the terminal leaves the live display mode and the process keeps running
		deadline := time.Now().Add(2 * time.Second)
		for term.String() != "enter,leave" && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if term.String() != "enter,leave" {
			t.Errorf("expected enter,leave, got %q", term.String())
		}
	})
}
