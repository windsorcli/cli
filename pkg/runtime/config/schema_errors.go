// The schema_errors file holds the error list steps of the SchemaValidator.
// It provides the flattening of a kaptinlin evaluation result into one line per violation.
// It drops duplicate lines, merges required errors, and hides errors that restate another.
// A note at the end of the list counts the hidden errors.

package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kaptinlin/jsonschema"
	"github.com/windsorcli/cli/pkg/debug"
)

// =============================================================================
// Constants
// =============================================================================

// structuralValidationKeywords are keywords whose failure is a symptom of a nested, more
// specific failure elsewhere rather than the actionable violation itself — "properties" only
// ever says "property X doesn't match its schema", never why; "allOf"/"anyOf"/"oneOf"/"if"/"then"
// are the same shape one level up. collectErrors sorts these after every other keyword so an
// operator sees the specific cause (additionalProperties, const, required, type, ...) first,
// instead of every ancestor schema that failed as a consequence of it.
var structuralValidationKeywords = map[string]bool{
	"properties": true,
	"allOf":      true,
	"anyOf":      true,
	"oneOf":      true,
	"if":         true,
	"then":       true,
}

// =============================================================================
// Types
// =============================================================================

// validationError pairs a formatted error line with its instance location, keyword, and message.
type validationError struct {
	location string
	keyword  string
	message  string
	line     string
}

// =============================================================================
// Helpers
// =============================================================================

// collectErrors flattens a kaptinlin EvaluationResult into a deduplicated, prioritized list of
// error strings, one per distinct leaf, formatted as "<instance-path>: <keyword>: <message>".
// A schema with several allOf/if branches referencing the same property routinely reports the
// identical (path, keyword, message) triple from each branch — e.g. a cluster-driver coherence
// check and an identity-driver coherence check both restating "/: properties: Property
// 'cluster' does not match the schema" — so exact-duplicate lines are dropped, and the survivors
// are stably reordered per structuralValidationKeywords.
func collectErrors(result *jsonschema.EvaluationResult) []string {
	if result.IsValid() {
		return nil
	}
	return flattenErrorList(result.ToList(true))
}

// flattenErrorList walks list via walkList, then dedups and prioritizes the result — split out
// from collectErrors so the dedup/prioritization logic is testable directly against a
// hand-constructed *jsonschema.List, without needing a real schema evaluation to reproduce a
// specific duplicate/cascading shape.
func flattenErrorList(list *jsonschema.List) []string {
	var raw []validationError
	walkList(list, "", &raw)
	if len(raw) == 0 {
		return []string{"validation failed"}
	}

	seen := make(map[string]bool, len(raw))
	deduped := make([]validationError, 0, len(raw))
	for _, e := range raw {
		if seen[e.line] {
			continue
		}
		seen[e.line] = true
		deduped = append(deduped, e)
	}

	deduped = focusOnSpecificViolations(deduped)
	deduped = mergeRequiredDuplicates(deduped)
	deduped = dropRedundantRequiredCompanions(deduped)

	sort.SliceStable(deduped, func(i, j int) bool {
		return !structuralValidationKeywords[deduped[i].keyword] && structuralValidationKeywords[deduped[j].keyword]
	})

	errs := make([]string, len(deduped))
	for i, e := range deduped {
		errs[i] = e.line
	}
	return errs
}

// walkList descends a flattened List, emitting one validationError per (instance-location,
// keyword) pair. Instance locations use JSON Pointer notation rooted at "/" (e.g.
// "/network/cidr_block"). kaptinlin scopes each evaluator's leaf to a path local to that
// evaluator — a nested enum failure on storage.driver surfaces as "/driver" rather than
// "/storage/driver" — so this walker carries the parent location down the recursion and joins
// it onto each child to reconstruct the fully-qualified pointer operators expect when reading
// the message.
func walkList(list *jsonschema.List, parent string, errs *[]validationError) {
	loc := joinInstanceLocation(parent, list.InstanceLocation)
	display := loc
	if display == "" {
		display = "/"
	}
	for keyword, msg := range list.Errors {
		*errs = append(*errs, validationError{
			location: display,
			keyword:  keyword,
			message:  msg,
			line:     fmt.Sprintf("%s: %s: %s", display, keyword, msg),
		})
	}
	for i := range list.Details {
		walkList(&list.Details[i], loc, errs)
	}
}

