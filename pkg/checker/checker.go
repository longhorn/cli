package checker

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-preflight/pkg/pkgmgr"
	"github.com/longhorn/longhorn-preflight/pkg/utils"
)

type Checker struct {
	pkgMgr pkgmgr.PackageManager

	packages        []string
	modules         []string
	services        []string
	spdkDepPackages []string
	spdkDepModules  []string
}

func NewChecker(pkgMgrType pkgmgr.PackageManagerType) (*Checker, error) {
	namespaces := []lhtypes.Namespace{
		lhtypes.NamespaceMnt,
		lhtypes.NamespaceNet,
	}

	executor, err := lhns.NewNamespaceExecutor(lhtypes.ProcessSelf, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return nil, err
	}

	pkgMgr, err := pkgmgr.New(pkgMgrType, executor)
	if err != nil {
		return nil, err
	}

	switch pkgMgrType {
	case pkgmgr.PackageManagerApt:
		return &Checker{
			pkgMgr: pkgMgr,
			packages: []string{
				"nfs-common", "open-iscsi",
			},
			modules: []string{},
			services: []string{
				"multipathd.service",
			},
			spdkDepPackages: []string{},
			spdkDepModules: []string{
				"nvme_tcp",
			},
		}, nil

	case pkgmgr.PackageManagerYum:
		return &Checker{
			pkgMgr: pkgMgr,
			packages: []string{
				"nfs-utils", "iscsi-initiator-utils",
			},
			modules: []string{},
			services: []string{
				"multipathd.service",
			},
			spdkDepPackages: []string{},
			spdkDepModules: []string{
				"nvme_tcp",
			},
		}, nil

	case pkgmgr.PackageManagerZypper:
		return &Checker{
			pkgMgr: pkgMgr,
			packages: []string{
				"nfs-client", "open-iscsi",
			},
			modules: []string{},
			services: []string{
				"multipathd.service",
			},
			spdkDepPackages: []string{},
			spdkDepModules: []string{
				"nvme_tcp",
			},
		}, nil

	case pkgmgr.PackageManagerPacman:
		return &Checker{
			pkgMgr: pkgMgr,
			packages: []string{
				"nfs-utils", "open-iscsi",
			},
			modules: []string{},
			services: []string{
				"multipathd.service",
			},
			spdkDepPackages: []string{
				"nvme-cli",
			},
			spdkDepModules: []string{
				"nvme_tcp",
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown package manager %s", pkgMgrType)
	}
}

// CheckMultipathService checks if the multipathd service is running
func (c *Checker) CheckMultipathService() {
	logrus.Infof("Checking multipathd service status")

	_, err := c.pkgMgr.GetServiceStatus("multipathd.service")
	if err == nil {
		logrus.Warn("multipathd.service is running. Please refer to https://longhorn.io/kb/troubleshooting-volume-with-multipath/ for more information.")
		return
	}

	_, err = c.pkgMgr.GetServiceStatus("multipathd.socket")
	if err == nil {
		logrus.Warn("multipathd.service is inactive, but it can still be activated by multipathd.socket")
		return
	}
	logrus.Info("Neither multipathd.service nor multipathd.socket is not running")
}

// CheckIscsidService checks if the iscsid service is running
func (c *Checker) CheckIscsidService() {
	logrus.Infof("Checking iscsid service status")

	_, err := c.pkgMgr.GetServiceStatus("iscsid.service")
	if err == nil {
		logrus.Info("iscsid.service is running")
		return
	}

	_, err = c.pkgMgr.GetServiceStatus("iscsid.socket")
	if err == nil {
		logrus.Info("iscsid.service is inactive, but it can still be activated by iscsid.socket")
		return
	}

	logrus.Error("Neither iscsid.service nor iscsid.socket is not running")
}

func (c *Checker) CheckHugePages() {
	logrus.Infof("Checking if HugePages is enabled")

	hugepage := os.Getenv("HUGEMEM")
	if hugepage == "" {
		logrus.Error("HUGEMEM environment variable is not set")
		return
	}

	size, err := strconv.Atoi(hugepage)
	if err != nil {
		logrus.WithError(err).Error("HUGEMEM environment variable is not a number")
		return
	}

	pages := size >> 1

	ok, err := c.isHugePagesTotalEqualOrLargerThan(pages)
	if err != nil {
		logrus.WithError(err).Error("Failed to check HugePages")
		return
	}
	if !ok {
		logrus.Errorf("HugePages is not enabled")
		return
	}

	logrus.Info("HugePages is enabled")
}

func (c *Checker) isHugePagesTotalEqualOrLargerThan(requiredHugePages int) (bool, error) {
	output, err := c.pkgMgr.Execute([]string{}, "grep", []string{"HugePages_Total", "/proc/meminfo"}, lhtypes.ExecuteNoTimeout)
	if err != nil {
		return false, errors.Wrap(err, "failed to get total number of HugePages")
	}
	line := strings.Split(output, "\n")[0]
	hugePagesTotal := strings.TrimSpace(strings.Split(line, ":")[1])

	hugePagesTotalNum, err := strconv.Atoi(hugePagesTotal)
	if err != nil {
		return false, errors.Wrap(err, "failed to convert HugePages total to a number")
	}

	return hugePagesTotalNum >= requiredHugePages, nil
}

func (c *Checker) CheckCpuInstructionSet(instructionSets map[string][]string) {
	logrus.Infof("Checking CPU instruction set")

	arch := runtime.GOARCH
	logrus.Infof("Detected CPU architecture: %v", arch)

	sets, ok := instructionSets[arch]
	if !ok {
		logrus.Errorf("CPU model is not supported")
		return
	}

	for _, set := range sets {
		_, err := c.pkgMgr.Execute([]string{}, "grep", []string{set, "/proc/cpuinfo"}, lhtypes.ExecuteNoTimeout)
		if err != nil {
			logrus.Errorf("CPU instruction set %v is not supported", set)
		} else {
			logrus.Infof("CPU instruction set %v is supported", set)
		}
	}
}

func (c *Checker) CheckPackagesInstalled(spdkDependent bool) {
	packages := c.packages
	if spdkDependent {
		packages = c.spdkDepPackages
	}

	if len(packages) == 0 {
		return
	}

	logrus.Infof("Checking if required packages are installed")

	for _, pkg := range packages {
		_, err := c.pkgMgr.CheckPackageInstalled(pkg)
		if err != nil {
			logrus.WithError(err).Errorf("Package %s is not installed", pkg)
		} else {
			logrus.Infof("Package %s is installed", pkg)
		}
	}
}

func (c *Checker) CheckModulesLoaded(spdkDependent bool) {
	modules := c.modules
	if spdkDependent {
		modules = c.spdkDepModules

		uioDriver := os.Getenv("UIO_DRIVER")
		if uioDriver != "" {
			modules = append(modules, uioDriver)
		}
	}

	if len(modules) == 0 {
		return
	}

	logrus.Infof("Checking if required modules are loaded")

	for _, mod := range modules {
		logrus.Infof("Checking if module %s is loaded", mod)

		err := c.pkgMgr.CheckModLoaded(mod)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to check if module %s is loaded", mod)
		} else {
			logrus.Infof("Module %s is loaded", mod)
		}
	}
}

// CheckNFSv4Support checks if NFS4 is supported
func (c *Checker) CheckNFSv4Support() {
	logrus.Infof("Checking if NFS4 (either 4.0, 4.1 or 4.2) is supported")

	kernelConfigPath := fmt.Sprintf("/host/boot/config-%s", utils.GetKernelVersion())
	configFile, err := os.Open(kernelConfigPath)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to check NFS4 support")
		return
	}
	defer func(configFile *os.File) {
		_ = configFile.Close()
	}(configFile)

	scanner := bufio.NewScanner(configFile)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "CONFIG_NFS_V4_2=") ||
			strings.HasPrefix(line, "CONFIG_NFS_V4_1=") ||
			strings.HasPrefix(line, "CONFIG_NFS_V4=") {
			option := strings.Split(line, "=")
			if len(option) == 2 {
				if option[1] == "y" {
					logrus.Info("NFS4 is supported")
					return
				} else if option[1] == "m" {
					// Check if the module is loaded
					moduleLoaded, err := utils.IsModuleLoaded(option[0])
					if err != nil {
						continue
					}
					if moduleLoaded {
						logrus.Info("NFS4 is supported")
						return
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logrus.WithError(err).Errorf("Failed to check NFS4 support")
		return
	}

	logrus.Error("NFS4 is not supported")
}
