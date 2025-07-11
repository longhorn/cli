package preflight

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	cp "github.com/otiai10/copy"

	"k8s.io/apimachinery/pkg/api/resource"

	kubeclient "k8s.io/client-go/kubernetes"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"
	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
	lhmgrutil "github.com/longhorn/longhorn-manager/util"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"

	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
	remote "github.com/longhorn/cli/pkg/remote/preflight"
	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

// Installer provide functions for the preflight installer.
type Installer struct {
	remote.InstallerCmdOptions

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

// Init initializes the Installer.
func (local *Installer) Init() error {
	local.collection.Log = &types.LogCollection{}

	config, err := commonkube.GetInClusterConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get client config")
	}

	local.kubeClient, err = kubeclient.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "failed to get Kubernetes clientset")
	}

	osRelease, err := utils.GetOSRelease()
	if err != nil {
		return errors.Wrap(err, "failed to get OS release")
	}
	local.osRelease = osRelease
	local.logger = logrus.WithField("os", local.osRelease)

	packageManagerType, err := utils.GetPackageManagerType(osRelease)
	if err != nil {
		logrus.WithError(err).Fatal("failed to get package manager")
	}
	local.logger = local.logger.WithField("package-manager", packageManagerType)

	namespaces := []commontypes.Namespace{
		commontypes.NamespaceMnt,
		commontypes.NamespaceNet,
	}

	executor, err := commonns.NewNamespaceExecutor(commontypes.ProcessSelf, commontypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	kernelRelease, err := executor.Execute([]string{}, "uname", []string{"-r"}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return err
	}
	kernelRelease = strings.TrimRight(kernelRelease, "\n")

	pkgMgr, err := pkgmgr.New(packageManagerType, executor)
	if err != nil {
		return err
	}

	switch packageManagerType {
	case pkgmgr.PackageManagerApt:
		local.packageManager = pkgMgr
		local.packages = []string{
			"nfs-common", "open-iscsi", "cryptsetup",
		}
		local.modules = []string{
			"nfs", "dm_crypt",
		}
		local.services = []string{
			"iscsid",
		}
		local.spdkDepPackages = []string{
			"linux-modules-extra-" + kernelRelease,
		}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}
		return nil

	case pkgmgr.PackageManagerYum:
		local.packageManager = pkgMgr
		local.packages = []string{
			"nfs-utils", "iscsi-initiator-utils", "cryptsetup",
		}
		local.modules = []string{
			"nfs", "iscsi_tcp", "dm_crypt",
		}
		local.services = []string{
			"iscsid",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}
		return nil

	case pkgmgr.PackageManagerZypper, pkgmgr.PackageManagerTransactionalUpdate:
		local.packageManager = pkgMgr
		local.packages = []string{
			"nfs-client", "open-iscsi", "cryptsetup",
		}
		local.modules = []string{
			"nfs", "iscsi_tcp", "dm_crypt",
		}
		local.services = []string{
			"iscsid",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}
		return nil

	case pkgmgr.PackageManagerPacman:
		local.packageManager = pkgMgr
		local.packages = []string{
			"nfs-utils", "open-iscsi", "cryptsetup",
		}
		local.modules = []string{
			"nfs", "iscsi_tcp", "dm_crypt",
		}
		local.services = []string{
			"iscsid",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}
		return nil

	default:
		return errors.Errorf("Operating system (%v) package manager (%s) is not supported", osRelease, packageManagerType)
	}
}

func (local *Installer) Run() error {
	var rebootRequired bool
	var err error

	if local.UpdatePackages {
		if err := local.updatePackageList(); err != nil {
			return err
		}
	}

	rebootRequired, err = local.checkAndinstallPackages(local.EnableSpdk)
	if err != nil {
		return err
	}

	if rebootRequired {
		logrus.Warn("Need to reboot the system and execute longhornctl install preflight again")
		local.collection.Log.Warn = append(local.collection.Log.Warn, "Need to reboot the system and execute longhornctl install preflight again")
		return nil
	}

	if err := local.probeModules(consts.DependencyModuleDefault); err != nil {
		return err
	}

	if err := local.startServices(); err != nil {
		return err
	}

	if local.EnableSpdk {
		if err := local.probeModules(consts.DependencyModuleSpdk); err != nil {
			return err
		}

		if err := local.configureSPDKEnv(); err != nil {
			return err
		}

		// At this point, HugePages size is updated on the node, but won't be visible to the k8s cluster until kubelet is restarted.
		if err := local.restartKubelet(); err != nil {
			return err
		}
	}

	return nil
}

// Output converts the collection to JSON and output to stdout or the output file.
func (local *Installer) Output() error {
	local.logger.Trace("Outputting preflight checks results")

	jsonBytes, err := json.Marshal(local.collection)
	if err != nil {
		return errors.Wrap(err, "failed to convert collection to JSON")
	}

	return utils.HandleResult(jsonBytes, local.OutputFilePath, local.logger)
}

// startServices starts services.
func (local *Installer) startServices() error {
	for _, svc := range local.services {
		logrus.Infof("Starting service %s", svc)

		_, err := local.packageManager.StartService(svc)
		if err != nil {
			return errors.Wrapf(err, "failed to start service %s", svc)
		}

		logrus.Infof("Successfully started service %s", svc)
		local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Successfully started service %s", svc))
	}

	return nil
}

