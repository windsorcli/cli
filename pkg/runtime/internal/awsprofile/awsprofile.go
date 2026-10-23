// The awsprofile package finds the AWS profiles in the .aws directory of a context or in the ambient AWS config.
// It provides a Resolver that checks for a profile and lists the profiles that are defined.
// Callers use it to decide whether to set AWS_PROFILE.
// If AWS_PROFILE names a profile that the SDK cannot find, the SDK fails and ignores the other credentials.

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

// Resolver holds the paths of the AWS config file and credentials file that a lookup reads.
// Use ForContext or Ambient to make one.
type Resolver struct {
	configPath, credentialsPath string
}

// ForContext returns a Resolver that reads <configRoot>/.aws/config and <configRoot>/.aws/credentials.
// Project mode uses it, because windsor owns the .aws directory of the context.
func ForContext(configRoot string) Resolver {
	awsDir := filepath.Join(configRoot, ".aws")
	return Resolver{
		configPath:      filepath.Join(awsDir, "config"),
		credentialsPath: filepath.Join(awsDir, "credentials"),
	}
}

// Ambient returns a Resolver for the files that the AWS SDK reads when windsor does not set them.
// It uses AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE when they are set, and the files in ~/.aws when they are not.
// A path stays empty when it has no variable and there is no home directory. An empty path matches no profile.
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

// HasProfile reports whether the named profile is in the config file or the credentials file.
// The AWS SDK accepts a profile from either file, so one match is sufficient.
// The config file uses "[profile <name>]", or "[default]" for the default profile.
// The credentials file uses "[<name>]".
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

// ListProfileNames returns each profile name in the config file and the credentials file, without duplicates.
// It reads the config file first and keeps the order in which it finds the names.
// When a profile is missing, the caller uses this list to tell the user which profiles exist.
// For example, `aws configure sso` without --profile names the profile after the SSO role, not the context.
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

// WarnOnProfileMismatch writes a warning to w when expected is missing but a different profile is defined.
// This tells the user that the profile probably has the wrong name.
// It writes nothing when expected is defined, or when no profile is defined.
func WarnOnProfileMismatch(w io.Writer, r Resolver, expected string) {
	if r.HasProfile(expected) {
		return
	}
	found := r.ListProfileNames()
	if len(found) == 0 {
		return
	}
	msg := fmt.Sprintf("Warning: AWS profile %q not found; found %s instead. Set aws.profile to match, or rename the profile to %q.",
		expected, strings.Join(found, ", "), expected)
	if shouldColorize(w) {
		msg = "\033[33m" + msg + "\033[0m"
	}
	fmt.Fprintln(w, msg)
}

// shouldColorize reports whether to add color to the output of w.
// It returns true only when NO_COLOR is not set, with any value, and w is a terminal.
// A pipe, a log file or a buffer gets no ANSI escapes, so the captured output of `windsor env` stays clean.
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

// profileNamesInFile returns the profile name of each INI section header in the file at path.
// When isConfigFile is true, it reads "[profile <name>]" and "[default]" headers.
// When it is false, it reads "[<name>]" headers, which the credentials file uses for all profiles.
// It returns nil when it cannot read the file.
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

// iniContainsSection reports whether the file at path has a line that is equal to section.
// Before it compares, it removes a "#" or ";" comment and the surrounding spaces from each line.
// It returns false when it cannot read the file.
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
