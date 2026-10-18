// The handler_schema file holds the schema and validation steps of the ConfigHandler.
// It provides schema loading, sensitive path lookup, and validation of context values.
// It merges the CLI fragment over every loaded schema fragment.
// It checks the context id against DNS label rules, with or without a schema.

package config

import (
	"fmt"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// contextIDRemediation is the hint for a context id that is not a valid DNS label.
const contextIDRemediation = "Change id in values.yaml or in the --set flag, or remove it so that windsor generates one."

// =============================================================================
// Public Methods
// =============================================================================

// GetSchema returns a shallow copy of the loaded schema map, or nil if no schema is loaded.
// A copy is returned so callers cannot mutate the validator's internal schema state.
func (c *configHandler) GetSchema() map[string]any {
	if c.schemaValidator == nil {
		return nil
	}
	src := c.schemaValidator.Schema
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// GetSensitivePaths returns the dotted config paths marked `sensitive: true` in the loaded
// schema, or nil if no schema is loaded. Consumers scrub these values from plaintext output
// and gate which properties may back a cluster Secret.
func (c *configHandler) GetSensitivePaths() []string {
	if c.schemaValidator == nil {
		return nil
	}
	return c.schemaValidator.GetSensitivePaths()
}

// IsSensitivePath reports whether the given dotted config path is marked sensitive in the loaded
// schema. A "*" segment in a schema path (from a free-form map region) matches any single
// segment of the queried path.
func (c *configHandler) IsSensitivePath(path string) bool {
	for _, sensitive := range c.GetSensitivePaths() {
		if sensitivePathMatches(sensitive, path) {
			return true
		}
	}
	return false
}

// LoadSchema reads the schema file at schemaPath and loads it as LoadSchemaFromBytes does.
// Returns an error if the file cannot be read or the schema is invalid.
func (c *configHandler) LoadSchema(schemaPath string) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("runtime not fully initialized: schema validator missing")
	}
	schemaContent, err := c.shims.ReadFile(schemaPath)
	if err != nil {
		return werror.Wrap(err, "failed to read schema file %s", schemaPath)
	}
	return c.LoadSchemaFromBytes(schemaContent)
}

// LoadSchemaFromBytes merges a schema fragment into the loaded schema, then merges the CLI
// fragment over the result. It first rejects a fragment that gives a CLI-owned path a different
// type, and leaves the loaded schema unchanged in that case. After the merge, values that Set
// converted from strings convert again against the new schema.
func (c *configHandler) LoadSchemaFromBytes(schemaContent []byte) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("runtime not fully initialized: schema validator missing")
	}
	if err := checkCLIOwnedTypes(schemaContent); err != nil {
		return err
	}
	if err := c.schemaValidator.LoadSchemaFromBytes(schemaContent); err != nil {
		return err
	}
	if err := c.applyCLISchema(); err != nil {
		return err
	}
	c.reconvertStringSets()
	return nil
}

// ValidateContextValues runs schema validation on the full context configuration map. Cross-field
// rules spanning header and dynamic fields are evaluated against the unified map,
// matching the file-load validation path. Use before impactful operations (e.g. windsor up) and after
// compositional Set sequences. Returns nil if no schema is loaded or if validation passes; returns an
// error if validation fails. When the same invalid data was already warned about by valuesSource.Load
// earlier in this invocation, the error refers back to that warning instead of re-printing the
// identical error list a second time. It also checks the id field. This check runs even with no
// schema loaded, so an explicit --set id=... or a checked-in values.yaml fails immediately.
func (c *configHandler) ValidateContextValues() error {
	if err := validateContextID(c.GetString("id")); err != nil {
		return err
	}

	if c.schemaValidator == nil || c.schemaValidator.Schema == nil {
		return nil
	}
	if result, err := c.schemaValidator.Validate(c.data); err != nil {
		return werror.Wrap(err, "error validating context values")
	} else if !result.Valid {
		if c.schemaValidator.ErrorsAlreadyReported(result.Errors) {
			return werror.New("Context values do not match the schema. The values.yaml warning above lists the errors.",
				"Correct the values in values.yaml.", nil)
		}
		return werror.New(fmt.Sprintf("Context values do not match the schema:%s", FormatValidationErrors(result.Errors)),
			"Correct the values listed above in values.yaml or in the --set flags.", nil)
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// validateContextID checks id against DNS label rules (RFC 1123): a lowercase letter, then any
// mix of lowercase letters, digits, and hyphens, with no leading, trailing, or repeated hyphen.
// This keeps a user-supplied id safe to embed in a domain name (e.g. id.local). An empty id is
// valid; GenerateContextID fills it in later.
func validateContextID(id string) error {
	if id == "" {
		return nil
	}
	if id[0] < 'a' || id[0] > 'z' {
		return werror.New(fmt.Sprintf("Context id %q must start with a lowercase letter (a-z).", id), contextIDRemediation, nil)
	}
	if id[len(id)-1] == '-' {
		return werror.New(fmt.Sprintf("Context id %q must not end with a hyphen.", id), contextIDRemediation, nil)
	}
	for i := 1; i < len(id); i++ {
		c := id[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if !isLower && !isDigit && c != '-' {
			return werror.New(fmt.Sprintf("Context id %q must contain only lowercase letters, digits, and hyphens.", id), contextIDRemediation, nil)
		}
		if c == '-' && id[i-1] == '-' {
			return werror.New(fmt.Sprintf("Context id %q must not repeat a hyphen.", id), contextIDRemediation, nil)
		}
	}
	return nil
}
