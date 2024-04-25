package consts

type OperatingSystem string

const (
	OperatingSystemContainerOptimizedOS OperatingSystem = "cos"
)

const (
	EnvCurrentNodeID  = "CURRENT_NODE_ID"
	EnvKubeConfigPath = "KUBECONFIG"
	EnvLogLevel       = "LOG_LEVEL"
	EnvOutputFilePath = "OUTPUT_FILE_PATH"

	EnvLonghornDataDirectory = "LONGHORN_DATA_DIRECTORY"
	EnvLonghornNamespace     = "LONGHORN_NAMESPACE"
	EnvLonghornReplicaName   = "REPLICA_NAME"
	EnvLonghornVolumeName    = "VOLUME_NAME"
)

// SPDK related environment variables
const (
	EnvDriverOverride    = "DRIVER_OVERRIDE"
	EnvEnableSpdk        = "ENABLE_SPDK"
	EnvHugePageSize      = "HUGEMEM"
	EnvPciAllowed        = "PCI_ALLOWED"
	EnvUioDriver         = "UIO_DRIVER"
	EnvUpdatePackageList = "UPDATE_PACKAGE_LIST"
	EnvSpdkOptions       = "SPDK_OPTIONS"
)
