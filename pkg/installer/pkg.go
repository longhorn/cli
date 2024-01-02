package installer

import "github.com/sirupsen/logrus"

// StartServices starts services
func (i *Installer) StartServices() {
	for _, svc := range i.services {
		logrus.Infof("Starting service %s", svc)

		_, err := i.pkgMgr.StartService(svc)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to start service %s", svc)
		} else {
			logrus.Infof("Successfully started service %s", svc)
		}
	}
}

// ProbeModules probes kernel modules
func (i *Installer) ProbeModules(spdkDependent bool) {
	modules := i.modules
	if spdkDependent {
		modules = i.spdkDepModules
	}
	for _, mod := range modules {
		logrus.Infof("Probing module %s", mod)

		_, err := i.pkgMgr.Modprobe(mod)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to probe module %s", mod)
		} else {
			logrus.Infof("Successfully probed module %s", mod)
		}
	}
}

// InstallPackages installs packages with a package manager
func (i *Installer) InstallPackages(spdkDependent bool) {
	packages := i.packages
	if spdkDependent {
		packages = i.spdkDepPackages
	}
	for _, pkg := range packages {
		logrus.Infof("Installing package %s", pkg)

		_, err := i.pkgMgr.InstallPackage(pkg)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to install package %s", pkg)
		} else {
			logrus.Infof("Successfully installed package %s", pkg)
		}
	}
}

// UpdatePackageList updates list of available packages
func (i *Installer) UpdatePackageList() {
	logrus.Info("Updating package list")
	_, err := i.pkgMgr.UpdatePackageList()
	if err != nil {
		logrus.WithError(err).Error("Failed to update package list")
	} else {
		logrus.Info("Successfully updated package list")
	}
}
