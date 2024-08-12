package preflight

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	lhgokube "github.com/longhorn/go-common-libs/kubernetes"
	lhgons "github.com/longhorn/go-common-libs/ns"
	lhgotypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/cli/pkg/consts"
	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
	remote "github.com/longhorn/cli/pkg/remote/preflight"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

// Checker provide functions for the preflight checker.
type Checker struct {
	remote.CheckerCmdOptions

	logger *logrus.Entry

	OutputFilePath string

	kubeClient *kubeclient.Clientset

	osRelease      string
	packageManager pkgmgr.PackageManager

	packages        []string
	modules         []string
	services        []string
	spdkDepPackages []string
	spdkDepModules  []string

	collection types.NodeCollection
}

// Init initializes the Checker.
func (local *Checker) Init() error {
	local.collection.Log = &types.LogCollection{}

	osRelease, err := utils.GetOSRelease()
	if err != nil {
		return errors.Wrap(err, "failed to get OS release")
	}
	local.osRelease = osRelease
	local.logger = logrus.WithField("os", local.osRelease)

	if local.osRelease == fmt.Sprint(consts.OperatingSystemContainerOptimizedOS) {
		config, err := lhgokube.GetInClusterConfig()
		if err != nil {
			return errors.Wrap(err, "failed to get client config")
		}

		local.kubeClient, err = kubeclient.NewForConfig(config)
		if err != nil {
			return errors.Wrap(err, "failed to get Kubernetes clientset")
		}

		return nil
	}

	packageManagerType, err := utils.GetPackageManagerType(osRelease)
	if err != nil {
		return errors.Wrap(err, "failed to get package manager")
	}
	local.logger = local.logger.WithField("package-manager", packageManagerType)

	namespaces := []lhgotypes.Namespace{
		lhgotypes.NamespaceMnt,
		lhgotypes.NamespaceNet,
	}

	executor, err := lhgons.NewNamespaceExecutor(lhgotypes.ProcessSelf, lhgotypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	packageManager, err := pkgmgr.New(packageManagerType, executor)
	if err != nil {
		return err
	}

	switch packageManagerType {
	case pkgmgr.PackageManagerApt:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-common", "open-iscsi",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	case pkgmgr.PackageManagerYum:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-utils", "iscsi-initiator-utils",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	case pkgmgr.PackageManagerZypper, pkgmgr.PackageManagerTransactionalUpdate:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-client", "open-iscsi",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	case pkgmgr.PackageManagerPacman:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-utils", "open-iscsi",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	default:
		return errors.Errorf("operating system (%v) package manager (%s) is not supported", osRelease, packageManagerType)
	}

	return nil
}

// Run executes the preflight checks.
func (local *Checker) Run() error {
	switch local.osRelease {
	case fmt.Sprint(consts.OperatingSystemContainerOptimizedOS):
		logrus.Infof("Checking preflight for %v", consts.OperatingSystemContainerOptimizedOS)
		if err := local.checkContainerOptimizedOS(); err != nil {
			return err
		}
	default:
		if err := local.checkIscsidService(); err != nil {
			return err
		}

		if err := local.checkMultipathService(); err != nil {
			return err
		}

		if err := local.checkNFSv4Support(); err != nil {
			return err
		}

		if err := local.checkPackagesInstalled(false); err != nil {
			return err
		}

		if err := local.checkModulesLoaded(false); err != nil {
			return err
		}

		if local.EnableSpdk {
			instructionSets := map[string][]string{
				"amd64": {"sse4_2"},
			}

			if err := local.checkCpuInstructionSet(instructionSets); err != nil {
				return err
			}

			if err := local.checkHugePages(); err != nil {
				return err
			}

			if err := local.checkPackagesInstalled(true); err != nil {
				return err
			}

			if err := local.checkModulesLoaded(true); err != nil {
				return err
			}
		}
	}

	return nil
}

// Output converts the collection to JSON and output to stdout or the output file.
func (local *Checker) Output() error {
	local.logger.Tracef("Outputting preflight checks results")

	jsonBytes, err := json.Marshal(local.collection)
	if err != nil {
		return errors.Wrap(err, "failed to convert collection to JSON")
	}

	return utils.HandleResult(jsonBytes, local.OutputFilePath, local.logger)
}

// checkContainerOptimizedOS checks if the node-agent DaemonSet is running.
func (local *Checker) checkContainerOptimizedOS() error {
	daemonSet, err := lhgokube.GetDaemonSet(local.kubeClient, metav1.NamespaceDefault, consts.AppNamePreflightContainerOptimizedOS)
	if err != nil {
		return errors.Wrapf(err, "failed to get DaemonSet %v", consts.AppNamePreflightContainerOptimizedOS)
	}

	if !lhgokube.IsDaemonSetReady(daemonSet) {
		return errors.Errorf("DaemonSet %v is not ready", consts.AppNamePreflightContainerOptimizedOS)
	}
	return nil
}

// checkMultipathService checks if the multipathd service is running.
func (local *Checker) checkMultipathService() error {
	logrus.Info("Checking multipathd service status")

	_, err := local.packageManager.GetServiceStatus("multipathd.service")
	if err == nil {
		local.collection.Log.Warn = append(local.collection.Log.Warn, "multipathd.service is running. Please refer to https://longhorn.io/kb/troubleshooting-volume-with-multipath/ for more information.")
		return nil
	}

	_, err = local.packageManager.GetServiceStatus("multipathd.socket")
	if err == nil {
		local.collection.Log.Warn = append(local.collection.Log.Warn, "multipathd.service is inactive, but it can still be activated by multipathd.socket")
		return nil
	}

	return nil
}

// checkIscsidService checks if the iscsid service is running.
func (local *Checker) checkIscsidService() error {
	logrus.Info("Checking iscsid service status")

	_, err := local.packageManager.GetServiceStatus("iscsid.service")
	if err == nil {
		local.collection.Log.Info = append(local.collection.Log.Info, "Service iscsid is running")
		return nil
	}

	_, err = local.packageManager.GetServiceStatus("iscsid.socket")
	if err == nil {
		local.collection.Log.Info = append(local.collection.Log.Info, "Service iscsid is inactive, but it can still be activated by iscsid.socket")
		return nil
	}

	local.collection.Log.Error = append(local.collection.Log.Error, "Neither iscsid.service nor iscsid.socket is running")
	return nil
}

// checkHugePages checks if HugePages is enabled.
func (local *Checker) checkHugePages() error {
	logrus.Info("Checking if HugePages is enabled")

	if local.HugePageSize == 0 {
		logrus.Error("HUGEMEM environment variable is not set")
		return nil
	}

	pages := local.HugePageSize >> 1

	ok, hugePagesTotalNum, requiredHugePages, err := local.isHugePagesTotalEqualOrLargerThan(pages)
	if err != nil {
		return errors.Wrapf(err, "failed to check HugePages")
	}
	if !ok {
		local.collection.Log.Error = append(local.collection.Log.Error,
			fmt.Sprintf("HugePages is insufficient. Required 2MiB HugePages: %v pages, Total 2MiB HugePages: %v pages", requiredHugePages, hugePagesTotalNum))
		return nil
	}

	local.collection.Log.Info = append(local.collection.Log.Info, "HugePages is enabled")
	return nil
}

func (local *Checker) isHugePagesTotalEqualOrLargerThan(requiredHugePages int) (bool, int, int, error) {
	output, err := local.packageManager.Execute([]string{}, "grep", []string{"HugePages_Total", "/proc/meminfo"}, lhgotypes.ExecuteNoTimeout)
	if err != nil {
		return false, 0, 0, errors.Wrap(err, "failed to get total number of HugePages")
	}
	line := strings.Split(output, "\n")[0]
	hugePagesTotal := strings.TrimSpace(strings.Split(line, ":")[1])

	hugePagesTotalNum, err := strconv.Atoi(hugePagesTotal)
	if err != nil {
		return false, 0, 0, errors.Wrap(err, "failed to convert HugePages total to a number")
	}

	return hugePagesTotalNum >= requiredHugePages, hugePagesTotalNum, requiredHugePages, nil
}

// CheckCpuInstructionSet checks if the CPU instruction set is supported.
func (local *Checker) checkCpuInstructionSet(instructionSets map[string][]string) error {
	logrus.Info("Checking CPU instruction set")

	arch := runtime.GOARCH
	logrus.Infof("Detected CPU architecture: %v", arch)

	sets, ok := instructionSets[arch]
	if !ok {
		local.collection.Log.Error = append(local.collection.Log.Error, fmt.Sprintf("CPU model is not supported: %v", arch))
		return nil
	}

	for _, set := range sets {
		_, err := local.packageManager.Execute([]string{}, "grep", []string{set, "/proc/cpuinfo"}, lhgotypes.ExecuteNoTimeout)
		if err != nil {
			local.collection.Log.Error = append(local.collection.Log.Error, fmt.Sprintf("CPU instruction set %v is not supported: %s", set, err))
		} else {
			local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("CPU instruction set %v is supported", set))
		}
	}

	return nil
}

