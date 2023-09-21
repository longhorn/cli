package utils

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/longhorn/longhorn-preflight/pkg/types"
)

func GetPackageManager(platform string) (types.PackageManager, error) {
	switch platform {
	case "sles", "suse", "opensuse", "opensuse-leap":
		return types.PackageManagerZypper, nil
	case "ubuntu", "debian":
		return types.PackageManagerApt, nil
	case "rhel", "ol":
		return types.PackageManagerYum, nil
	default:
		return types.PackageManagerUnknown, fmt.Errorf("unknown platform %s", platform)
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
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
