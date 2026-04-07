package workstation

import (
	"testing"
)

// TestGitConfig_Merge tests the Merge method of GitConfig
func TestGitConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:    boolPtr(true),
				Include:    stringPtr("*.go"),
				Exclude:    stringPtr("*.tmp"),
				Protect:    stringPtr("*.secret"),
				Username:   stringPtr("user"),
				Password:   stringPtr("pass"),
				WebhookUrl: stringPtr("https://webhook.example.com"),
				VerifySsl:  boolPtr(true),
				Image:      stringPtr("git-livereload:latest"),
			},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.Livereload == nil {
			t.Error("Expected Livereload to remain initialized")
		}
		if *base.Livereload.Enabled != *original.Livereload.Enabled {
			t.Errorf("Expected Enabled to remain unchanged")
		}
		if *base.Livereload.Include != *original.Livereload.Include {
			t.Errorf("Expected Include to remain unchanged")
		}
		if *base.Livereload.Exclude != *original.Livereload.Exclude {
			t.Errorf("Expected Exclude to remain unchanged")
		}
		if *base.Livereload.Protect != *original.Livereload.Protect {
			t.Errorf("Expected Protect to remain unchanged")
		}
		if *base.Livereload.Username != *original.Livereload.Username {
			t.Errorf("Expected Username to remain unchanged")
		}
		if *base.Livereload.Password != *original.Livereload.Password {
			t.Errorf("Expected Password to remain unchanged")
		}
		if *base.Livereload.WebhookUrl != *original.Livereload.WebhookUrl {
			t.Errorf("Expected WebhookUrl to remain unchanged")
		}
		if *base.Livereload.VerifySsl != *original.Livereload.VerifySsl {
			t.Errorf("Expected VerifySsl to remain unchanged")
		}
		if *base.Livereload.Image != *original.Livereload.Image {
			t.Errorf("Expected Image to remain unchanged")
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:    boolPtr(true),
				Include:    stringPtr("*.go"),
				Exclude:    stringPtr("*.tmp"),
				Protect:    stringPtr("*.secret"),
				Username:   stringPtr("user"),
				Password:   stringPtr("pass"),
				WebhookUrl: stringPtr("https://webhook.example.com"),
				VerifySsl:  boolPtr(true),
				Image:      stringPtr("git-livereload:latest"),
			},
		}
		overlay := &GitConfig{}

		base.Merge(overlay)

		if base.Livereload == nil {
			t.Error("Expected Livereload to remain initialized")
		}
		if *base.Livereload.Enabled != true {
			t.Errorf("Expected Enabled to remain true")
		}
		if *base.Livereload.Include != "*.go" {
			t.Errorf("Expected Include to remain '*.go'")
		}
		if *base.Livereload.Exclude != "*.tmp" {
			t.Errorf("Expected Exclude to remain '*.tmp'")
		}
		if *base.Livereload.Protect != "*.secret" {
			t.Errorf("Expected Protect to remain '*.secret'")
		}
		if *base.Livereload.Username != "user" {
			t.Errorf("Expected Username to remain 'user'")
		}
		if *base.Livereload.Password != "pass" {
			t.Errorf("Expected Password to remain 'pass'")
		}
		if *base.Livereload.WebhookUrl != "https://webhook.example.com" {
			t.Errorf("Expected WebhookUrl to remain 'https://webhook.example.com'")
		}
		if *base.Livereload.VerifySsl != true {
			t.Errorf("Expected VerifySsl to remain true")
		}
		if *base.Livereload.Image != "git-livereload:latest" {
			t.Errorf("Expected Image to remain 'git-livereload:latest'")
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:    boolPtr(false),
				Include:    stringPtr("*.go"),
				Exclude:    stringPtr("*.tmp"),
				Protect:    stringPtr("*.secret"),
				Username:   stringPtr("olduser"),
				Password:   stringPtr("oldpass"),
				WebhookUrl: stringPtr("https://old-webhook.example.com"),
				VerifySsl:  boolPtr(false),
				Image:      stringPtr("old-git-livereload:latest"),
			},
		}
		overlay := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:    boolPtr(true),
				Include:    stringPtr("*.js"),
				Exclude:    stringPtr("*.log"),
				Protect:    stringPtr("*.key"),
				Username:   stringPtr("newuser"),
				Password:   stringPtr("newpass"),
				WebhookUrl: stringPtr("https://new-webhook.example.com"),
				VerifySsl:  boolPtr(true),
				Image:      stringPtr("new-git-livereload:latest"),
			},
		}

		base.Merge(overlay)

		if base.Livereload == nil {
			t.Error("Expected Livereload to be initialized")
		}
		if *base.Livereload.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Livereload.Enabled)
		}
		if *base.Livereload.Include != "*.js" {
			t.Errorf("Expected Include to be '*.js', got %s", *base.Livereload.Include)
		}
		if *base.Livereload.Exclude != "*.log" {
			t.Errorf("Expected Exclude to be '*.log', got %s", *base.Livereload.Exclude)
		}
		if *base.Livereload.Protect != "*.key" {
			t.Errorf("Expected Protect to be '*.key', got %s", *base.Livereload.Protect)
		}
		if *base.Livereload.Username != "newuser" {
			t.Errorf("Expected Username to be 'newuser', got %s", *base.Livereload.Username)
		}
		if *base.Livereload.Password != "newpass" {
			t.Errorf("Expected Password to be 'newpass', got %s", *base.Livereload.Password)
		}
		if *base.Livereload.WebhookUrl != "https://new-webhook.example.com" {
			t.Errorf("Expected WebhookUrl to be 'https://new-webhook.example.com', got %s", *base.Livereload.WebhookUrl)
		}
		if *base.Livereload.VerifySsl != true {
			t.Errorf("Expected VerifySsl to be true, got %v", *base.Livereload.VerifySsl)
		}
		if *base.Livereload.Image != "new-git-livereload:latest" {
			t.Errorf("Expected Image to be 'new-git-livereload:latest', got %s", *base.Livereload.Image)
		}
	})

	t.Run("MergeWithNilBaseLivereload", func(t *testing.T) {
		base := &GitConfig{}
		overlay := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:    boolPtr(true),
				Include:    stringPtr("*.go"),
				Exclude:    stringPtr("*.tmp"),
				Protect:    stringPtr("*.secret"),
				Username:   stringPtr("user"),
				Password:   stringPtr("pass"),
				WebhookUrl: stringPtr("https://webhook.example.com"),
				VerifySsl:  boolPtr(true),
				Image:      stringPtr("git-livereload:latest"),
			},
		}

		base.Merge(overlay)

		if base.Livereload == nil {
			t.Error("Expected Livereload to be initialized")
		}
		if *base.Livereload.Enabled != true {
			t.Errorf("Expected Enabled to be true")
		}
		if *base.Livereload.Include != "*.go" {
			t.Errorf("Expected Include to be '*.go'")
		}
		if *base.Livereload.Exclude != "*.tmp" {
			t.Errorf("Expected Exclude to be '*.tmp'")
		}
		if *base.Livereload.Protect != "*.secret" {
			t.Errorf("Expected Protect to be '*.secret'")
		}
		if *base.Livereload.Username != "user" {
			t.Errorf("Expected Username to be 'user'")
		}
		if *base.Livereload.Password != "pass" {
			t.Errorf("Expected Password to be 'pass'")
		}
		if *base.Livereload.WebhookUrl != "https://webhook.example.com" {
			t.Errorf("Expected WebhookUrl to be 'https://webhook.example.com'")
		}
		if *base.Livereload.VerifySsl != true {
			t.Errorf("Expected VerifySsl to be true")
		}
		if *base.Livereload.Image != "git-livereload:latest" {
			t.Errorf("Expected Image to be 'git-livereload:latest'")
		}
	})
}

