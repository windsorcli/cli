package constants

// Default git livereload settings
const (
	DEFAULT_GIT_LIVE_RELOAD_IMAGE         = "ghcr.io/windsor-hotel/git-livereload-server:v0.2.1"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE = ".docker-cache,.terraform,data,.venv"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT = "flux-system"
	DEFAULT_GIT_LIVE_RELOAD_USERNAME      = "local"
	DEFAULT_GIT_LIVE_RELOAD_PASSWORD      = "local"
	DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL   = "http://flux-webhook.private.test"
)