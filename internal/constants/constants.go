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

// Default Talos settings
const (
	DEFAULT_TALOS_IMAGE             = "ghcr.io/siderolabs/talos:v1.7.6"
	DEFAULT_TALOS_WORKER_CPU        = 4
	DEFAULT_TALOS_WORKER_RAM        = 4
	DEFAULT_TALOS_CONTROL_PLANE_CPU = 2
	DEFAULT_TALOS_CONTROL_PLANE_RAM = 2
)

// Default AWS settings
const (
	DEFAULT_AWS_LOCALSTACK_IMAGE     = "localstack/localstack:3.8.1"
	DEFAULT_AWS_LOCALSTACK_PRO_IMAGE = "localstack/localstack-pro:3.8.1"
)
