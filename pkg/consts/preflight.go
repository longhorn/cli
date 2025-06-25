package consts

const (
	AppNamePreflightChecker              = "longhorn-preflight-checker"
	AppNamePreflightContainerOptimizedOS = "longhorn-gke-cos-node-agent"
	AppNamePreflightInstaller            = "longhorn-preflight-installer"
)

const (
	PreflightCheckTopicContainerOptimizedOS = "ContainerOptimizedOS"
	PreflightCheckTopicMultipathService     = "MultipathService"
	PreflightCheckTopicIscsidService        = "IscsidService"
	PreflightCheckTopicHugePages            = "HugePages"
	PreflightCheckTopicCpuInstructionSet    = "CPUInstructionSet"
	PreflightCheckTopicPackages             = "Packages"
	PreflightCheckTopicKernelModules        = "KernelModules"
	PreflightCheckTopicKubeDNS              = "KubeDNS"
	PreflightCheckTopicNFS                  = "NFSv4"
	PreflightCheckTopicSPDK                 = "SPDK"
	PreflightCheckTopicInternalError        = "InternalError"
)

const (
	KubeAppLabel    = "k8s-app"
	KubeAppValueDNS = "kube-dns"
)

type DependencyModuleType int

const (
	DependencyModuleDefault DependencyModuleType = iota
	DependencyModuleSpdk
)
