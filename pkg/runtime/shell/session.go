// The session methods are the DefaultShell methods for the session token and the env reset.
// It provides the session token, the reset token files in .windsor, and the reset check.
// Reset removes the env variables and aliases that WINDSOR_MANAGED_ENV and WINDSOR_MANAGED_ALIAS name.
// The reset token file for a session tells the next hook run to reset the environment.

package shell

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// SessionTokenPrefix is the prefix used for session token files
const SessionTokenPrefix = ".session."

// =============================================================================
// Public Methods
// =============================================================================

// WriteResetToken writes a reset token file based on the WINDSOR_SESSION_TOKEN
// environment variable. If the environment variable doesn't exist, no file is written.
// Returns the path to the written file or an empty string if no file was written.
func (s *DefaultShell) WriteResetToken() (string, error) {
	sessionToken := s.shims.Getenv("WINDSOR_SESSION_TOKEN")
	if sessionToken == "" {
		return "", nil
	}

	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return "", werror.Wrap(err, "error getting project root")
	}

	windsorDir := filepath.Join(projectRoot, ".windsor")
	if err := s.shims.MkdirAll(windsorDir, 0750); err != nil {
		return "", werror.Wrap(err, "error creating .windsor directory")
	}

	sessionFilePath := filepath.Join(windsorDir, SessionTokenPrefix+sessionToken)

	if err := s.shims.WriteFile(sessionFilePath, []byte{}, 0600); err != nil {
		return "", werror.Wrap(err, "error writing reset token file")
	}

	return sessionFilePath, nil
}

// GetSessionToken retrieves or generates a session token. It first checks if a token is already stored in memory.
// If not, it looks for a token in the environment variable. If no token is found in the environment, it generates a new token.
func (s *DefaultShell) GetSessionToken() (string, error) {
	if s.sessionToken != "" {
		return s.sessionToken, nil
	}

	envToken := s.shims.Getenv("WINDSOR_SESSION_TOKEN")
	if envToken != "" {
		s.sessionToken = envToken
		return envToken, nil
	}

	token, err := s.generateRandomString(7)
	if err != nil {
		return "", werror.Wrap(err, "error generating session token")
	}

	s.sessionToken = token
	return token, nil
}

// CheckResetFlags checks if a reset signal file exists for the current session token.
// It returns true if the specific session token file exists and always removes all .session.* files.
func (s *DefaultShell) CheckResetFlags() (bool, error) {
	envToken := s.shims.Getenv("WINDSOR_SESSION_TOKEN")
	if envToken == "" {
		return false, nil
	}

	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return false, werror.Wrap(err, "error getting project root")
	}

	windsorDir := filepath.Join(projectRoot, ".windsor")
	tokenFilePath := filepath.Join(windsorDir, ".session."+envToken)

	tokenFileExists := false
	if _, err := s.shims.Stat(tokenFilePath); err == nil {
		tokenFileExists = true
	}

	sessionFiles, err := s.shims.Glob(filepath.Join(windsorDir, SessionTokenPrefix+"*"))
	if err != nil {
		return false, werror.Wrap(err, "error finding session files")
	}

	for _, file := range sessionFiles {
		if err := s.shims.RemoveAll(file); err != nil {
			return false, werror.Wrap(err, "error removing session file %s", file)
		}
	}

	return tokenFileExists, nil
}

// Reset removes all managed environment variables and aliases.
// It uses the environment variables "WINDSOR_MANAGED_ENV" and "WINDSOR_MANAGED_ALIAS"
// to retrieve the previous set of managed environment variables and aliases, respectively.
// These environment variables represent the previous set of managed values that need to be reset.
// The optional quiet parameter controls whether shell commands are printed during reset.
func (s *DefaultShell) Reset(quiet ...bool) {
	isQuiet := len(quiet) > 0 && quiet[0]
	var managedEnvs []string
	if envStr := s.shims.Getenv("WINDSOR_MANAGED_ENV"); envStr != "" {
		for env := range strings.SplitSeq(envStr, ",") {
			env = strings.TrimSpace(env)
			if env != "" {
				managedEnvs = append(managedEnvs, env)
				os.Unsetenv(env)
			}
		}
	}

	var managedAliases []string
	if aliasStr := s.shims.Getenv("WINDSOR_MANAGED_ALIAS"); aliasStr != "" {
		for alias := range strings.SplitSeq(aliasStr, ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				managedAliases = append(managedAliases, alias)
			}
		}
	}

	if !isQuiet {
		if len(managedEnvs) > 0 {
			s.UnsetEnvs(managedEnvs)
		}
		if len(managedAliases) > 0 {
			s.UnsetAlias(managedAliases)
		}
	}
}

// ResetSessionToken resets the session token - used primarily for testing
func (s *DefaultShell) ResetSessionToken() {
	s.sessionToken = ""
}

// =============================================================================
// Private Methods
// =============================================================================

// generateRandomString creates a secure random string of the given length using a predefined charset.
func (s *DefaultShell) generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := s.shims.RandRead(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}
