package consts

const (
	// Binary names
	CmdLonghornctlLocal  = "longhornctl-local"
	CmdLonghornctlRemote = "longhornctl"
)

const (
	// The first layer of subcommands (verb)
	SubCmdCheck   = "check"
	SubCmdExport  = "export"
	SubCmdGet     = "get"
	SubCmdInstall = "install"
	SubCmdTrim    = "trim"

	// The second layer of subcommands (noun)
	SubCmdPreflight = "preflight"
	SubCmdReplica   = "replica"
	SubCmdVolume    = "volume"

	// The third layer of subcommands (action to the previous layers)
	SubCmdStop = "stop"

	// Other subcommands
	SubCmdVersion = "version"
)

const (
	// Global options
	CmdOptKubeConfigPath = "kube-config"
	CmdOptLogLevel       = "log-level"
	CmdOptImage          = "image"

	// General options
	CmdOptName            = "name"
	CmdOptNodeId          = "node-id"
	CmdOptOperatingSystem = "operating-system"
	CmdOptOutputFile      = "output-file"
	CmdOptTargetDirectory = "target-dir"
	CmdOptUpdatePackages  = "update-packages"
	CmdOptNodeSelector    = "node-selector"

	// SPDK options
	CmdOptAllowPci        = "allow-pci"
	CmdOptDriverOverride  = "driver-override"
	CmdOptEnableSpdk      = "enable-spdk"
	CmdOptHugePageSize    = "huge-page-size"
	CmdOptSpdkOptions     = "spdk-options"
	CmdOptUserspaceDriver = "userspace-driver"

	// Longhorn options
	CmdOptLonghornDataDirectory = "data-dir"
	CmdOptLonghornEngineImage   = "engine-image"
	CmdOptLonghornNamespace     = "longhorn-namespace"
	CmdOptLonghornVolumeName    = "volume-name"
)

const CmdOptSeperator = ","
