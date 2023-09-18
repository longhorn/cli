package installer

import (
	"fmt"

	lhns "github.com/longhorn/go-common-libs/namespace"
	lhtypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/longhorn-preflight/pkg/installer/apt"
	"github.com/longhorn/longhorn-preflight/pkg/installer/command"
	"github.com/longhorn/longhorn-preflight/pkg/types"
)

type Installer struct {
	name    types.PackageManager
	command command.CommandInterface

	packages       []string
	pythonPackages []string
	modules        []string
}

func NewInstaller(packageManager types.PackageManager) (*Installer, error) {
	namespaces := []lhtypes.Namespace{
		lhtypes.NamespaceMnt,
		lhtypes.NamespaceNet,
	}

	executor, err := lhns.NewNamespaceExecutor(lhtypes.ProcessSelf, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return nil, err
	}

	switch packageManager {
	case types.PackageManagerApt:
		return &Installer{
			name:    types.PackageManagerApt,
			command: apt.NewCommand(executor),
			packages: []string{
				"nfs-common", "open-iscsi", "nvme-cli",
			},
			pythonPackages: []string{
				"ninja", "meson", "pyelftools", "ijson", "python-magic", "grpcio", "grpcio-tools", "pyyaml",
			},
			modules: []string{
				"nfs",
				"nvme-tcp",
			},
		}, nil
	case types.PackageManagerYum:
		return &Installer{
			name:    types.PackageManagerYum,
			command: nil,
			packages: []string{
				"nfs-utils", "iscsi-initiator-utils", "nvme-cli",
			},
			pythonPackages: []string{},
			modules: []string{
				"nfs", "iscsi_tcp", "nvme-tcp",
			},
		}, nil
	case types.PackageManagerZypper:
		return &Installer{
			name:    types.PackageManagerZypper,
			command: nil,
			packages: []string{
				"nfs-client", "open-iscsi", "nvme-cli",
			},
			pythonPackages: []string{},
			modules: []string{
				"nfs", "iscsi_tcp", "nvme-tcp",
			},
		}, nil
	case types.PackageManagerApk:
		return &Installer{
			name:           types.PackageManagerApk,
			command:        nil,
			packages:       []string{},
			pythonPackages: []string{},
			modules:        []string{},
		}, nil
	case types.PackageManagerPacman:
		return &Installer{
			name:           types.PackageManagerPacman,
			command:        nil,
			packages:       []string{},
			pythonPackages: []string{},
			modules:        []string{},
		}, nil
	default:
		return nil, fmt.Errorf("unknown package manager %s", packageManager)
	}
}
