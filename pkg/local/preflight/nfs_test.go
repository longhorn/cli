package preflight

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/longhorn/cli/pkg/local/preflight/packagemanager/mocks"
	"github.com/longhorn/cli/pkg/types"
)

// TestNFSv4SupportWithMissingKernelConfig tests the NFSv4 support check with missing boot config.
// It tests the case where the kernel config file doesn't exist in /boot,
// which happens on Fedora CoreOS, and the fallback to /proc/config.gz.
func TestNFSv4SupportWithMissingKernelConfig(t *testing.T) {
	tests := []struct {
		name             string
		getKernelVersion func() (string, error)
		getBootConfigMap func(bootDir, kernelVersion string) (map[string]string, error)
		getProcConfigMap func(procDir string) (map[string]string, error)
		successLog       string // Expected success log message when NFSv4 is supported
		errorLog         string // Expected error log message when NFSv4 is not supported
	}{
		{
			name: "kernel config missing and /proc/config.gz also missing",
			getKernelVersion: func() (string, error) {
				return "6.9.11-200.fc40", nil
			},
			getBootConfigMap: func(bootDir, kernelVersion string) (map[string]string, error) {
				return nil, fmt.Errorf("no such file or directory")
			},
			getProcConfigMap: func(procDir string) (map[string]string, error) {
				return nil, fmt.Errorf("no such file or directory")
			},
		},
		{
			name: "kernel config missing but /proc/config.gz exists with CONFIG_NFS_V4_2",
			getKernelVersion: func() (string, error) {
				return "6.9.11-200.fc40", nil
			},
			getBootConfigMap: func(bootDir, kernelVersion string) (map[string]string, error) {
				return nil, fmt.Errorf("no such file or directory")
			},
			getProcConfigMap: func(procDir string) (map[string]string, error) {
				// Create a mock kernel config with CONFIG_NFS_V4_2 enabled
				configText := "# Config file\nCONFIG_NFS_V4_2=y\nCONFIG_NFS_V4_1=m\nCONFIG_NFS_V4=y\n"
				return parseKernelModuleConfigString(configText)
			},
			successLog: "NFS4 is supported",
		},
		{
			name: "kernel config missing but /proc/config.gz exists with CONFIG_NFS_V4_1",
			getKernelVersion: func() (string, error) {
				return "6.9.11-200.fc40", nil
			},
			getBootConfigMap: func(bootDir, kernelVersion string) (map[string]string, error) {
				return nil, fmt.Errorf("no such file or directory")
			},
			getProcConfigMap: func(procDir string) (map[string]string, error) {
				// Create a mock kernel config with CONFIG_NFS_V4_1 enabled
				configText := "# Config file\nCONFIG_NFS_V4_1=y\nCONFIG_NFS_V4=y\n"
				return parseKernelModuleConfigString(configText)
			},
			successLog: "NFS4 is supported",
		},
		{
			name: "kernel config missing but /proc/config.gz exists without NFSv4 support",
			getKernelVersion: func() (string, error) {
				return "6.9.11-200.fc40", nil
			},
			getBootConfigMap: func(bootDir, kernelVersion string) (map[string]string, error) {
				return nil, fmt.Errorf("no such file or directory")
			},
			getProcConfigMap: func(procDir string) (map[string]string, error) {
				// Create a mock kernel config without NFS support
				configText := "# Config file\nCONFIG_NFS=y\nCONFIG_NFSD=m\n"
				return parseKernelModuleConfigString(configText)
			},
			errorLog: "kernel does not support NFSv4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPM := new(mocks.MockPackageManager)

			checker := &Checker{
				packageManager: mockPM,
				collection: types.NodeCollection{
					Log: &types.LogCollection{},
				},
				// Inject mocked functions
				utils: &mockUtils{
					getKernelVersion:               tc.getKernelVersion,
					getBootKernelConfigMap:         tc.getBootConfigMap,
					getProcKernelConfigMap:         tc.getProcConfigMap,
				},
			}

			err := checker.checkNFSv4Support()

			// check that the appropriate log/error exists
			if tc.successLog != "" {
				// NFSv4 is supported - check for success log
				found := false
				for _, log := range checker.collection.Log.Info {
					if strings.Contains(log, tc.successLog) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find log containing %q", tc.successLog)
			} else if tc.errorLog != "" {
				// NFSv4 is not supported - check for error log
				found := false
				for _, log := range checker.collection.Log.Error {
					if strings.Contains(log, tc.errorLog) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find error log containing %q", tc.errorLog)
			} else {
				// Both boot and proc config files missing - function returns error
				assert.Error(t, err, "Expected error when both configs are missing")
			}
		})
	}
}

// parseKernelModuleConfigString parses a kernel config string into a key-value map.
func parseKernelModuleConfigString(configText string) (map[string]string, error) {
	configMap := make(map[string]string)
	lines := strings.Split(configText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse "CONFIG_XXX=value" format
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			configMap[key] = value
		}
	}
	return configMap, nil
}
