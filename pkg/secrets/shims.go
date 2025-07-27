package secrets

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/1password/onepassword-sdk-go"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/goccy/go-yaml"
)

// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	Stat                 func(string) (os.FileInfo, error)
	YAMLUnmarshal        func([]byte, any) error
	DecryptFile          func(string, string) ([]byte, error)
	NewOnePasswordClient func(context.Context, ...onepassword.ClientOption) (*onepassword.Client, error)
	ResolveSecret        func(*onepassword.Client, context.Context, string) (string, error)
	Command              func(name string, arg ...string) *exec.Cmd
	CmdOutput            func(cmd *exec.Cmd) ([]byte, error)
}

// =============================================================================
// Shims
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Stat:          os.Stat,
		YAMLUnmarshal: yaml.Unmarshal,
		DecryptFile:   decrypt.File,
		NewOnePasswordClient: func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return onepassword.NewClient(ctx, opts...)
		},
		ResolveSecret: func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
			if client == nil {
				return "", errors.New("client is nil")
			}
			return client.Secrets().Resolve(ctx, secretRef)
		},
		Command: exec.Command,
		CmdOutput: func(cmd *exec.Cmd) ([]byte, error) {
			return cmd.Output()
		},
	}
}
