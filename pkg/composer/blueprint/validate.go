// The ValidateComposedBlueprint function checks the structure of a composed blueprint.
// It checks that the backend field names a declared terraform component.
// It checks that each kubernetes.io/tls secret has the tls.crt and tls.key data keys after facets merge.
// A failure wraps ErrBlueprintInvalid, which callers detect with errors.Is.

package blueprint

import (
	"errors"
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Errors
// =============================================================================

// ErrBlueprintInvalid is the sentinel that wraps a structural error in a composed blueprint.
var ErrBlueprintInvalid = errors.New("invalid blueprint")

// =============================================================================
// Helpers
// =============================================================================

// ValidateComposedBlueprint returns an error if the backend field names a terraform component that does not exist,
// or if a kubernetes.io/tls secret does not have both the tls.crt and tls.key data keys. It accepts a nil blueprint
// and an empty backend field. The error has a remediation hint and wraps ErrBlueprintInvalid.
func ValidateComposedBlueprint(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return nil
	}
	if err := validateTLSSecrets(blueprint); err != nil {
		return err
	}
	if blueprint.Backend == "" {
		return nil
	}

	for i := range blueprint.TerraformComponents {
		if blueprint.TerraformComponents[i].GetID() == blueprint.Backend {
			return nil
		}
	}

	return werror.New(
		fmt.Sprintf("invalid blueprint: backend names terraform component %q but no component with that ID is declared", blueprint.Backend),
		"Set backend to the ID of an existing terraform component, or remove the backend field to use no in-blueprint backend.",
		ErrBlueprintInvalid,
	)
}

// validateTLSSecrets returns an error for the first kubernetes.io/tls secret of a flux system that does not have
// both the tls.crt and tls.key data keys. It runs on merged secrets, so a facet can supply one key and another
// facet the other.
func validateTLSSecrets(blueprint *blueprintv1alpha1.Blueprint) error {
	for _, system := range blueprint.FluxSystems {
		for secretName, entry := range system.Secrets {
			if entry.Type != secretTypeTLS {
				continue
			}
			for _, key := range []string{"tls.crt", "tls.key"} {
				if _, ok := entry.Data[key]; !ok {
					return werror.New(
						fmt.Sprintf("invalid blueprint: secret %q in flux system %q has type %s but no %s data key", secretName, system.Name, secretTypeTLS, key),
						"Set both tls.crt and tls.key under data:.",
						ErrBlueprintInvalid,
					)
				}
			}
		}
	}
	return nil
}
