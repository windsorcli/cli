package secrets

import (
	"context"
	"errors"
	"os"

	"github.com/1password/onepassword-sdk-go"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/goccy/go-yaml"
)

// stat is a shim for os.Stat to allow for easier testing and mocking.
var stat = os.Stat

// yamlUnmarshal is a shim for yaml.Unmarshal to allow for easier testing and mocking.
var yamlUnmarshal = yaml.Unmarshal

// decryptFileFunc is a shim for decrypt.File to allow for easier testing and mocking.
var decryptFileFunc = decrypt.File

// newOnePasswordClient is a shim for onepassword.NewClient to allow for easier testing and mocking.
var newOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
	return onepassword.NewClient(ctx, opts...)
}

// resolveSecret is a shim for client.Secrets().Resolve to allow for easier testing and mocking.
var resolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
	if client == nil {
		return "", errors.New("client is nil")
	}
	return client.Secrets().Resolve(ctx, secretRef)
}