// quotedToken extracts the first single-quoted token from a message, e.g. "Required property
// 'identity' is missing" -> "identity". Returns "" when the message quotes nothing.
func quotedToken(message string) string {
	tokens := quotedTokens(message)
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

// quotedTokens extracts every single-quoted token from a message, in order, e.g. "Required
// properties 'storage', 'driver' are missing" -> ["storage", "driver"]. Returns nil when the
// message quotes nothing.
func quotedTokens(message string) []string {
	var tokens []string
	rest := message
	for {
		start := strings.IndexByte(rest, '\'')
		if start < 0 {
			return tokens
		}
		rest = rest[start+1:]
		end := strings.IndexByte(rest, '\'')
		if end < 0 {
			return tokens
		}
		tokens = append(tokens, rest[:end])
		rest = rest[end+1:]
	}
}

// cascadePairedNames finds property names with both a root "required" failure and a matching
// "type: null but should be object" failure. Only object-typed fields get this pairing;
// composition fills those in later. A missing scalar, like platform, never pairs.
func cascadePairedNames(errs []validationError) map[string]bool {
	nullAt := map[string]bool{}
	for _, e := range errs {
		if e.keyword == "type" && strings.Contains(e.message, "null") && strings.Contains(e.message, "object") {
			nullAt[strings.TrimPrefix(e.location, "/")] = true
		}
	}
	paired := map[string]bool{}
	for _, e := range errs {
		if e.location != "/" || e.keyword != "required" {
			continue
		}
		if name := quotedToken(e.message); name != "" && nullAt[name] {
			paired[name] = true
		}
	}
	return paired
}

// isCascadeProne reports whether e is part of a pair from cascadePairedNames. A nested
// required error, an unpaired required error, or a non-null type error stays visible.
func isCascadeProne(e validationError, paired map[string]bool) bool {
	switch e.keyword {
	case "required":
		return e.location == "/" && paired[quotedToken(e.message)]
	case "type":
		return paired[strings.TrimPrefix(e.location, "/")]
	}
	return false
}

// focusOnSpecificViolations hides cascade-prone and structural errors when a specific
// violation is also present. It returns errs unchanged when either side is empty. It logs
// every hidden line via debug.Log, so an operator can rerun with --debug to see them.
func focusOnSpecificViolations(errs []validationError) []validationError {
	paired := cascadePairedNames(errs)
	var specific, noise []validationError
	cascadeCount := 0
	for _, e := range errs {
		switch {
		case isCascadeProne(e, paired):
			cascadeCount++
			noise = append(noise, e)
		case structuralValidationKeywords[e.keyword]:
			noise = append(noise, e)
		default:
			specific = append(specific, e)
		}
	}
	if len(specific) == 0 || cascadeCount == 0 {
		return errs
	}

	for _, e := range noise {
		debug.Log("hidden validation error: %s", e.line)
	}

	note := fmt.Sprintf("%d other error(s) are hidden. Run with --debug to see them. Fix the error(s) above and run the command again.", len(noise))
	return append(specific, validationError{line: note})
}

// mergeRequiredDuplicates unions multiple "required" errors at the same location into one.
// Several allOf/if branches routinely each declare their own required list for the same
// object — e.g. a cluster-driver coherence check and a separate cluster-storage coherence
// check — so kaptinlin reports "Required property 'driver' is missing" and "Required
// properties 'storage', 'driver' are missing" as two lines restating an overlapping fact.
// This keeps the first line's position, rewriting it to list the sorted union of names once,
// and drops the later duplicates at that location.
func mergeRequiredDuplicates(errs []validationError) []validationError {
	type group struct {
		idx   int
		names []string
		seen  map[string]bool
	}
	groups := make(map[string]*group)
	merged := make([]validationError, 0, len(errs))
	for _, e := range errs {
		if e.keyword != "required" {
			merged = append(merged, e)
			continue
		}
		g, ok := groups[e.location]
		if !ok {
			g = &group{idx: len(merged), seen: map[string]bool{}}
			groups[e.location] = g
			merged = append(merged, e)
		}
		for _, name := range quotedTokens(e.message) {
			if !g.seen[name] {
				g.seen[name] = true
				g.names = append(g.names, name)
			}
		}
	}
	for loc, g := range groups {
		sort.Strings(g.names)
		message := formatRequiredMessage(g.names)
		merged[g.idx] = validationError{
			location: loc,
			keyword:  "required",
			message:  message,
			line:     fmt.Sprintf("%s: required: %s", loc, message),
		}
	}
	return merged
}

// formatRequiredMessage renders a sorted list of missing property names in kaptinlin's own
// "required" phrasing, so a merged line reads exactly as it would have for the same names.
func formatRequiredMessage(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	if len(quoted) == 1 {
		return fmt.Sprintf("Required property %s is missing", quoted[0])
	}
	return fmt.Sprintf("Required properties %s are missing", strings.Join(quoted, ", "))
}

// dropRedundantRequiredCompanions removes a "type" (null) or "const" error whose location is
// an immediate child of a "required" error naming that same field. "/cluster: required:
// Required property 'driver' is missing" already says cluster.driver is absent, so
// "/cluster/driver: type: Value is null but should be string" and "/cluster/driver: const:
// Value does not match the constant value" add nothing: a null value trivially fails any
// type or const check. Unlike focusOnSpecificViolations, this always drops the companion —
// it does not wait for some other violation to exist elsewhere — because the required error
// alone is already the complete, actionable statement.
func dropRedundantRequiredCompanions(errs []validationError) []validationError {
	requiredNames := make(map[string]map[string]bool)
	for _, e := range errs {
		if e.keyword != "required" {
			continue
		}
		set := requiredNames[e.location]
		if set == nil {
			set = map[string]bool{}
			requiredNames[e.location] = set
		}
		for _, name := range quotedTokens(e.message) {
			set[name] = true
		}
	}

	out := make([]validationError, 0, len(errs))
	dropped := 0
	for _, e := range errs {
		isCompanion := e.keyword == "const" || (e.keyword == "type" && strings.Contains(e.message, "null"))
		if isCompanion {
			parent, name := splitInstanceLocation(e.location)
			if requiredNames[parent][name] {
				debug.Log("hidden validation error: %s", e.line)
				dropped++
				continue
			}
		}
		out = append(out, e)
	}
	if dropped == 0 {
		return errs
	}
	return addHiddenCount(out, dropped)
}

// splitInstanceLocation splits a JSON Pointer into its parent and final segment, e.g.
// "/cluster/driver" -> ("/cluster", "driver"). A single-segment path like "/identity" splits
// to ("/", "identity").
func splitInstanceLocation(loc string) (parent, name string) {
	trimmed := strings.TrimPrefix(loc, "/")
	idx := strings.LastIndexByte(trimmed, '/')
	if idx < 0 {
		return "/", trimmed
	}
	return "/" + trimmed[:idx], trimmed[idx+1:]
}

// addHiddenCount folds extra into an existing "N other error(s) are hidden" note, or appends
// a new one when focusOnSpecificViolations did not already add one — e.g. every surviving
// violation was cascade-free until dropRedundantRequiredCompanions ran.
func addHiddenCount(errs []validationError, extra int) []validationError {
	for i, e := range errs {
		if e.keyword == "" && e.location == "" && strings.Contains(e.line, "other error(s) are hidden") {
			var n int
			if parsed, err := fmt.Sscanf(e.line, "%d", &n); err != nil || parsed != 1 {
				n = 0
			}
			errs[i] = validationError{line: fmt.Sprintf(
				"%d other error(s) are hidden. Run with --debug to see them. Fix the error(s) above and run the command again.", n+extra)}
			return errs
		}
	}
	return append(errs, validationError{line: fmt.Sprintf(
		"%d other error(s) are hidden. Run with --debug to see them. Fix the error(s) above and run the command again.", extra)})
}

// joinInstanceLocation concatenates two JSON Pointer fragments, treating "" and "/" as the
// document root and guarding against double-prepending when a child evaluator already
// reports its absolute path.
func joinInstanceLocation(parent, child string) string {
	switch {
	case child == "" || child == "/":
		return parent
	case parent == "" || parent == "/":
		return child
	case strings.HasPrefix(child, parent+"/"):
		return child
	default:
		return parent + child
	}
}
