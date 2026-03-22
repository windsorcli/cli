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
