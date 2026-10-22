// The dotenv package loads files in the standard dotenv format.
// It provides functions that parse KEY=VALUE content and evaluate ${...} expressions.
// It also applies the NO_CACHE rule, so a new shell can use the cached values again.

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

// Parse returns the KEY=VALUE lines of dotenv content as a map.
// It skips comments, blank lines and lines that have no "=".
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

// ShouldUseCache reports whether to use a cached value again, from the NO_CACHE variable that lookupEnv reads.
// The cache is on by default. Set NO_CACHE=1 or NO_CACHE=true to turn it off.
func ShouldUseCache(lookupEnv func(string) (string, bool)) bool {
	noCache, _ := lookupEnv("NO_CACHE")
	return noCache == "" || noCache == "0" || noCache == "false" || noCache == "False"
}

// EvaluateExpressionValue evaluates value with eval, with deferred evaluation on, so that secrets resolve now.
// It returns "<ERROR: ...>" when the evaluation fails, and an empty string when the result is nil.
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

// WarnOnLoosePermissions writes a warning to w when the group or other users can access the file at path.
// It does nothing when goos is "windows", because NTFS permissions do not use the FileMode bits.
func WarnOnLoosePermissions(w io.Writer, goos, path string, mode os.FileMode) {
	if goos == "windows" {
		return
	}
	if mode.Perm()&0077 != 0 {
		fmt.Fprintf(w, "\033[33mWarning: %s is readable by group/other; it may contain credentials. Consider running: chmod 600 %s\033[0m\n", path, path)
	}
}
