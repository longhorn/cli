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

// extractOSReleaseField extracts a specific field from os-release lines
func extractOSReleaseField(lines []string, fieldName string) string {
	pattern := fmt.Sprintf(`^%s=["']?(.+?)["']?\n?$`, fieldName)
	fieldRexp := regexp.MustCompile(pattern)
	for _, line := range lines {
		match := fieldRexp.FindStringSubmatch(line)
		if len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

func parseOSreleaseFile(lines []string) (string, error) {
	// Extract key fields from os-release
	id := extractOSReleaseField(lines, "ID")
	idLike := extractOSReleaseField(lines, "ID_LIKE")
	variantID := extractOSReleaseField(lines, "VARIANT_ID")

	// For SUSE-based systems, determine if transactional or regular
	if isSUSEBased(id, idLike) {
		// Priority 1: Check if ID or ID_LIKE contains "sle-micro" or "sl-micro"
		// Example (SLE 6.1):
		//     ID="sl-micro"
		//     ID_LIKE="suse sle-micro opensuse-microos microos"
		idLower := strings.ToLower(id)
		idLikeLower := strings.ToLower(idLike)
		if strings.Contains(idLower, "sle-micro") || strings.Contains(idLower, "sl-micro") ||
			strings.Contains(idLikeLower, "sle-micro") || strings.Contains(idLikeLower, "sl-micro") {
			return "sl-micro", nil
		}

		// Priority 2: Check if VARIANT_ID indicates transactional system
		// Example (SLE 6.2):
		//     ID="sles"
		//     ID_LIKE="suse opensuse"
		//     VARIANT="Micro"
		//     VARIANT_ID="transactional"
		variantLower := strings.ToLower(variantID)
		if variantLower == "transactional" {
			return "sl-micro", nil
		}

		// Default: Non-transactional SUSE system
		return "suse", nil
	}

	// For non-SUSE systems, prefer ID_LIKE over ID (use first word from ID_LIKE)
	if idLike != "" {
		fields := strings.Fields(idLike)
		if len(fields) > 0 {
			return fields[0], nil
		}
	}

	// Fall back to ID if ID_LIKE is not available (use first word)
	if id != "" {
		fields := strings.Fields(id)
		if len(fields) > 0 {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("could not find platform information in os-release: %v", lines)
}

// isSUSEBased checks if the system is SUSE-based by examining ID and ID_LIKE fields
func isSUSEBased(id, idLike string) bool {
	// Check ID field
	idLower := strings.ToLower(id)
	if strings.Contains(idLower, "suse") || strings.Contains(idLower, "sles") ||
		strings.Contains(idLower, "opensuse") || idLower == "sl-micro" || idLower == "sle-micro" {
		return true
	}

	// Check ID_LIKE field
	idLikeLower := strings.ToLower(idLike)
	if strings.Contains(idLikeLower, "suse") || strings.Contains(idLikeLower, "sles") ||
		strings.Contains(idLikeLower, "opensuse") {
		return true
	}

	return false
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
