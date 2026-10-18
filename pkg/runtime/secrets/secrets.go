// The Resolver is the entry point of the secrets package.
// It provides the secret() helper, which resolves a reference through the first provider that handles its vault.
// It normalizes every supported secret notation to one SecretRef before resolution.
// It registers each resolved value with the shell so that output scrubbing hides it.

package secrets

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// secretArgsRemediation is the hint for a secret() call with arguments of the wrong number or type.
const secretArgsRemediation = `Call secret() with string arguments in the order vault, item, field. The field is optional.`

// =============================================================================
// Vars
// =============================================================================

var version = "dev"

var legacyBracePattern = regexp.MustCompile(`\${{\s*(.*?)\s*}}`)

// exactSecretCallPattern matches exactly secret("vault", "item") or secret("vault", "item", "field")
// with no surrounding operators.
// Each argument may contain any characters except unescaped double-quotes.
var exactSecretCallPattern = regexp.MustCompile(`^secret\(\s*"[^"]*"\s*,\s*"[^"]*"\s*(,\s*"[^"]*"\s*)?\)$`)

// =============================================================================
// Types
// =============================================================================

// SecretRef is the canonical internal representation of a secret reference.
// All notation formats normalize to this before resolution.
type SecretRef struct {
	Vault string // "sops" or a 1Password vault ID
	Item  string // item name or SOPS key path
	Field string // field name (empty for SOPS)
}

// OnePasswordVault identifies the 1Password vault that a 1Password provider reads. ID is the alias
// that secret() references, Name is the vault name, and URL is the account.
type OnePasswordVault struct {
	ID   string
	Name string
	URL  string
}

// DeferredError signals that a secret expression should be preserved for later
// evaluation. This is not an error condition but a control-flow signal.
type DeferredError struct {
	Expression string
	Message    string
}

func (e *DeferredError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("deferred expression: %s", e.Expression)
}

// =============================================================================
// Interfaces
// =============================================================================

// Provider is the minimal contract for secret backends.
type Provider interface {
	// LoadSecrets loads/unlocks the secret store.
	LoadSecrets() error

	// Resolve fetches a secret value by reference.
	// Returns (value, handled, error). handled=false means this
	// provider doesn't own the requested vault.
	Resolve(ref SecretRef) (string, bool, error)
}

// =============================================================================
// Resolver
// =============================================================================

// Resolver dispatches secret references across configured providers and
// serves as the evaluator helper implementation.
type Resolver struct {
	providers []Provider
	shell     shell.Shell
}

// NewResolver creates a Resolver from the given providers and shell.
func NewResolver(providers []Provider, sh shell.Shell) *Resolver {
	return &Resolver{
		providers: providers,
		shell:     sh,
	}
}

// Resolve finds the right provider and returns the secret value.
// Registers the value with shell for scrubbing.
func (r *Resolver) Resolve(ref SecretRef) (string, error) {
	for _, p := range r.providers {
		value, handled, err := p.Resolve(ref)
		if !handled {
			continue
		}
		if err != nil {
			return "", err
		}
		if r.shell != nil {
			r.shell.RegisterResolvedSecret(value)
		}
		return value, nil
	}
	return "", werror.New(fmt.Sprintf("No secrets provider handles the vault %q.", ref.Vault),
		`Set secrets.sops.enabled to true to use the "sops" vault, or add the vault under secrets.onepassword.vaults.`, nil)
}

// EvaluateHelper is the expr helper callback for secret(vault, item[, field]).
// On first pass (deferred=false), returns DeferredError.
// On second pass (deferred=true), resolves the secret.
func (r *Resolver) EvaluateHelper(params []any, deferred bool) (any, error) {
	vault, item, field, err := parseHelperParams(params)
	if err != nil {
		return nil, err
	}
	if !deferred {
		return nil, &DeferredError{
			Expression: fmt.Sprintf(`secret("%s", "%s", "%s")`, vault, item, field),
			Message:    "secret expression is deferred",
		}
	}
	ref := SecretRef{Vault: vault, Item: item, Field: field}
	return r.Resolve(ref)
}

