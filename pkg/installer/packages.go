package installer

import "github.com/sirupsen/logrus"

// InstallPythonPackages installs Python packages with pip
func (i *Installer) ProbeModules() {
	for _, mod := range i.modules {
		logrus.Infof("Probing module %s", mod)

		_, err := i.command.Modprobe(mod)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to probe module %s", mod)
		} else {
			logrus.Infof("Successfully probed module %s", mod)
		}
	}
}

// InstallPackages installs packages with a package manager
func (i *Installer) InstallPackages() {
	for _, pkg := range i.packages {
		logrus.Infof("Installing package %s", pkg)

		_, err := i.command.InstallPackage(pkg)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to install package %s", pkg)
		} else {
			logrus.Infof("Successfully installed package %s", pkg)
		}
	}
}

// UpdatePackageList updates list of available packages
func (i *Installer) UpdatePackageList() (string, error) {
	return i.command.UpdatePackageList()
}

// InstallPackage install a package with a package manager
func (i *Installer) InstallPackage(name string) (string, error) {
	return i.command.InstallPackage(name)
}

// UninstallPackage uninstall a package with a package manager
func (i *Installer) UninstallPackage(name string) (string, error) {
	return i.command.UninstallPackage(name)
}
