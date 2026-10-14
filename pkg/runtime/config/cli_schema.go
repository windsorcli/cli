package config

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
)

// The CLISchema is the schema fragment the CLI owns, applied to every merged context schema.
// It provides the merge of the CLI fragment after every other fragment, and a type check
// that runs on each incoming fragment before it merges. A fragment may define a path the CLI
// owns only with the same type, so the CLI can always interpret the values it reads.

// cliSchemaTree caches the decoded CLI fragment for checkCLIOwnedTypes.
var cliSchemaTree = sync.OnceValues(func() (map[string]any, error) {
	var tree map[string]any
	if err := yaml.Unmarshal(cliconfig.Schema(), &tree); err != nil {
		return nil, fmt.Errorf("error decoding the CLI schema: %w", err)
	}
	return tree, nil
})

// =============================================================================
// Private Methods
// =============================================================================

// applyCLISchema merges the CLI fragment over the loaded schema, so the CLI's types win on every
// path it owns. It does nothing when no schema is loaded.
func (c *configHandler) applyCLISchema() error {
	if c.schemaValidator == nil || c.schemaValidator.Schema == nil {
		return nil
	}
	if err := c.schemaValidator.LoadSchemaFromBytes(cliconfig.Schema()); err != nil {
		return fmt.Errorf("error applying the CLI schema: %w", err)
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// checkCLIOwnedTypes returns an error that names every path where schemaContent declares a type
// that the CLI fragment does not allow. A path with no declared type, such as a $ref, passes. A
// type list passes when it includes the CLI's type. Content that does not decode is left to the
// schema validator to report.
func checkCLIOwnedTypes(schemaContent []byte) error {
	var incoming map[string]any
	if err := yaml.Unmarshal(schemaContent, &incoming); err != nil || incoming == nil {
		return nil
	}
	tree, err := cliSchemaTree()
	if err != nil {
		return err
	}
	var conflicts []string
	collectTypeConflicts(tree, incoming, "", &conflicts)
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("schema redefines settings the Windsor CLI owns:%s", FormatValidationErrors(conflicts))
}

// collectTypeConflicts walks the CLI node and the incoming node together and appends one line per
// path whose incoming type the CLI type does not match. It follows properties and the
// additionalProperties schema of free-form maps.
func collectTypeConflicts(cli, incoming map[string]any, path string, conflicts *[]string) {
	if want, ok := cli["type"].(string); ok {
		if declared, ok := incoming["type"]; ok && !typeAllows(declared, want) {
			*conflicts = append(*conflicts, fmt.Sprintf("%s: type %s, but the CLI requires %s", path, describeType(declared), want))
			return
		}
	}
	cliProperties, _ := cli["properties"].(map[string]any)
	incomingProperties, _ := incoming["properties"].(map[string]any)
	for name, cliChild := range cliProperties {
		cliChildMap, cliOK := cliChild.(map[string]any)
		incomingChild, incomingOK := incomingProperties[name].(map[string]any)
		if cliOK && incomingOK {
			collectTypeConflicts(cliChildMap, incomingChild, joinSchemaPath(path, name), conflicts)
		}
	}
	cliItems, cliOK := cli["additionalProperties"].(map[string]any)
	incomingItems, incomingOK := incoming["additionalProperties"].(map[string]any)
	if cliOK && incomingOK {
		collectTypeConflicts(cliItems, incomingItems, joinSchemaPath(path, "*"), conflicts)
	}
}

// typeAllows reports whether a declared schema type, a name or a list of names, includes want.
func typeAllows(declared any, want string) bool {
	switch t := declared.(type) {
	case string:
		return t == want
	case []any:
		for _, name := range t {
			if name == want {
				return true
			}
		}
	}
	return false
}

// describeType renders a declared schema type for an error message.
func describeType(declared any) string {
	if list, ok := declared.([]any); ok {
		names := make([]string, 0, len(list))
		for _, name := range list {
			names = append(names, fmt.Sprint(name))
		}
		return "[" + strings.Join(names, ", ") + "]"
	}
	return fmt.Sprint(declared)
}

// joinSchemaPath appends name to a dotted schema path.
func joinSchemaPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