// LoadAll calls LoadSecrets on every provider.
func (r *Resolver) LoadAll() error {
	for _, p := range r.providers {
		if err := p.LoadSecrets(); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Notation Functions
// =============================================================================

const (
	secretPrefix       = "secret."
	secretsPrefix      = "secrets."
	secretHelperPrefix = "secret("
)

// NormalizeExpression takes any supported secret notation and returns the
// equivalent secret("vault","item","field") function call string.
// Returns the original string and false if the input is not a secret reference.
//
// Handles:
//
//	"secret.op.vault.item.field"  -> secret("vault","item","field")
//	"secrets.op.vault.item.field" -> secret("vault","item","field")
//	"secret.sops.key.path"        -> secret("sops","key.path","")
//	"op.vault.item.field"          -> secret("vault","item","field")
//	"sops.key.path"                -> secret("sops","key.path","")
//	"secret(\"v\",\"i\",\"f\")"  -> unchanged (already canonical)
func NormalizeExpression(expr string) (string, bool) {
	trimmed := strings.TrimSpace(expr)

	// Already in canonical form
	if strings.HasPrefix(trimmed, secretHelperPrefix) {
		return trimmed, false
	}

	// Try secret./secrets. prefix first
	if parts, ok := parseSecretNotationParts(trimmed); ok {
		if rewritten, ok := rewriteNotationParts(parts); ok {
			return rewritten, true
		}
	}

	// Try bare op./sops. prefix (legacy)
	if strings.HasPrefix(trimmed, "op.") || strings.HasPrefix(trimmed, "op[") {
		parts := parseKeys(trimmed)
		if len(parts) >= 1 && strings.EqualFold(parts[0], "op") {
			if rewritten, ok := rewriteNotationParts(parts); ok {
				return rewritten, true
			}
		}
	}
	if strings.HasPrefix(trimmed, "sops.") || strings.HasPrefix(trimmed, "sops[") {
		parts := parseKeys(trimmed)
		if len(parts) >= 1 && strings.EqualFold(parts[0], "sops") {
			if rewritten, ok := rewriteNotationParts(parts); ok {
				return rewritten, true
			}
		}
	}

	return "", false
}

// NormalizeLegacyBraces converts ${{ expr }} to ${ expr }.
func NormalizeLegacyBraces(value string) string {
	return legacyBracePattern.ReplaceAllString(value, `${ $1 }`)
}

// isSecretExpression reports whether an expression body is a secret reference.
func isSecretExpression(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, secretPrefix) ||
		strings.HasPrefix(trimmed, secretsPrefix) ||
		strings.HasPrefix(trimmed, secretHelperPrefix)
}

// IsCacheable reports whether an expression is a standalone secret reference
// eligible for TF_VAR cache reuse.
func IsCacheable(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	if isExactSecretHelperCall(trimmed) {
		return true
	}
	_, rewritten := NormalizeExpression(trimmed)
	return rewritten
}

// =============================================================================
// Private Helpers
// =============================================================================

// missingOnePasswordFieldError reports a secret() call to a 1Password vault that names no field.
func missingOnePasswordFieldError(ref SecretRef) error {
	return werror.New(fmt.Sprintf("The secret(%q, %q) call names no field, which a 1Password vault needs.", ref.Vault, ref.Item),
		"Add the field name as the third argument of secret().", nil)
}

// parseHelperParams validates the secret(...) helper has 2 or 3 string arguments. A missing
// field argument is returned as an empty string.
func parseHelperParams(params []any) (string, string, string, error) {
	if len(params) != 2 && len(params) != 3 {
		return "", "", "", werror.New(fmt.Sprintf("The secret() helper takes 2 or 3 arguments, but the call has %d.", len(params)), secretArgsRemediation, nil)
	}
	vault, ok := params[0].(string)
	if !ok {
		return "", "", "", werror.New(fmt.Sprintf("The secret() vault argument must be a string, not %T.", params[0]), secretArgsRemediation, nil)
	}
	item, ok := params[1].(string)
	if !ok {
		return "", "", "", werror.New(fmt.Sprintf("The secret() item argument must be a string, not %T.", params[1]), secretArgsRemediation, nil)
	}
	field := ""
	if len(params) == 3 {
		field, ok = params[2].(string)
		if !ok {
			return "", "", "", werror.New(fmt.Sprintf("The secret() field argument must be a string, not %T.", params[2]), secretArgsRemediation, nil)
		}
	}
	return vault, item, field, nil
}

// isExactSecretHelperCall reports whether expr is a standalone secret(...) call
// with two or three double-quoted string arguments and no surrounding operators.
func isExactSecretHelperCall(expr string) bool {
	return exactSecretCallPattern.MatchString(expr)
}

// parseSecretNotationParts extracts tokens from secret.* and secrets.* notation.
func parseSecretNotationParts(expression string) ([]string, bool) {
	rawPath, ok := stripSecretPrefix(expression)
	if !ok {
		return nil, false
	}
	parts := parseKeys(rawPath)
	if len(parts) == 0 {
		return nil, false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil, false
		}
	}
	return parts, true
}

