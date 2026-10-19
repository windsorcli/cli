// The Shims are the system call abstractions of the secrets package.
// It provides the shared shims of internal/shims and the sops, 1Password, and command shims.
// Tests replace the shims to simulate decryption, 1Password lookups, and command output.
// NewShims returns the shims with the real implementations.

package secrets

import (
	"context"
	"fmt"
	"os/exec"

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
	CmdOutput            func(cmd *exec.Cmd) ([]byte, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation. DecryptFile runs
// sops and returns its stderr as the error text when sops exits with a failure.
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
		Command: exec.Command,
		CmdOutput: func(cmd *exec.Cmd) ([]byte, error) {
			return cmd.Output()
		},
	}
}
