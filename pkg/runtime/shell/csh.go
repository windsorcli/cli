// The csh rendering prints environment variables and aliases in tcsh syntax.
// It provides the output that the tcsh hook evaluates on each prompt.
// Each command ends in a semicolon, because tcsh joins the lines of a command substitution into one line.

package shell

import (
	"fmt"
	"maps"
	"sort"
	"strings"
)

// =============================================================================
// Public Methods
// =============================================================================

// RenderCsh returns envVars as setenv commands and aliases as alias commands in tcsh syntax.
// An empty value gives unsetenv or unalias. Keys are in sorted order, env vars first.
// A newline in a value becomes a space, because a tcsh command substitution cannot keep it.
func RenderCsh(envVars, aliases map[string]string) string {
	var result strings.Builder
	for _, k := range sortedKeys(envVars) {
		if envVars[k] == "" {
			fmt.Fprintf(&result, "unsetenv %s;\n", k)
		} else {
			fmt.Fprintf(&result, "setenv %s %s;\n", k, quoteCsh(envVars[k]))
		}
	}
	for _, k := range sortedKeys(aliases) {
		if aliases[k] == "" {
			fmt.Fprintf(&result, "unalias %s;\n", k)
		} else {
			fmt.Fprintf(&result, "alias %s %s;\n", k, quoteCsh(aliases[k]))
		}
	}
	return result.String()
}

// WithRemoved returns a copy of values with an empty value for each name in names that values does not have.
// RenderCsh then prints unsetenv or unalias for these names.
func WithRemoved(values map[string]string, names []string) map[string]string {
	result := make(map[string]string, len(values)+len(names))
	maps.Copy(result, values)
	for _, name := range names {
		if _, ok := result[name]; !ok {
			result[name] = ""
		}
	}
	return result
}

// =============================================================================
// Helpers
// =============================================================================

// sortedKeys returns the keys of m in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// quoteCsh puts value in single quotes for tcsh. It escapes single quotes and the history character.
func quoteCsh(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "'", `'\''`)
	value = strings.ReplaceAll(value, "!", `\!`)
	return "'" + value + "'"
}