// rewriteNotationParts rewrites parsed tokens into the secret(...) helper call.
func rewriteNotationParts(parts []string) (string, bool) {
	if len(parts) == 0 {
		return "", false
	}
	switch strings.ToLower(parts[0]) {
	case "op":
		if len(parts) != 4 {
			return "", false
		}
		return fmt.Sprintf("secret(%q, %q, %q)", parts[1], parts[2], parts[3]), true
	case "sops":
		if len(parts) < 2 {
			return "", false
		}
		return fmt.Sprintf("secret(%q, %q, %q)", "sops", strings.Join(parts[1:], "."), ""), true
	default:
		return "", false
	}
}

// stripSecretPrefix removes secret./secrets. prefix.
func stripSecretPrefix(expression string) (string, bool) {
	rawPath := expression
	switch {
	case strings.HasPrefix(rawPath, secretPrefix):
		rawPath = strings.TrimPrefix(rawPath, secretPrefix)
	case strings.HasPrefix(rawPath, secretsPrefix):
		rawPath = strings.TrimPrefix(rawPath, secretsPrefix)
	default:
		return "", false
	}
	if strings.TrimSpace(rawPath) == "" {
		return "", false
	}
	return rawPath, true
}

// parseKeys processes a string path with mixed dot and bracket notations,
// extracting and returning an array of keys.
func parseKeys(path string) []string {
	var keys []string
	var currentKey strings.Builder
	var bracketDepth int
	inQuotes := false
	justClosedBracket := false

	trimmedPath := strings.TrimSpace(path)

	for i := 0; i < len(trimmedPath); i++ {
		char := rune(trimmedPath[i])
		switch char {
		case '[':
			if !inQuotes {
				if bracketDepth == 0 {
					if currentKey.Len() > 0 {
						keys = append(keys, currentKey.String())
						currentKey.Reset()
					}
					justClosedBracket = false
				} else {
					currentKey.WriteRune(char)
				}
				bracketDepth++
			} else {
				currentKey.WriteRune(char)
			}
		case ']':
			if !inQuotes {
				bracketDepth--
				if bracketDepth < 0 {
					bracketDepth = 0
				}
				if bracketDepth == 0 {
					if currentKey.Len() > 0 {
						keys = append(keys, currentKey.String())
						currentKey.Reset()
					} else {
						keys = append(keys, "")
					}
					justClosedBracket = true
				} else {
					currentKey.WriteRune(char)
				}
			} else {
				currentKey.WriteRune(char)
			}
		case '.':
			if bracketDepth == 0 && !inQuotes {
				if currentKey.Len() > 0 {
					keys = append(keys, currentKey.String())
					currentKey.Reset()
					justClosedBracket = false
				} else {
					if !justClosedBracket {
						keys = append(keys, "")
					}
					justClosedBracket = false
				}
			} else {
				currentKey.WriteRune(char)
			}
		case '"', '\'':
			if bracketDepth > 0 {
				inQuotes = !inQuotes
			}
			justClosedBracket = false
		case '\\':
			if bracketDepth > 0 && inQuotes && i+1 < len(trimmedPath) {
				i++
				currentKey.WriteRune(rune(trimmedPath[i]))
			} else {
				currentKey.WriteRune(char)
			}
			justClosedBracket = false
		default:
			currentKey.WriteRune(char)
			justClosedBracket = false
		}
	}

	if currentKey.Len() > 0 || !justClosedBracket {
		keys = append(keys, currentKey.String())
	}

	return keys
}
