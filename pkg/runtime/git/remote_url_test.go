package git

import "testing"

func TestNormalizeRemoteURL(t *testing.T) {
	t.Run("ConvertsScpStyleSSHRemote", func(t *testing.T) {
		normalized := NormalizeRemoteURL("git@github.com:owner/repo.git")
		if normalized != "ssh://git@github.com/owner/repo.git" {
			t.Errorf("Expected ssh://git@github.com/owner/repo.git, got %s", normalized)
		}
	})

	t.Run("PrefixesHTTPSForBareRemote", func(t *testing.T) {
		normalized := NormalizeRemoteURL("github.com/owner/repo.git")
		if normalized != "https://github.com/owner/repo.git" {
			t.Errorf("Expected https://github.com/owner/repo.git, got %s", normalized)
		}
	})

	t.Run("PreservesGitScheme", func(t *testing.T) {
		normalized := NormalizeRemoteURL("git://github.com/owner/repo.git")
		if normalized != "git://github.com/owner/repo.git" {
			t.Errorf("Expected git://github.com/owner/repo.git, got %s", normalized)
		}
	})

	t.Run("PreservesFileScheme", func(t *testing.T) {
		normalized := NormalizeRemoteURL("file:///tmp/repo")
		if normalized != "file:///tmp/repo" {
			t.Errorf("Expected file:///tmp/repo, got %s", normalized)
		}
	})

	t.Run("PreservesExtendedValidScheme", func(t *testing.T) {
		normalized := NormalizeRemoteURL("git+ssh://github.com/owner/repo.git")
		if normalized != "git+ssh://github.com/owner/repo.git" {
			t.Errorf("Expected git+ssh://github.com/owner/repo.git, got %s", normalized)
		}
	})

	t.Run("PrefixesHTTPSForInvalidSchemePrefix", func(t *testing.T) {
		normalized := NormalizeRemoteURL("1invalid://github.com/owner/repo.git")
		if normalized != "https://1invalid://github.com/owner/repo.git" {
			t.Errorf("Expected https://1invalid://github.com/owner/repo.git, got %s", normalized)
		}
	})
}

func TestNormalizeRepositoryURL(t *testing.T) {
	t.Run("NormalizesHTTPSRemoteToHostPath", func(t *testing.T) {
		normalized := NormalizeRepositoryURL("https://github.com/owner/repo.git")
		if normalized != "github.com/owner/repo" {
			t.Errorf("Expected github.com/owner/repo, got %s", normalized)
		}
	})

	t.Run("NormalizesScpStyleSSHRemoteToHostPath", func(t *testing.T) {
		normalized := NormalizeRepositoryURL("git@github.com:owner/repo.git")
		if normalized != "github.com/owner/repo" {
			t.Errorf("Expected github.com/owner/repo, got %s", normalized)
		}
	})

	t.Run("NormalizesGitSchemeRemoteToHostPath", func(t *testing.T) {
		normalized := NormalizeRepositoryURL("git://github.com/owner/repo.git")
		if normalized != "github.com/owner/repo" {
			t.Errorf("Expected github.com/owner/repo, got %s", normalized)
		}
	})

	t.Run("NormalizesExtendedSchemeRemoteToHostPath", func(t *testing.T) {
		normalized := NormalizeRepositoryURL("git+ssh://github.com/owner/repo.git")
		if normalized != "github.com/owner/repo" {
			t.Errorf("Expected github.com/owner/repo, got %s", normalized)
		}
	})

	t.Run("NormalizesInvalidSchemePrefixUsingParsedHostPath", func(t *testing.T) {
		normalized := NormalizeRepositoryURL("1invalid://github.com/owner/repo.git")
		if normalized != "github.com/owner/repo" {
			t.Errorf("Expected github.com/owner/repo, got %s", normalized)
		}
	})

	t.Run("FallsBackForFileSchemeRemote", func(t *testing.T) {
		normalized := NormalizeRepositoryURL("file:///tmp/repo")
		if normalized != "file:///tmp/repo" {
			t.Errorf("Expected file:///tmp/repo, got %s", normalized)
		}
	})

	t.Run("PreservesBareHostPathRemoteWithoutProtocol", func(t *testing.T) {
		normalized := NormalizeRepositoryURL("github.com/owner/repo.git")
		if normalized != "github.com/owner/repo" {
			t.Errorf("Expected github.com/owner/repo, got %s", normalized)
		}
	})
}
