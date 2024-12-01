package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	ctrl "github.com/windsor-hotel/cli/internal/controller"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper functions to create pointers for basic types
func ptrInt(i int) *int {
	return &i
}

// Helper function to capture stdout output
func captureStdout(f func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	return buf.String()
}

// Helper function to capture stderr output
func captureStderr(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	return buf.String()
}

// Mock exit function to capture exit code
var exitCode int

func mockExit(code int) {
	exitCode = code
}

type MockObjects struct {
	Controller     *ctrl.MockController
	Shell          *shell.MockShell
	EnvPrinter     *env.MockEnvPrinter
	ConfigHandler  *config.MockConfigHandler
	ContextHandler *context.MockContext
}

func TestRoot_Execute(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})
}
