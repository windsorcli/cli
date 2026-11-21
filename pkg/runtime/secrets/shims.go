// The Shims are the system call abstractions of the secrets package.
// It provides the shared shims of internal/shims and the sops, 1Password, and command shims.
// Tests replace the shims to simulate decryption, 1Password lookups, and command output.
// NewShims returns the shims with the real implementations.

package secrets

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"github.com/1password/onepassword-sdk-go"

	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the secrets-specific provider shims.
type Shims struct {
	shims.Shims
	DecryptFile          func(string, string) ([]byte, error)
	NewOnePasswordClient func(context.Context, ...onepassword.ClientOption) (*onepassword.Client, error)
	ResolveSecret        func(*onepassword.Client, context.Context, string) (string, error)
	Command              func(name string, arg ...string) *exec.Cmd
	CmdOutput            func(ctx context.Context, cmd *exec.Cmd) (stdout []byte, stderr []byte, err error)
	ListVaults           func(*onepassword.Client, context.Context) error
	WatchInterrupt       func() (<-chan struct{}, func())
	HintWriter           io.Writer
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation. DecryptFile runs
// sops and returns its stderr as the error text when sops exits with a failure.
// CmdOutput runs a command under a context and returns its stdout and stderr separately.
// WatchInterrupt reports an interrupt signal on its channel until its stop function runs.
func NewShims() *Shims {
	return &Shims{
		Shims: *shims.NewShims(),
		DecryptFile: func(filePath string, format string) ([]byte, error) {
			cmd := exec.Command("sops", "--decrypt", "--input-type", format, filePath) // #nosec G204 -- sops is a known, trusted CLI tool
			output, err := cmd.Output()
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					if len(exitError.Stderr) > 0 {
						return nil, fmt.Errorf("sops decryption failed: %s", string(exitError.Stderr))
					}
					return nil, fmt.Errorf("sops decryption failed with exit code %d", exitError.ExitCode())
				}
				return nil, werror.Wrap(err, "failed to execute sops")
			}
			return output, nil
		},
		NewOnePasswordClient: func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return onepassword.NewClient(ctx, opts...)
		},
		ResolveSecret: func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
			if client == nil {
				return "", fmt.Errorf("client is nil")
			}
			return client.Secrets().Resolve(ctx, secretRef)
		},
		ListVaults: func(client *onepassword.Client, ctx context.Context) error {
			if client == nil {
				return fmt.Errorf("client is nil")
			}
			_, err := client.Vaults().List(ctx)
			return err
		},
		Command: exec.Command,
		CmdOutput: func(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				return nil, nil, err
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				return stdout.Bytes(), stderr.Bytes(), err
			case <-ctx.Done():
				_ = cmd.Process.Kill()
				<-done
				return nil, stderr.Bytes(), ctx.Err()
			}
		},
		WatchInterrupt: func() (<-chan struct{}, func()) {
			sigCh := make(chan os.Signal, 1)
			out := make(chan struct{}, 1)
			signal.Notify(sigCh, os.Interrupt)
			stop := make(chan struct{})
			go func() {
				select {
				case <-sigCh:
					out <- struct{}{}
				case <-stop:
				}
			}()
			return out, func() {
				signal.Stop(sigCh)
				close(stop)
			}
		},
		HintWriter: os.Stderr,
	}
}
