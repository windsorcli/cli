// The dotenv package provides shared, dependency-light helpers for loading standard
// dotenv-format files. It parses KEY=VALUE content, resolves ${...} expressions through
// an injected evaluator, and implements the NO_CACHE-gated reuse rule so a fresh shell
// session doesn't re-pay resolution cost.

package dotenv

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Public Methods
// =============================================================================

// Parse parses standard dotenv content (KEY=VALUE lines, # comments, blank
// lines) into a map. Lines without a top-level "=" are skipped silently.
func Parse(content string) map[string]string {
	result := make(map[string]string)

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		result[key] = strings.TrimSpace(value)
	}

	return result
}

// ShouldUseCache reports whether a cached value should be reused rather than
// re-evaluated, based on the NO_CACHE environment variable (read via lookupEnv).
// Cache is enabled by default and disabled by setting NO_CACHE=1 or NO_CACHE=true.
func ShouldUseCache(lookupEnv func(string) (string, bool)) bool {
	noCache, _ := lookupEnv("NO_CACHE")
	return noCache == "" || noCache == "0" || noCache == "false" || noCache == "False"
}

// EvaluateExpressionValue evaluates value through eval with deferred evaluation
// enabled so secrets resolve immediately. Formats a resolution error inline as
// "<ERROR: ...>" and normalizes a nil result to the empty string.
func EvaluateExpressionValue(eval evaluator.ExpressionEvaluator, value string) string {
	result, err := eval.Evaluate(value, "", nil, true)
	if err != nil {
		return fmt.Sprintf("<ERROR: %s>", err)
	}
	if result == nil {
		return ""
	}
	return fmt.Sprint(result)
}

// WarnOnLoosePermissions writes a non-fatal warning to w when path's mode is
// group- or world-accessible rather than restricted to the owner (0600-equivalent).
// A no-op when goos is "windows": NTFS has no owner/group/other model, so FileMode
// bits there don't reflect real ACL restrictiveness and chmod has no equivalent.
func WarnOnLoosePermissions(w io.Writer, goos, path string, mode os.FileMode) {
	if goos == "windows" {
		return
	}
	if mode.Perm()&0077 != 0 {
		fmt.Fprintf(w, "\033[33mWarning: %s is readable by group/other; it may contain credentials. Consider running: chmod 600 %s\033[0m\n", path, path)
	}
}
