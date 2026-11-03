package cmd

// =============================================================================
// Test Setup
// =============================================================================

// The constants and variables below give the black-box tests in package cmd_test access to
// the context override keys, the commands, and the flag variables. A flag variable is a pointer
// so that a test can set it and read it back.
const (
	ComposerOverridesKey   = composerOverridesKey
	InitProjectHint        = initProjectHint
	ProjectOverridesKey    = projectOverridesKey
	RuntimeOverridesKey    = runtimeOverridesKey
	TestRunnerOverridesKey = testRunnerOverridesKey
)

var (
	ApplyCmd                  = applyCmd
	ApplyKustomizeCmd         = applyKustomizeCmd
	ApplyPruneFlag            = &applyPruneFlag
	ApplyTerraformCmd         = applyTerraformCmd
	BootstrapBlueprint        = &bootstrapBlueprint
	BootstrapCmd              = bootstrapCmd
	BootstrapPlatform         = &bootstrapPlatform
	BootstrapSetFlags         = &bootstrapSetFlags
	BootstrapYes              = &bootstrapYes
	BuildIdCmd                = buildIdCmd
	BundleCmd                 = bundleCmd
	ConfigureCmd              = configureCmd
	ConfigureNetworkCmd       = configureNetworkCmd
	ConfigureNetworkPreflight = &configureNetworkPreflight
	CheckNodeHealthCmd        = checkNodeHealthCmd
	CheckNodeReady            = &checkNodeReady
	DestroyCmd                = destroyCmd
	DestroyConfirm            = &destroyConfirm
	DestroyContinue           = &destroyContinue
	DestroyKustomizeCmd       = destroyKustomizeCmd
	DestroyTerraformCmd       = destroyTerraformCmd
	DownCmd                   = downCmd
	EnvCmd                    = envCmd
	ExecCmd                   = execCmd
	FormatFlag                = &formatFlag
	InitArch                  = &initArch
	InitAwsEndpointURL        = &initAwsEndpointURL
	InitAwsProfile            = &initAwsProfile
	InitBackend               = &initBackend
	InitBlueprint             = &initBlueprint
	InitCmd                   = initCmd
	InitCpu                   = &initCpu
	InitDisk                  = &initDisk
	InitDocker                = &initDocker
	InitEndpoint              = &initEndpoint
	InitGitLivereload         = &initGitLivereload
	InitMemory                = &initMemory
	InitPlatform              = &initPlatform
	InitReset                 = &initReset
	InitSetFlags              = &initSetFlags
	InitVmDriver              = &initVmDriver
	InstallCmd                = installCmd
	K8sEndpoint               = &k8sEndpoint
	NodeHealthNodes           = &nodeHealthNodes
	NodeHealthTimeout         = &nodeHealthTimeout
	NodeHealthVersion         = &nodeHealthVersion
	NodeHealthWaitForReboot   = &nodeHealthWaitForReboot
	PlanCmd                   = planCmd
	PlanJSON                  = &planJSON
	PlanKustomizeCmd          = planKustomizeCmd
	PlanOutput                = &planOutput
	PlanSummary               = &planSummary
	PlanTerraformCmd          = planTerraformCmd
	PushCmd                   = pushCmd
	ShowBlueprintCmd          = showBlueprintCmd
	ShowBlueprintJSON         = &showBlueprintJSON
	ShowBlueprintRaw          = &showBlueprintRaw
	ShowCmd                   = showCmd
	ShowKustomizationCmd      = showKustomizationCmd
	ShowKustomizationJSON     = &showKustomizationJSON
	ShowValuesCmd             = showValuesCmd
	ShowValuesJSON            = &showValuesJSON
	TestCmd                   = testCmd
	UpCmd                     = upCmd
	UpSetFlags                = &upSetFlags
	UpVmDriver                = &upVmDriver
	UpgradeAllowDowngrade     = &upgradeAllowDowngrade
	UpgradeCmd                = upgradeCmd
	UpgradeImage              = &upgradeImage
	UpgradeNodeAddr           = &upgradeNodeAddr
	UpgradeNodeImage          = &upgradeNodeImage
	UpgradeNodeOfflineTimeout = &upgradeNodeOfflineTimeout
	UpgradeNodeRebootMode     = &upgradeNodeRebootMode
	UpgradeNodeTimeout        = &upgradeNodeTimeout
	UpgradeNodes              = &upgradeNodes
	UpgradeRebootMode         = &upgradeRebootMode
	UpgradeSources            = &upgradeSources
	UpgradeStdinIsTerminal    = &upgradeStdinIsTerminal
	UpgradeYes                = &upgradeYes
	Verbose                   = &verbose
	VersionCmd                = versionCmd
)
