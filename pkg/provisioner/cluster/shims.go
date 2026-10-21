// The Shims are the system call and Talos client abstractions of the cluster clients.
// It provides the shared shims of internal/shims, the Talos client operations, and the network dial.
// Tests replace the shims to simulate Talos nodes and the environment.
// NewShims returns the shims with the real implementations.

package cluster

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the Talos client operations.
type Shims struct {
	shims.Shims

	// Talos client operations
	TalosConfigOpen  func(configPath string) (*clientconfig.Config, error)
	TalosNewClient   func(ctx context.Context, opts ...client.OptionFunc) (*client.Client, error)
	TalosVersion     func(ctx context.Context, client *client.Client) (*machine.VersionResponse, error)
	TalosWithNodes   func(ctx context.Context, nodes ...string) context.Context
	TalosServiceList func(ctx context.Context, client *client.Client) (*machine.ServiceListResponse, error)
	TalosUpgrade     func(ctx context.Context, client *client.Client, image string, powercycle bool) error
	TalosRead        func(ctx context.Context, client *client.Client, path string) (io.ReadCloser, error)
	TalosClose       func(client *client.Client)

	// Network operations
	NetDialTimeout func(network, address string, timeout time.Duration) (net.Conn, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims: *shims.NewShims(),

		TalosConfigOpen: clientconfig.Open,
		TalosNewClient: func(ctx context.Context, opts ...client.OptionFunc) (*client.Client, error) {
			return client.New(ctx, opts...)
		},
		TalosVersion: func(ctx context.Context, c *client.Client) (*machine.VersionResponse, error) {
			return c.Version(ctx)
		},
		TalosWithNodes: client.WithNodes,
		TalosServiceList: func(ctx context.Context, c *client.Client) (*machine.ServiceListResponse, error) {
			return c.ServiceList(ctx)
		},
		TalosUpgrade: func(ctx context.Context, c *client.Client, image string, powercycle bool) error {
			opts := []client.UpgradeOption{client.WithUpgradeImage(image)}
			if powercycle {
				opts = append(opts, client.WithUpgradeRebootMode(machine.UpgradeRequest_POWERCYCLE))
			}
			resp, err := c.UpgradeWithOptions(ctx, opts...)
			if err != nil {
				return err
			}
			if len(resp.GetMessages()) == 0 {
				return fmt.Errorf("upgrade request returned no acknowledgment from the node")
			}
			return nil
		},
		TalosRead: func(ctx context.Context, c *client.Client, path string) (io.ReadCloser, error) {
			return c.Read(ctx, path)
		},
		TalosClose: func(c *client.Client) {
			_ = c.Close()
		},
		NetDialTimeout: net.DialTimeout,
	}
}
