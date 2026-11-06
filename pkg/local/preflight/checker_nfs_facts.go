package preflight

import (
	commonnfs "github.com/longhorn/go-common-libs/nfs"
	commonsys "github.com/longhorn/go-common-libs/sys"

	"github.com/longhorn/cli/pkg/utils"
)

type checkerNFSFacts interface {
	KernelVersion() (string, error)
	BootKernelConfig(directory, version string) (map[string]string, error)
	ProcKernelConfig(directory string) (map[string]string, error)
	ModuleLoaded(name string) (bool, error)
	DefaultNFSVersion(directory string) (int, int, error)
}

type hostCheckerNFSFacts struct{}

func (hostCheckerNFSFacts) KernelVersion() (string, error) { return utils.GetKernelVersion() }

func (hostCheckerNFSFacts) BootKernelConfig(directory, version string) (map[string]string, error) {
	return commonsys.GetBootKernelConfigMap(directory, version)
}

func (hostCheckerNFSFacts) ProcKernelConfig(directory string) (map[string]string, error) {
	return commonsys.GetProcKernelConfigMap(directory)
}

func (hostCheckerNFSFacts) ModuleLoaded(name string) (bool, error) { return utils.IsModuleLoaded(name) }

func (hostCheckerNFSFacts) DefaultNFSVersion(directory string) (int, int, error) {
	return commonnfs.GetSystemDefaultNFSVersion(directory)
}
