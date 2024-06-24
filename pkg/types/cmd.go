package types

// GlobalCmdOptions is the common options for all subcommands.
type GlobalCmdOptions struct {
	LogLevel       string // The log level for the CLI.
	KubeConfigPath string // The path to the kubeconfig file.
	Image          string // The image to use for local interactions.
}
