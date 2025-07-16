package types

// GlobalCmdOptions is the common options for all subcommands.
type GlobalCmdOptions struct {
	LogLevel       string // The log level for the CLI.
	KubeConfigPath string // The path to the kubeconfig file.
	Image          string // The image to use for local interactions.
	NodeSelector   string // The node selector to choose nodes on which to run DaemonSet pods
	Namespace      string // The namespace to run DaemonSet pods
}
