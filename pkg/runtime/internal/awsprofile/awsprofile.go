// Package awsprofile checks whether a named AWS profile is defined in either a
// context-scoped .aws/ directory or the operator's ambient AWS config. Callers
// use the result to decide whether emitting AWS_PROFILE is safe — pinning a
// profile that doesn't exist in the file the SDK will read causes a hard
// "profile not found" error that masks ambient env-var or IMDS credentials.
package awsprofile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
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
