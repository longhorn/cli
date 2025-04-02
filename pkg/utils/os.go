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
	case "ubuntu", "debian", "raspbian":
		return pkgmgr.PackageManagerApt, nil
	case "rhel", "ol", "rocky", "centos", "fedora", "amzn":
		return pkgmgr.PackageManagerYum, nil
	case "arch":
		return pkgmgr.PackageManagerPacman, nil
	default:
		return pkgmgr.PackageManagerUnknown, fmt.Errorf("operating system (%s) is not supported", osRelease)
	}
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
	var platform string

	platformRexp := regexp.MustCompile(`^ID=["']?(.+?)["']?\n?$`)

	for _, line := range lines {
		match := platformRexp.FindStringSubmatch(line)
		if len(match) > 0 {
			platform = match[1]
		}
	}

	if platform == "" {
		return "", fmt.Errorf("could not find platform information in os-release: %v", lines)
	}

	return platform, nil
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
