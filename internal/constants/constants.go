package constants

// Default git livereload settings
const (
	// renovate: datasource=docker depName=ghcr.io/windsorcli/git-livereload-server
	DEFAULT_GIT_LIVE_RELOAD_IMAGE         = "ghcr.io/windsorcli/git-livereload-server:v0.2.1"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE = ".docker-cache,.terraform,data,.venv"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT = "flux-system"
	DEFAULT_GIT_LIVE_RELOAD_USERNAME      = "local"
	DEFAULT_GIT_LIVE_RELOAD_PASSWORD      = "local"
	DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL   = "http://flux-webhook.private.test"
)

// Default Talos settings
const (
	// renovate: datasource=docker depName=ghcr.io/siderolabs/talos
	DEFAULT_TALOS_IMAGE             = "ghcr.io/siderolabs/talos:v1.7.6"
	DEFAULT_TALOS_WORKER_CPU        = 4
	DEFAULT_TALOS_WORKER_RAM        = 4
	DEFAULT_TALOS_CONTROL_PLANE_CPU = 2
	DEFAULT_TALOS_CONTROL_PLANE_RAM = 2
)

// Default AWS settings
const (
	// renovate: datasource=docker depName=localstack/localstack
	DEFAULT_AWS_LOCALSTACK_IMAGE = "localstack/localstack:3.8.1"
	// renovate: datasource=docker depName=localstack/localstack-pro
	DEFAULT_AWS_LOCALSTACK_PRO_IMAGE = "localstack/localstack-pro:3.8.1"
)

// Default DNS settings
const (
	DEFAULT_DNS_IMAGE = "coredns/coredns:1.11.3"
)

// Default Registry settings
const (
	REGISTRY_DEFAULT_IMAGE = "registry:2.8.3"
)