// TestGitConfig_Copy tests the Copy method of GitConfig
func TestGitConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *GitConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Error("Expected nil copy for nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &GitConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy of empty config")
		}
		if copied.Livereload != nil {
			t.Error("Expected Livereload to be nil in copy")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:    boolPtr(true),
				Include:    stringPtr("*.go"),
				Exclude:    stringPtr("*.tmp"),
				Protect:    stringPtr("*.secret"),
				Username:   stringPtr("user"),
				Password:   stringPtr("pass"),
				WebhookUrl: stringPtr("https://webhook.example.com"),
				VerifySsl:  boolPtr(true),
				Image:      stringPtr("git-livereload:latest"),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied == config {
			t.Error("Expected copy to be a new instance")
		}
		if copied.Livereload == nil {
			t.Error("Expected Livereload to be copied")
		}
		if copied.Livereload == config.Livereload {
			t.Error("Expected Livereload to be a new instance")
		}
		if *copied.Livereload.Enabled != *config.Livereload.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if *copied.Livereload.Include != *config.Livereload.Include {
			t.Errorf("Expected Include to be copied correctly")
		}
		if *copied.Livereload.Exclude != *config.Livereload.Exclude {
			t.Errorf("Expected Exclude to be copied correctly")
		}
		if *copied.Livereload.Protect != *config.Livereload.Protect {
			t.Errorf("Expected Protect to be copied correctly")
		}
		if *copied.Livereload.Username != *config.Livereload.Username {
			t.Errorf("Expected Username to be copied correctly")
		}
		if *copied.Livereload.Password != *config.Livereload.Password {
			t.Errorf("Expected Password to be copied correctly")
		}
		if *copied.Livereload.WebhookUrl != *config.Livereload.WebhookUrl {
			t.Errorf("Expected WebhookUrl to be copied correctly")
		}
		if *copied.Livereload.VerifySsl != *config.Livereload.VerifySsl {
			t.Errorf("Expected VerifySsl to be copied correctly")
		}
		if *copied.Livereload.Image != *config.Livereload.Image {
			t.Errorf("Expected Image to be copied correctly")
		}
	})

	t.Run("CopyWithPartialLivereload", func(t *testing.T) {
		config := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled: boolPtr(true),
				Include: stringPtr("*.go"),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied.Livereload == nil {
			t.Error("Expected Livereload to be copied")
		}
		if *copied.Livereload.Enabled != *config.Livereload.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if *copied.Livereload.Include != *config.Livereload.Include {
			t.Errorf("Expected Include to be copied correctly")
		}
		if copied.Livereload.Exclude != nil {
			t.Error("Expected Exclude to be nil in copy")
		}
		if copied.Livereload.Protect != nil {
			t.Error("Expected Protect to be nil in copy")
		}
		if copied.Livereload.Username != nil {
			t.Error("Expected Username to be nil in copy")
		}
		if copied.Livereload.Password != nil {
			t.Error("Expected Password to be nil in copy")
		}
		if copied.Livereload.WebhookUrl != nil {
			t.Error("Expected WebhookUrl to be nil in copy")
		}
		if copied.Livereload.VerifySsl != nil {
			t.Error("Expected VerifySsl to be nil in copy")
		}
		if copied.Livereload.Image != nil {
			t.Error("Expected Image to be nil in copy")
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:    boolPtr(true),
				Include:    stringPtr("*.go"),
				Exclude:    stringPtr("*.tmp"),
				Protect:    stringPtr("*.secret"),
				Username:   stringPtr("user"),
				Password:   stringPtr("pass"),
				WebhookUrl: stringPtr("https://webhook.example.com"),
				VerifySsl:  boolPtr(true),
				Image:      stringPtr("git-livereload:latest"),
			},
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		*config.Livereload.Enabled = false
		*config.Livereload.Include = "*.js"
		*config.Livereload.Exclude = "*.log"
		*config.Livereload.Protect = "*.key"
		*config.Livereload.Username = "newuser"
		*config.Livereload.Password = "newpass"
		*config.Livereload.WebhookUrl = "https://new-webhook.example.com"
		*config.Livereload.VerifySsl = false
		*config.Livereload.Image = "new-git-livereload:latest"

		if *copied.Livereload.Enabled != true {
			t.Error("Expected copied Enabled to remain independent")
		}
		if *copied.Livereload.Include != "*.go" {
			t.Error("Expected copied Include to remain independent")
		}
		if *copied.Livereload.Exclude != "*.tmp" {
			t.Error("Expected copied Exclude to remain independent")
		}
		if *copied.Livereload.Protect != "*.secret" {
			t.Error("Expected copied Protect to remain independent")
		}
		if *copied.Livereload.Username != "user" {
			t.Error("Expected copied Username to remain independent")
		}
		if *copied.Livereload.Password != "pass" {
			t.Error("Expected copied Password to remain independent")
		}
		if *copied.Livereload.WebhookUrl != "https://webhook.example.com" {
			t.Error("Expected copied WebhookUrl to remain independent")
		}
		if *copied.Livereload.VerifySsl != true {
			t.Error("Expected copied VerifySsl to remain independent")
		}
		if *copied.Livereload.Image != "git-livereload:latest" {
			t.Error("Expected copied Image to remain independent")
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
