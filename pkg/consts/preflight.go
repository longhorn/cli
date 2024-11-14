package consts

const (
	AppNamePreflightChecker              = "longhorn-preflight-checker"
	AppNamePreflightContainerOptimizedOS = "longhorn-gke-cos-node-agent"
	AppNamePreflightInstaller            = "longhorn-preflight-installer"
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
