package preflight

import (
	"os"
	"path/filepath"
	"strings"

	cp "github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/cli/pkg/consts"
	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
	remote "github.com/longhorn/cli/pkg/remote/preflight"
	"github.com/longhorn/cli/pkg/utils"
)

// Installer provide functions for the preflight installer.
type Installer struct {
	remote.InstallerCmdOptions

	logger *logrus.Entry

	osRelease      string
	packageManager pkgmgr.PackageManager

	packages        []string
	modules         []string
	services        []string
	spdkDepPackages []string
	spdkDepModules  []string
}

// Init initializes the Installer.
func (local *Installer) Init() error {
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
			"nfs-common", "open-iscsi",
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
			"nfs-utils", "iscsi-initiator-utils",
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
			"nfs-client", "open-iscsi",
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
			"nfs-utils", "open-iscsi",
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
	if local.UpdatePackages {
		if err := local.updatePackageList(); err != nil {
			return err
		}
	}

	if err := local.probeModules(consts.DependencyModuleDefault); err != nil {
		return err
	}

	if err := local.installPackages(false); err != nil {
		return err
	}

	if err := local.startServices(); err != nil {
		return err
	}

	if local.EnableSpdk {
		if err := local.installPackages(true); err != nil {
			return err
		}

		if err := local.probeModules(consts.DependencyModuleSpdk); err != nil {
			return err
		}

		if err := local.configureSPDKEnv(); err != nil {
			return err
		}
	}

	return nil
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
	}

	return nil
}

// installPackages installs packages with a package manager.
func (local *Installer) installPackages(spdkDependent bool) error {
	packages := local.packages
	if spdkDependent {
		packages = local.spdkDepPackages
	}
	for _, pkg := range packages {
		logrus.Infof("Installing package %s", pkg)

		_, err := local.packageManager.InstallPackage(pkg)
		if err != nil {
			return errors.Wrapf(err, "failed to install package %s", pkg)
		} else {
			logrus.Infof("Successfully installed package %s", pkg)
		}
	}

	return nil
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
