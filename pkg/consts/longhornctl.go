package consts

import (
	"fmt"

	"github.com/longhorn/cli/meta"
)

const (
	ImageBciBase = "registry.suse.com/bci/bci-base:15.6"
	ImagePause   = "registry.k8s.io/pause:3.1"
)

var (
	ImageEngine      = fmt.Sprintf("longhornio/longhorn-engine:%s", meta.Version)
	ImageLonghornCli = fmt.Sprintf("longhornio/longhorn-cli:%s", meta.Version)
)

const (
	ContainerName       = "longhornctl"
	ContainerNameEngine = "engine"
	ContainerNameInit   = "init-longhornctl"
	ContainerNameOutput = "output-longhornctl"
	ContainerNamePause  = "pause"
)

const (
	ContainerConditionMaxTolerationLong   = 60 * 10 // 10 minutes: for container responsible for long running tasks. For example: package installation.
	ContainerConditionMaxTolerationMedium = 60 * 5  // 5 minutes: for container responsible for medium running tasks. For example: export replica.
	ContainerConditionMaxTolerationShort  = 60      // 1 minute: for container responsible for short running tasks. For example: print file contents.
)

const (
	VolumeMountHostName      = "host"
	VolumeMountHostDirectory = "/host"

	VolumeMountSharedName      = "shared"
	VolumeMountSharedDirectory = "/shared"

	VolumeMountHostExporterName      = "host-exporter"
	VolumeMountHostExporterDirectory = "/host-exporter"

	VolumeMountEntrypointName      = "entrypoint"
	VolumeMountEntrypointDirectory = "/scripts"

	VolumeMountVolumeName      = "volume"
	VolumeMountVolumeDirectory = "/volume"
)

const (
	FileNamePreStopScript = "pre-stop.sh"
	FileNameOutputJSON    = "output.json"
)

const (
	LogPrefixError = "ERROR: "
	LogPrefixWarn  = "WARN: "
)