// probeModules probes kernel modules.
func (local *Installer) probeModules(dependencyModule consts.DependencyModuleType) error {
	var modules []string
	switch dependencyModule {
	case consts.DependencyModuleSpdk:
		modules = local.spdkDepModules
	case consts.DependencyModuleDefault:
		modules = local.modules
	default:
		return errors.Errorf("dependency module type (%d) is not supported", dependencyModule)
	}
	for _, mod := range modules {
		logrus.Infof("Probing module %s", mod)

		_, err := local.packageManager.Modprobe(mod)
		if err != nil {
			return errors.Wrapf(err, "failed to probe module %s", mod)
		}

		logrus.Infof("Successfully probed module %s", mod)
		local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Successfully probed module %s", mod))
	}

	return nil
}

// checkAndinstallPackages check and installs packages with a package manager.
func (local *Installer) checkAndinstallPackages(spdkDependent bool) (bool, error) {
	var rebootRequired = false

	_, err := local.packageManager.StartPackageSession()
	if err != nil {
		return false, errors.Wrap(err, "failed to start package session")
	}

	packages := local.packages
	if spdkDependent {
		packages = append(packages, local.spdkDepPackages...)
	}
	for _, pkg := range packages {
		logrus.Infof("Checking package %s", pkg)

		_, err := local.packageManager.CheckPackageInstalled(pkg)
		if err != nil {
			logrus.Infof("Installing package %s", pkg)

			_, err := local.packageManager.InstallPackage(pkg)
			if err != nil {
				return false, errors.Wrapf(err, "failed to install package %s", pkg)
			} else {
				logrus.Infof("Successfully installed package %s", pkg)
				local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Successfully installed package %s", pkg))

				if local.packageManager.NeedReboot() {
					rebootRequired = true
				}
			}
		} else {
			logrus.Infof("Package %s already installed", pkg)
		}
	}

	return rebootRequired, nil
}

// updatePackageList updates list of available packages.
func (local *Installer) updatePackageList() error {
	logrus.Info("Updating package list")
	_, err := local.packageManager.UpdatePackageList()
	if err != nil {
		return errors.Wrapf(err, "failed to update package list")
	}

	logrus.Info("Successfully updated package list")
	return nil
}

// configureSPDKEnv configures SPDK environment.
func (local *Installer) configureSPDKEnv() error {
	// Blindly remove the SPDK source code directory if it exists.
	spdkPath := filepath.Join(consts.VolumeMountHostDirectory, consts.SpdkPath)
	if err := os.RemoveAll(spdkPath); err != nil {
		return err
	}
	if err := cp.Copy("/spdk", spdkPath); err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(spdkPath)
	}()

	// Configure SPDK environment
	logrus.Info("Configuring SPDK environment")
	args := getArgsForConfiguringSPDKEnv(local.SpdkOptions)
	if _, err := local.packageManager.Execute([]string{}, "bash", args, commontypes.ExecuteNoTimeout); err != nil {
		logrus.WithError(err).Error("Failed to configure SPDK environment")
	} else {
		logrus.Info("Successfully configured SPDK environment")
		local.collection.Log.Info = append(local.collection.Log.Info, "Successfully configured SPDK environment")
	}

	return nil
}

func getArgsForConfiguringSPDKEnv(options string) []string {
	args := []string{filepath.Join(consts.SpdkPath, "scripts/setup.sh")}
	if options != "" {
		logrus.Infof("Configuring SPDK environment with custom options: %v", options)
		customOptions := strings.Split(options, consts.CmdOptSeperator)
		args = append(args, customOptions...)
	}
	return args
}

func (local *Installer) restartKubelet() error {
	if !local.RestartKubelet {
		return nil
	}

	currentHugePagesCapacity, err := kubeutils.GetHugePagesCapacity(local.kubeClient)
	if err != nil {
		return err
	}
	requiredHugePagesCapacity := resource.NewQuantity(int64(local.HugePageSize*lhmgrutil.MiB), resource.BinarySI)

	if currentHugePagesCapacity.Cmp(*requiredHugePagesCapacity) < 0 {
		logrus.Infof("K8s node CR doesn't have enough hugepages-2Mi capacity. Required: %v, Current: %v", requiredHugePagesCapacity, currentHugePagesCapacity)

		restartWindow, err := time.ParseDuration(local.RestartKubeletWindow)
		if err != nil {
			return errors.Wrapf(err, "failed to parse %q argument", consts.CmdOptRestartKubeletWindow)
		}
		var restartDelay time.Duration
		if restartWindow > 0 {
			restartDelay = time.Duration(rand.Int63n(int64(restartWindow)))
		}
		logrus.Infof("Restarting kubelet service after %s", restartDelay)
		time.Sleep(restartDelay)

		return local.restartKubeletService()
	}

	return nil
}

func (local *Installer) restartKubeletService() error {
	// Kubelet may be managed by different services depending on the k8s distribution
	serviceCandidates := []string{"kubelet", "k3s", "rke2-server"}
	var restartErr error

	for _, svc := range serviceCandidates {
		_, restartErr = local.packageManager.RestartService(svc)
		if restartErr == nil {
			logrus.Infof("Successfully restarted service %s", svc)
			local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Successfully restarted service %s", svc))
			return nil
		}
		// If error is not "not found", stop trying further services
		if !pkgmgr.ServiceNotFoundRegex.MatchString(restartErr.Error()) {
			break
		}
		// else continue trying next service
	}

	logrus.Errorf("Failed to restart kubelet service: %v", restartErr)
	local.collection.Log.Error = append(local.collection.Log.Error, fmt.Sprintf("Failed to restart kubelet service: %s", restartErr))
	return restartErr
}
