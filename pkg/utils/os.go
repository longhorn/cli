package utils

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/longhorn/cli/pkg/pkgmgr"
)

func GetPackageManagerType(platform string) (pkgmgr.PackageManagerType, error) {
	switch platform {
	case "sles", "suse", "opensuse", "opensuse-leap":
		return pkgmgr.PackageManagerZypper, nil
	case "ubuntu", "debian":
		return pkgmgr.PackageManagerApt, nil
	case "rhel", "ol", "rocky", "centos", "fedora":
		return pkgmgr.PackageManagerYum, nil
	case "arch":
		return pkgmgr.PackageManagerPacman, nil
	default:
		return pkgmgr.PackageManagerUnknown, fmt.Errorf("unknown platform %s", platform)
	}
}

func GetOSRelease() (string, error) {
	var lines []string
	var err error

	if _, err = os.Stat("/host/etc/os-release"); err == nil {
		lines, err = readFileLines("/host/etc/os-release")
	} else if _, err = os.Stat("/host/usr/lib/os-release"); err == nil {
		lines, err = readFileLines("/host/usr/lib/os-release")
	} else {
		err = errors.New("no os-release file found")
	}

	if err != nil {
		return "", err
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

func GetKernelVersion() string {
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
