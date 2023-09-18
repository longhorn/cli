package types

type PackageManager string

const (
	PackageManagerUnknown = PackageManager("")
	PackageManagerApt     = PackageManager("apt")
	PackageManagerYum     = PackageManager("yum")
	PackageManagerZypper  = PackageManager("zypper")
	PackageManagerApk     = PackageManager("apk")
	PackageManagerPacman  = PackageManager("pacman")
)
