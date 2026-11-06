package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cp "github.com/otiai10/copy"

	"github.com/longhorn/cli/pkg/consts"
)

const v2SPDKReadTimeout = 15 * time.Second
const v2SPDKSetupTimeout = 3 * time.Minute

var v2HostReadEnvironment = []string{"-i", "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "/bin/bash", "--noprofile", "--norc"}

// The pinned setup script accepts eval-like NUMA grammar and optionally sources
// host-provided scripts. V2 accepts one canonical node and rejects those hooks.
func (local *Installer) qualifyV2SPDKHost() error {
	_, node, err := local.V2SPDKSetupEnvironment()
	if err != nil {
		return err
	}
	args := append(append([]string{}, v2HostReadEnvironment...), "-c", `set -eu
test -x /usr/bin/env
test -x /bin/bash
test ! -e /bin/wpdk_common.sh && test ! -L /bin/wpdk_common.sh
test ! -e /etc/opt/spdk-pkgdep/paths/export.sh && test ! -L /etc/opt/spdk-pkgdep/paths/export.sh
test -d "/sys/devices/system/node/node$1/hugepages/hugepages-2048kB"
test -r "/sys/devices/system/node/node$1/hugepages/hugepages-2048kB/nr_hugepages"
test -r "/sys/devices/system/node/node$1/hugepages/hugepages-2048kB/free_hugepages"
`, "--", node)
	if _, err := local.packageManager.Execute(nil, "/usr/bin/env", args, v2SPDKReadTimeout); err != nil {
		return fmt.Errorf("V2 SPDK host prerequisites are unqualified: %w", err)
	}
	_, err = local.readV2NodeHugepages(node)
	return err
}

func (local *Installer) readV2NodeHugepages(node string) (uint64, error) {
	path := "/sys/devices/system/node/node" + node + "/hugepages/hugepages-2048kB/nr_hugepages"
	args := append(append([]string{}, v2HostReadEnvironment...), "-c", `cat -- "$1"`, "--", path)
	output, err := local.packageManager.Execute(nil, "/usr/bin/env", args, v2SPDKReadTimeout)
	if err != nil {
		return 0, fmt.Errorf("failed to read actual V2 2MiB allocation: %w", err)
	}
	value := strings.TrimSpace(output)
	pages, err := strconv.ParseUint(value, 10, 64)
	if err != nil || value == "" || strconv.FormatUint(pages, 10) != value {
		return 0, fmt.Errorf("actual V2 2MiB allocation is unresolved")
	}
	return pages, nil
}

func (local *Installer) configureV2SPDKEnv() (resultErr error) {
	environment, node, err := local.V2SPDKSetupEnvironment()
	if err != nil {
		return err
	}
	if err := local.qualifyV2SPDKHost(); err != nil {
		return err
	}
	// Copy only into this invocation's new directory. Never remove or reuse a
	// shared host SPDK tree belonging to another invocation or application.
	hostDirectory, err := os.MkdirTemp(filepath.Join(consts.VolumeMountHostDirectory, "tmp"), "longhorn-spdk-v2-")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, os.RemoveAll(hostDirectory)) }()
	if err := cp.Copy("/spdk", hostDirectory); err != nil {
		return err
	}
	if err := local.qualifyV2SPDKHost(); err != nil {
		return err
	}
	relative, err := filepath.Rel(consts.VolumeMountHostDirectory, hostDirectory)
	if err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return fmt.Errorf("V2 setup host directory is not identified")
	}
	setupPath := filepath.Join("/", relative, "scripts/setup.sh")
	return local.executeV2SPDKSetup(setupPath, environment, node)
}

func (local *Installer) executeV2SPDKSetup(setupPath string, environment []string, node string) error {
	args := append([]string{"-i"}, environment...)
	args = append(args, "/bin/bash", "--noprofile", "--norc", "--", setupPath, "config")
	if _, err := local.packageManager.Execute(nil, "/usr/bin/env", args, v2SPDKSetupTimeout); err != nil {
		return fmt.Errorf("V2 SPDK setup failed: %w", err)
	}
	pages, err := local.readV2NodeHugepages(node)
	if err != nil {
		return err
	}
	if pages < uint64(local.HugePageSize/2) {
		return fmt.Errorf("V2 SPDK setup did not allocate the requested 2MiB hugepages on the selected NUMA node")
	}
	local.collection.Log.Info = append(local.collection.Log.Info, "Configured V2 SPDK environment and verified actual 2MiB hugepage allocation; run V2 preflight qualification after any required kubelet restart")
	return nil
}
