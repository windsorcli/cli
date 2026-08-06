// Package awsprofile checks whether a named AWS profile is defined in either a
// context-scoped .aws/ directory or the operator's ambient AWS config. Callers
// use the result to decide whether emitting AWS_PROFILE is safe — pinning a
// profile that doesn't exist in the file the SDK will read causes a hard
// "profile not found" error that masks ambient env-var or IMDS credentials.
package awsprofile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// Resolver carries the AWS config and credentials file paths a single profile
// lookup should read against. Construct via ForContext or Ambient.
type Resolver struct {
	configPath, credentialsPath string
}

// ForContext scopes lookups to <configRoot>/.aws/config and .aws/credentials.
// Used in project mode where windsor owns the context's .aws/ directory.
func ForContext(configRoot string) Resolver {
	awsDir := filepath.Join(configRoot, ".aws")
	return Resolver{
		configPath:      filepath.Join(awsDir, "config"),
		credentialsPath: filepath.Join(awsDir, "credentials"),
	}
}

// Ambient resolves to the file paths the AWS SDK reads when windsor does not
// redirect them — AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE if the operator
// set them, else ~/.aws/config and ~/.aws/credentials. Any path that cannot be
// resolved (no env override and no home dir) stays empty — HasProfile treats
// empty paths as "no match".
func Ambient() Resolver {
	configPath := os.Getenv("AWS_CONFIG_FILE")
	credentialsPath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if configPath == "" || credentialsPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			if configPath == "" {
				configPath = filepath.Join(home, ".aws", "config")
			}
			if credentialsPath == "" {
				credentialsPath = filepath.Join(home, ".aws", "credentials")
			}
		}
	}
	return Resolver{configPath: configPath, credentialsPath: credentialsPath}
}

// HasProfile reports whether the named profile is defined in either of the
// resolver's files. The AWS SDK treats a profile found in either file as
// satisfying the lookup, so a single match is enough. Section headers are
// expected as "[profile <name>]" in the config file (or "[default]" for the
// default profile) and "[<name>]" in the credentials file.
func (r Resolver) HasProfile(name string) bool {
	configHeader := "[profile " + name + "]"
	if name == "default" {
		configHeader = "[default]"
	}
	if iniContainsSection(r.configPath, configHeader) {
		return true
	}
	return iniContainsSection(r.credentialsPath, "["+name+"]")
}

// ListProfileNames returns every profile name defined across the resolver's config and
// credentials files, deduplicated, in the order first encountered (config file scanned before
// credentials). Used for diagnostics when an expected profile isn't found, so the caller can
// name what's actually configured instead of just "not found" — e.g. a bare `aws configure sso`
// (without --profile) names the profile after the SSO role/account rather than the windsor
// context, which HasProfile alone can't distinguish from "nothing configured at all."
func (r Resolver) ListProfileNames() []string {
	seen := make(map[string]bool)
	var names []string
	for _, name := range profileNamesInFile(r.configPath, true) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, name := range profileNamesInFile(r.credentialsPath, false) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// WarnOnProfileMismatch writes a non-fatal warning to w when expected is not defined in r's
// files but at least one other profile is — the "configured under the wrong name" case HasProfile
// alone can't distinguish from "nothing configured yet." Silent when expected is present (nothing
// to warn about) or when no profile is defined at all (unconfigured, not mismatched).
func WarnOnProfileMismatch(w io.Writer, r Resolver, expected string) {
	if r.HasProfile(expected) {
		return
	}
	found := r.ListProfileNames()
	if len(found) == 0 {
		return
	}
	msg := fmt.Sprintf("Warning: AWS profile %q not found; found %s instead. Set aws.awsProfile to match, or rename the profile to %q.",
		expected, strings.Join(found, ", "), expected)
	if shouldColorize(w) {
		msg = "\033[33m" + msg + "\033[0m"
	}
	fmt.Fprintln(w, msg)
}

// shouldColorize reports whether w is safe to color: NO_COLOR is unset (its mere presence, any
// value, opts out per the spec) and w is a terminal file descriptor. A non-terminal writer —
// a pipe, a redirected log file, or any io.Writer that isn't *os.File at all (e.g. a test's
// bytes.Buffer) — never gets raw ANSI escapes, so captured/piped `windsor env` output stays
// clean instead of showing literal \033[33m garbage.
func shouldColorize(w io.Writer) bool {
	if _, present := os.LookupEnv("NO_COLOR"); present {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd())) // #nosec G115 -- file descriptors are small, safe to cast to int
}

// profileNamesInFile scans path for INI section headers and returns the profile name each one
// names. isConfigFile selects the config file's "[profile <name>]" / "[default]" header shape;
// the credentials file uses a bare "[<name>]" for every profile including default. Returns nil
// for a missing or unreadable file, matching iniContainsSection's tolerance.
func profileNamesInFile(path string, isConfigFile bool) []string {
	if path == "" {
		return nil
	}
	// #nosec G304 - path is composed from the caller's trusted configRoot or AWS-SDK-equivalent env vars
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		inner := line[1 : len(line)-1]
		if isConfigFile {
			if inner == "default" {
				names = append(names, "default")
			} else if name, ok := strings.CutPrefix(inner, "profile "); ok && name != "" {
				names = append(names, name)
			}
			continue
		}
		if inner != "" {
			names = append(names, inner)
		}
	}
	return names
}

// iniContainsSection scans the file at path for a line whose trimmed contents
// match section exactly, stripping any trailing "#" or ";" inline comment
// before the comparison. Returns false on any read error so a missing or
// unreadable file is treated as "no section present" rather than fatal.
func iniContainsSection(path, section string) bool {
	if path == "" {
		return false
	}
	// #nosec G304 - path is composed from the caller's trusted configRoot or AWS-SDK-equivalent env vars
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		if strings.TrimSpace(line) == section {
			return true
		}
	}
	return false
}
