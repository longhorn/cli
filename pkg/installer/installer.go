package installer

import (
	"fmt"
	"strings"

	lhns "github.com/longhorn/go-common-libs/namespace"
	lhtypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/longhorn-preflight/pkg/command"
	"github.com/longhorn/longhorn-preflight/pkg/packagemanager/apt"
	"github.com/longhorn/longhorn-preflight/pkg/packagemanager/pacman"
	"github.com/longhorn/longhorn-preflight/pkg/packagemanager/yum"
	"github.com/longhorn/longhorn-preflight/pkg/packagemanager/zypper"
	"github.com/longhorn/longhorn-preflight/pkg/types"
)

type Installer struct {
	name    types.PackageManager
	command command.CommandInterface

	packages        []string
	modules         []string
	services        []string
	spdkDepPackages []string
	spdkDepModules  []string
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

	kernelRelease, err := executor.Execute("uname", []string{"-r"}, lhtypes.ExecuteNoTimeout)
	if err != nil {
		return nil, err
	}
	kernelRelease = strings.TrimRight(kernelRelease, "\n")

	switch packageManager {
	case types.PackageManagerApt:
		return &Installer{
			name:    types.PackageManagerApt,
			command: apt.NewCommand(executor),
			packages: []string{
				"nfs-common", "open-iscsi",
			},
			modules: []string{
				"nfs",
			},
			services: []string{},
			spdkDepPackages: []string{
				"linux-modules-extra-" + kernelRelease,
			},
			spdkDepModules: []string{
				"nvme-tcp",
			},
		}, nil
	case types.PackageManagerYum:
		return &Installer{
			name:    types.PackageManagerYum,
			command: yum.NewCommand(executor),
			packages: []string{
				"nfs-utils", "iscsi-initiator-utils",
			},
			modules: []string{
				"nfs", "iscsi_tcp",
			},
			services: []string{
				"iscsid",
			},
			spdkDepPackages: []string{},
			spdkDepModules: []string{
				"nvme-tcp",
			},
		}, nil
	case types.PackageManagerZypper:
		return &Installer{
			name:    types.PackageManagerZypper,
			command: zypper.NewCommand(executor),
			packages: []string{
				"nfs-client", "open-iscsi",
			},
			modules: []string{
				"nfs", "iscsi_tcp",
			},
			services: []string{
				"iscsid",
			},
			spdkDepPackages: []string{},
			spdkDepModules: []string{
				"nvme-tcp",
			},
		}, nil
	case types.PackageManagerPacman:
		return &Installer{
			name:    types.PackageManagerPacman,
			command: pacman.NewCommand(executor),
			packages: []string{
				"nfs-utils", "open-iscsi",
			},
			modules: []string{
				"nfs", "iscsi_tcp",
			},
			services: []string{
				"iscsid",
			},
			spdkDepPackages: []string{},
			spdkDepModules: []string{
				"nvme-tcp",
			},
		}, nil
	default:
		return nil, fmt.Errorf("unknown package manager %s", packageManager)
	}
}