// checkPackagesInstalled checks if the packages are installed.
func (local *Checker) checkPackagesInstalled(spdkDependent bool) error {
	packages := local.packages
	if spdkDependent {
		packages = local.spdkDepPackages
	}

	if len(packages) == 0 {
		return nil
	}

	logrus.Info("Checking if required packages are installed")

	for _, pkg := range packages {
		_, err := local.packageManager.CheckPackageInstalled(pkg)
		if err != nil {
			local.collection.Log.Error = append(local.collection.Log.Error, fmt.Sprintf("Package %s is not installed: %s", pkg, err))
		} else {
			local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Package %s is installed", pkg))
		}
	}
	return nil
}

// checkModulesLoaded checks if the modules are loaded.
func (local *Checker) checkModulesLoaded(spdkDependent bool) error {
	modules := local.modules
	if spdkDependent {
		modules = local.spdkDepModules

		if local.UserspaceDriver != "" {
			modules = append(modules, local.UserspaceDriver)
		}
	}

	if len(modules) == 0 {
		return nil
	}

	logrus.Info("Checking if required modules are loaded")

	for _, mod := range modules {
		logrus.Infof("Checking if module %s is loaded", mod)

		err := local.packageManager.CheckModLoaded(mod)
		if err != nil {
			local.collection.Log.Error = append(local.collection.Log.Error, fmt.Sprintf("Module %s is not loaded: %s", mod, err))
		} else {
			local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Module %s is loaded", mod))
		}
	}
	return nil
}

// checkNFSv4Support checks if NFS4 is supported on the host.
func (local *Checker) checkNFSv4Support() error {
	logrus.Info("Checking if NFS4 (either 4.0, 4.1 or 4.2) is supported")

	kernelVersion, err := utils.GetKernelVersion()
	if err != nil {
		return err
	}
	kernelConfigPath := "/boot/config-" + kernelVersion
	kernelConfigPath = filepath.Join(consts.VolumeMountHostDirectory, kernelConfigPath)
	configFile, err := os.Open(kernelConfigPath)
	if err != nil {
		return err
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
					local.collection.Log.Info = append(local.collection.Log.Info, "NFS4 is supported")
					return nil
				} else if option[1] == "m" {
					// Check if the module is loaded
					moduleLoaded, err := utils.IsModuleLoaded(option[0])
					if err != nil {
						continue
					}
					if moduleLoaded {
						local.collection.Log.Info = append(local.collection.Log.Info, "NFS4 is supported")
						return nil
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to check NFS4 support")
	}

	local.collection.Log.Error = append(local.collection.Log.Error, "NFS4 is not supported")
	return nil
}
