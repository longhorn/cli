package preflight

import "fmt"

func validateV2HostOS(osRelease string) error {
	switch osRelease {
	case "sles", "suse", "opensuse", "opensuse-leap", "sl-micro", "ubuntu", "debian", "rhel", "ol", "rocky", "centos", "fedora", "amzn", "opencloudos", "arch":
		return nil
	default:
		return fmt.Errorf("V2-only host qualification is unsupported for this operating system; use its declarative host prerequisites")
	}
}
