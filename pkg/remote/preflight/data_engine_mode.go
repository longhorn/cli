package preflight

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/longhorn/cli/pkg/types"
)

func validateV2Hugepages(size int) error {
	if size <= 0 || size%2 != 0 || int64(size) > math.MaxInt64/(1024*1024) {
		return fmt.Errorf("v2-only requires a positive, bounded amount of 2MiB hugepages")
	}
	return nil
}

func validateV2Driver(driver string) error {
	if driver != "" && driver != "vfio-pci" && driver != "uio_pci_generic" {
		return fmt.Errorf("unsupported v2-only userspace driver")
	}
	return nil
}

func (options *CheckerCmdOptions) ValidateDataEngineMode() error {
	mode, err := types.ParseDataEngineMode(options.DataEngineMode)
	if err != nil {
		return err
	}
	options.DataEngineMode = string(mode)
	if mode != types.DataEngineModeV2Only {
		return nil
	}
	if err := validateV2Hugepages(options.HugePageSize); err != nil {
		return err
	}
	if options.UserspaceDriver != "" && options.UserspaceDriver != "vfio_pci" && options.UserspaceDriver != "uio_pci_generic" {
		return fmt.Errorf("unsupported v2-only userspace kernel module")
	}
	return nil
}

func (options *InstallerCmdOptions) ValidateDataEngineMode() error {
	mode, err := types.ParseDataEngineMode(options.DataEngineMode)
	if err != nil {
		return err
	}
	options.DataEngineMode = string(mode)
	if mode != types.DataEngineModeV2Only {
		return nil
	}
	_, _, err = options.V2SPDKSetupEnvironment()
	return err
}

var v2PCIAddress = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[01][0-9a-fA-F]\.[0-7]$`)

func normalizeV2PCIAllowed(value string) (string, error) {
	if value == "" || value == "none" {
		return "none", nil
	}
	addresses := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(addresses) == 0 {
		return "", fmt.Errorf("v2-only PCI selection is unresolved")
	}
	seen := map[string]bool{}
	for index, address := range addresses {
		address = strings.ToLower(address)
		if !v2PCIAddress.MatchString(address) || seen[address] {
			return "", fmt.Errorf("v2-only requires unique full PCI addresses or none")
		}
		seen[address], addresses[index] = true, address
	}
	return strings.Join(addresses, " "), nil
}

// V2SPDKSetupEnvironment owns the pinned setup script's relevant inputs.
// It does not establish host qualification: callers must bind the script and
// host prerequisites before writes, then verify actual allocation afterward.
func (options InstallerCmdOptions) V2SPDKSetupEnvironment() ([]string, string, error) {
	mode, err := types.ParseDataEngineMode(options.DataEngineMode)
	if err != nil || mode != types.DataEngineModeV2Only {
		return nil, "", fmt.Errorf("v2-only setup requires an explicit capability")
	}
	if options.OperatingSystem != "" {
		return nil, "", fmt.Errorf("v2-only installation supports qualified package-manager hosts; COS and explicit OS overrides are unsupported")
	}
	if err := validateV2Hugepages(options.HugePageSize); err != nil {
		return nil, "", err
	}
	if err := validateV2Driver(options.DriverOverride); err != nil {
		return nil, "", err
	}
	if options.RestartKubelet {
		window, err := time.ParseDuration(options.RestartKubeletWindow)
		if err != nil || window < 0 {
			return nil, "", fmt.Errorf("v2-only requires a valid nonnegative kubelet restart window")
		}
	}
	pciAllowed, err := normalizeV2PCIAllowed(options.AllowPci)
	if err != nil {
		return nil, "", err
	}
	node, seen := "0", map[string]bool{}
	if options.SpdkOptions != "" {
		for _, option := range strings.Split(options.SpdkOptions, ",") {
			name, value, assigned := strings.Cut(option, "=")
			if !assigned || seen[name] {
				return nil, "", fmt.Errorf("invalid or duplicate v2-only SPDK option")
			}
			seen[name] = true
			switch name {
			case "HUGEMEM":
				if value != strconv.Itoa(options.HugePageSize) {
					return nil, "", fmt.Errorf("SPDK memory override contradicts v2-only hugepage intent")
				}
			case "HUGEPGSZ":
				if value != "2048" {
					return nil, "", fmt.Errorf("v2-only setup requires 2MiB hugepages")
				}
			case "PCI_ALLOWED":
				selected, err := normalizeV2PCIAllowed(value)
				if err != nil || selected != pciAllowed {
					return nil, "", fmt.Errorf("SPDK PCI override contradicts v2-only selection")
				}
			case "DRIVER_OVERRIDE":
				if value != options.DriverOverride {
					return nil, "", fmt.Errorf("SPDK driver override contradicts v2-only selection")
				}
			case "HUGENODE":
				number, err := strconv.ParseUint(value, 10, 31)
				if err != nil || strconv.FormatUint(number, 10) != value {
					return nil, "", fmt.Errorf("v2-only setup requires one canonical NUMA node number")
				}
				node = value
			default:
				return nil, "", fmt.Errorf("unsupported SPDK option in v2-only setup")
			}
		}
	}
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C",
		"HUGEMEM=" + strconv.Itoa(options.HugePageSize), "HUGEPGSZ=2048", "HUGENODE=" + node,
		"PCI_ALLOWED=" + pciAllowed, "DRIVER_OVERRIDE=" + options.DriverOverride, "TARGET_USER=root",
	}, node, nil
}
