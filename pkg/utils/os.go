package utils

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/longhorn/cli/pkg/consts"

	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
)

func GetPackageManagerType(osRelease string) (pkgmgr.PackageManagerType, error) {
	switch osRelease {
	case "sles", "suse", "opensuse", "opensuse-leap":
		return pkgmgr.PackageManagerZypper, nil
	case "sl-micro":
		return pkgmgr.PackageManagerTransactionalUpdate, nil
	case "ubuntu", "debian":
		return pkgmgr.PackageManagerApt, nil
	case "rhel", "ol", "rocky", "centos", "fedora", "amzn":
		return pkgmgr.PackageManagerYum, nil
	case "arch":
		return pkgmgr.PackageManagerPacman, nil
	default:
		return detectPackageManagerUnknown(osRelease)
	}
}

func detectPackageManagerUnknown(osRelease string) (pkgmgr.PackageManagerType, error) {
	packageManagers := []struct {
		command string
		pkgType pkgmgr.PackageManagerType
		distro  string
	}{
		{"transactional-update", pkgmgr.PackageManagerTransactionalUpdate, "SUSE micro"},
		{"zypper", pkgmgr.PackageManagerZypper, "SUSE-based"},
		{"apt", pkgmgr.PackageManagerApt, "Debian-based"},
		{"microdnf", pkgmgr.PackageManagerYum, "RPM-based (microdnf)"},
		{"yum", pkgmgr.PackageManagerYum, "RPM-based (yum)"},
		{"dnf", pkgmgr.PackageManagerYum, "RPM-based (dnf)"},
		{"pacman", pkgmgr.PackageManagerPacman, "Arch Linux"},
	}

	for _, pm := range packageManagers {
		if isCommandAvailableOnHost(pm.command) {
			fmt.Fprintf(os.Stderr, "WARNING: Operating system '%s' is not officially supported by the Longhorn command-line tool. Please check the official documentation to install the prerequisites manually. "+
				"Detected package manager '%s' (%s). "+
				"Proceeding with compatibility mode, but there may be compatibility issues.\n",
				osRelease, pm.command, pm.distro)
			return pm.pkgType, nil
		}
	}
	return pkgmgr.PackageManagerUnknown, fmt.Errorf("operating system (%s) is not supported by the Longhorn command-line tool and no known package manager could be detected. Please check the official documentation to install the prerequisites manually", osRelease)
}

// isCommandAvailableOnHost checks if a command is available on the host system
// by checking common binary locations in the host filesystem
func isCommandAvailableOnHost(command string) bool {
	// Common paths where package managers are typically installed
	commonPaths := []string{
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
	}

	// Check if running in a container with host mount
	hostRoot := consts.VolumeMountHostDirectory
	if _, err := os.Stat(hostRoot); err == nil {
		// Check in host paths
		for _, dir := range commonPaths {
			hostPath := filepath.Join(hostRoot, dir, command)
			if info, err := os.Stat(hostPath); err == nil {
				// Verify it's a regular file or symlink and has execute permissions
				if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
					// Check if file has execute permission (at least one execute bit set)
					if info.Mode().Perm()&0111 != 0 {
						return true
					}
				}
			}
		}
	}

	return false
}

func GetOSRelease() (string, error) {
	// List of possible locations for the os-release file.
	possiblePaths := []string{
		filepath.Join("/etc/os-release"),
		filepath.Join("/usr/lib/os-release"),
	}

	// Try to find the os-release file
	var lines []string
	var err error
	for _, path := range possiblePaths {
		hostPath := filepath.Join(consts.VolumeMountHostDirectory, path)
		if _, err = os.Stat(hostPath); err == nil {
			lines, err = readFileLines(hostPath)
			break
		}
	}

	// Return error is os-release file is not found
	if err != nil {
		return "", errors.New("no os-release file found")
	}

	return parseOSreleaseFile(lines)
}

func parseOSreleaseFile(lines []string) (string, error) {
	// First, try using `ID_LIKE` because some users might be on customized OS with a modified `ID`,
	// making it difficult to determine things like the proper package manager. If `ID_LIKE` is not found, use `ID`.
	platformRexp := regexp.MustCompile(`^ID_LIKE=["']?(.+?)["']?\n?$`)
	platform := parsePlatform(lines, platformRexp)
	if platform == "" {
		platformRexp = regexp.MustCompile(`^ID=["']?(.+?)["']?\n?$`)
		platform = parsePlatform(lines, platformRexp)
	}

	if platform == "" {
		return "", fmt.Errorf("could not find platform information in os-release: %v", lines)
	}

	return platform, nil
}

func parsePlatform(lines []string, platformRexp *regexp.Regexp) (platforms string) {
	for _, line := range lines {
		match := platformRexp.FindStringSubmatch(line)
		if len(match) > 0 {
			platforms = match[1]
			break
		}
	}
	fields := strings.Fields(platforms)
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func readFileLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func IsModuleLoaded(moduleName string) (bool, error) {
	cmd := exec.Command("lsmod")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	if strings.Contains(string(output), moduleName) {
		return true, nil
	}

	return false, nil
}

func GetKernelVersion() (string, error) {
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
