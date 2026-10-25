package consts

const (
	// Binary names
	CmdLonghornctlLocal  = "longhornctl-local"
	CmdLonghornctlRemote = "longhornctl"
)

const (
	// The first layer of subcommands (verb)
	SubCmdCheck       = "check"
	SubCmdDiagnose    = "diagnose"
	SubCmdExport      = "export"
	SubCmdGet         = "get"
	SubCmdInstall     = "install"
	SubCmdMaintenance = "maintenance"
	SubCmdTrim        = "trim"

	// The second layer of subcommands (noun)
	SubCmdPreflight = "preflight"
	SubCmdReplica   = "replica"
	SubCmdVolume    = "volume"

	// The third layer of subcommands (action to the previous layers)
	SubCmdStop      = "stop"
	SubCmdEvictNode = "evict-node"
	SubCmdEvictDisk = "evict-disk"

	// Other subcommands
	SubCmdVersion = "version"
)

const (
	// Global options
	CmdOptKubeConfigPath  = "kubeconfig"
	CmdOptLogLevel        = "log-level"
	CmdOptImage           = "image"
	CmdOptImageRegistry   = "image-registry"
	CmdOptImagePullSecret = "image-pull-secret"

	// General options
	CmdOptName            = "name"
	CmdOptNamespace       = "namespace"
	CmdOptNodeId          = "node-id"
	CmdOptOperatingSystem = "operating-system"
	CmdOptOutput          = "output"
	CmdOptOutputFile      = "output-file"
	CmdOptTargetDirectory = "target-dir"
	CmdOptUpdatePackages  = "update-packages"
	CmdOptNodeSelector    = "node-selector"
	CmdOptTolerations     = "tolerations"
	CmdOptAll             = "all"

	// SPDK options
	CmdOptAllowPci             = "allow-pci"
	CmdOptDriverOverride       = "driver-override"
	CmdOptEnableSpdk           = "enable-spdk"
	CmdOptHugePageSize         = "huge-page-size"
	CmdOptSpdkOptions          = "spdk-options"
	CmdOptUserspaceDriver      = "userspace-driver"
	CmdOptRestartKubelet       = "restart-kubelet"
	CmdOptRestartKubeletWindow = "restart-kubelet-window"

	// Longhorn options
	CmdOptLonghornDataDirectory = "data-dir"
	CmdOptLonghornEngineImage   = "engine-image"
	CmdOptLonghornNamespace     = "longhorn-namespace"
	CmdOptLonghornVolumeName    = "volume-name"
	CmdOptReplicaDir            = "replica-dir"
	CmdOptReplicaDryRun         = "dry-run"
	EnvReplicaDirectory         = "REPLICA_DIR"
	SubCmdRecover               = "recover"

	// Maintenance options
	CmdOptDiskUUID = "disk-uuid"
	CmdOptWait     = "wait"
)

const (
	NamespaceLonghorn = "longhorn-system"
)

const CmdOptSeperator = ","
