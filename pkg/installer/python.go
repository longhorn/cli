package installer

import "github.com/sirupsen/logrus"

func (i *Installer) InstallPythonPackages() {
	for _, pkg := range i.pythonPackages {
		logrus.Infof("Installing Python package %s", pkg)

		_, err := i.command.PipInstallPackage(pkg)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to install Python package %s", pkg)
		} else {
			logrus.Infof("Successfully installed Python package %s", pkg)
		}
	}
}
