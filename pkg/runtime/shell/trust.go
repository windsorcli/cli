// The trust methods are the DefaultShell methods that manage the trusted project directories.
// It provides the addition of the project root to the trusted file and the trust check.
// The trusted file is $HOME/.config/windsor/.trusted, with one directory on each line.
// In global mode, trust is implicit and both methods do nothing.

package shell

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Public Methods
// =============================================================================

// AddCurrentDirToTrustedFile adds the current directory to a trusted list stored in a file.
// Creates necessary directories if they don't exist.
// Checks if the directory is already trusted before adding.
// In global mode, trust is implicit for $HOME/.config/windsor and this is a no-op.
func (s *DefaultShell) AddCurrentDirToTrustedFile() error {
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return werror.Wrap(err, "Error getting project root directory")
	}

	if s.global {
		return nil
	}

	homeDir, err := s.shims.UserHomeDir()
	if err != nil {
		return werror.Wrap(err, "Error getting user home directory")
	}

	trustedDirPath := path.Join(homeDir, ".config", "windsor")
	err = s.shims.MkdirAll(trustedDirPath, 0750)
	if err != nil {
		return werror.Wrap(err, "Error creating directories for trusted file")
	}

	trustedFilePath := path.Join(trustedDirPath, ".trusted")

	data, err := s.shims.ReadFile(trustedFilePath)
	if err != nil && !os.IsNotExist(err) {
		return werror.Wrap(err, "Error reading trusted file")
	}

	trustedDirs := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, dir := range trustedDirs {
		if strings.TrimSpace(dir) == projectRoot {
			return nil
		}
	}

	data = append(data, []byte(projectRoot+"\n")...)
	if err := s.shims.WriteFile(trustedFilePath, data, 0600); err != nil {
		return werror.Wrap(err, "Error writing to trusted file")
	}

	return nil
}

// CheckTrustedDirectory verifies that the project root equals a trusted directory or is inside one.
// It compares path components, so "/a/proj-evil" does not inherit trust from "/a/proj".
// In global mode, trust is implicit for $HOME/.config/windsor and this check is skipped.
func (s *DefaultShell) CheckTrustedDirectory() error {
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return werror.Wrap(err, "Error getting project root directory")
	}

	if s.global {
		return nil
	}

	homeDir, err := s.shims.UserHomeDir()
	if err != nil {
		return werror.Wrap(err, "Error getting user home directory")
	}

	trustedDirPath := path.Join(homeDir, ".config", "windsor")
	trustedFilePath := path.Join(trustedDirPath, ".trusted")

	data, err := s.shims.ReadFile(trustedFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Trusted file does not exist")
		}
		return werror.Wrap(err, "Error reading trusted file")
	}

	isTrusted := false
	trustedDirs := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, trustedDir := range trustedDirs {
		trimmedDir := strings.TrimSpace(trustedDir)
		if trimmedDir == "" {
			continue
		}
		rel, err := filepath.Rel(filepath.Clean(trimmedDir), filepath.Clean(projectRoot))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		return fmt.Errorf("Current directory not in the trusted list")
	}

	return nil
}
