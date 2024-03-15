package installer

import (
	"fmt"
	"strings"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/longhorn-preflight/pkg/pkgmgr"
)

type Installer struct {
	pkgMgr pkgmgr.PackageManager

	packages        []string
	modules         []string
	services        []string
	spdkDepPackages []string
	spdkDepModules  []string
}

func NewInstaller(pkgMgrType pkgmgr.PackageManagerType) (*Installer, error) {
	namespaces := []lhtypes.Namespace{
		lhtypes.NamespaceMnt,
		lhtypes.NamespaceNet,
	}

	executor, err := lhns.NewNamespaceExecutor(lhtypes.ProcessSelf, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return nil, err
	}

	kernelRelease, err := executor.Execute([]string{}, "uname", []string{"-r"}, lhtypes.ExecuteNoTimeout)
	if err != nil {
		return nil, err
	}
	kernelRelease = strings.TrimRight(kernelRelease, "\n")

	pkgMgr, err := pkgmgr.New(pkgMgrType, executor)
	if err != nil {
		return nil, err
	}

	switch pkgMgrType {
	case pkgmgr.PackageManagerApt:
		return &Installer{
			pkgMgr: pkgMgr,
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

	case pkgmgr.PackageManagerYum:
		return &Installer{
			pkgMgr: pkgMgr,
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

	case pkgmgr.PackageManagerZypper:
		return &Installer{
			pkgMgr: pkgMgr,
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

	case pkgmgr.PackageManagerPacman:
		return &Installer{
			pkgMgr: pkgMgr,
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
		return nil, fmt.Errorf("unknown package manager %s", pkgMgrType)
	}
}
