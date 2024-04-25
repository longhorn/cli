package consts

const (
	AppNamePreflightChecker              = "longhorn-preflight-checker"
	AppNamePreflightContainerOptimizedOS = "longhorn-gke-cos-node-agent"
	AppNamePreflightInstaller            = "longhorn-preflight-installer"
)

type DependencyModuleType int

const (
	DependencyModuleDefault DependencyModuleType = iota
	DependencyModuleSpdk
)
