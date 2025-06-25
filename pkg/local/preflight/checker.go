package preflight

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"
	commonnfs "github.com/longhorn/go-common-libs/nfs"
	commonns "github.com/longhorn/go-common-libs/ns"
	commonsys "github.com/longhorn/go-common-libs/sys"
	commontypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/cli/pkg/consts"
	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
	remote "github.com/longhorn/cli/pkg/remote/preflight"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

// Checker provide functions for the preflight checker.
type Checker struct {
	remote.CheckerCmdOptions

	logger *logrus.Entry

	OutputFilePath string

	kubeClient *kubeclient.Clientset

	osRelease      string
	packageManager pkgmgr.PackageManager

	packages        []string
	modules         []string
	services        []string
	spdkDepPackages []string
	spdkDepModules  []string

	collection types.NodeCollection
}

// Init initializes the Checker.
func (local *Checker) Init() error {
	local.collection.Log = &types.LogCollection{}

	config, err := commonkube.GetInClusterConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get client config")
	}

	local.kubeClient, err = kubeclient.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "failed to get Kubernetes clientset")
	}

	osRelease, err := utils.GetOSRelease()
	if err != nil {
		return errors.Wrap(err, "failed to get OS release")
	}
	local.osRelease = osRelease
	local.logger = logrus.WithField("os", local.osRelease)

	if local.osRelease == fmt.Sprint(consts.OperatingSystemContainerOptimizedOS) {
		return nil
	}

	packageManagerType, err := utils.GetPackageManagerType(osRelease)
	if err != nil {
		return errors.Wrap(err, "failed to get package manager")
	}
	local.logger = local.logger.WithField("package-manager", packageManagerType)

	namespaces := []commontypes.Namespace{
		commontypes.NamespaceMnt,
		commontypes.NamespaceNet,
	}

	executor, err := commonns.NewNamespaceExecutor(commontypes.ProcessSelf, commontypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	packageManager, err := pkgmgr.New(packageManagerType, executor)
	if err != nil {
		return err
	}

	switch packageManagerType {
	case pkgmgr.PackageManagerApt:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-common", "open-iscsi", "cryptsetup", "dmsetup",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	case pkgmgr.PackageManagerYum:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-utils", "iscsi-initiator-utils", "cryptsetup", "device-mapper",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	case pkgmgr.PackageManagerZypper, pkgmgr.PackageManagerTransactionalUpdate:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-client", "open-iscsi", "cryptsetup", "device-mapper",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	case pkgmgr.PackageManagerPacman:
		local.packageManager = packageManager
		local.packages = []string{
			"nfs-utils", "open-iscsi", "cryptsetup", "device-mapper",
		}
		local.modules = []string{
			"dm_crypt",
		}
		local.services = []string{
			"multipathd.service",
		}
		local.spdkDepPackages = []string{}
		local.spdkDepModules = []string{
			"nvme_tcp",
			"uio_pci_generic",
			"vfio_pci",
		}

	default:
		return errors.Errorf("operating system (%v) package manager (%s) is not supported", osRelease, packageManagerType)
	}

	return nil
}

// Run executes the preflight checks.
func (local *Checker) Run() error {
	checkTasks := []func() error{
		local.checkKubeDNS,
	}

	switch local.osRelease {
	case fmt.Sprint(consts.OperatingSystemContainerOptimizedOS):
		logrus.Infof("Checking preflight for %v", consts.OperatingSystemContainerOptimizedOS)
		checkTasks = append(checkTasks,
			local.checkContainerOptimizedOS,
		)
	default:
		checkTasks = append(checkTasks,
			local.checkIscsidService,
			local.checkMultipathService,
			local.checkNFSv4Support,
			func() error { return local.checkPackagesInstalled(false) },
			func() error { return local.checkModulesLoaded(false) },
		)

		if local.EnableSpdk {
			instructionSets := map[string][]string{
				"amd64": {"sse4_2"},
			}

			checkTasks = append(checkTasks,
				local.checkHugePages,
				func() error { return local.checkCpuInstructionSet(instructionSets) },
				func() error { return local.checkPackagesInstalled(true) },
				func() error { return local.checkModulesLoaded(true) },
			)
		}
	}

	for _, checkTaskFn := range checkTasks {
		// collect application-level error
		// [Topic][InternalError]: error msg
		if internalErr := checkTaskFn(); internalErr != nil {
			local.collection.Log.Error = append(local.collection.Log.Error,
				strings.ReplaceAll(internalErr.Error(), "\n", " "))
		}
	}

	return nil
}

// Output converts the collection to JSON and output to stdout or the output file.
func (local *Checker) Output() error {
	local.logger.Tracef("Outputting preflight checks results")

	jsonBytes, err := json.Marshal(local.collection)
	if err != nil {
		return errors.Wrap(err, "failed to convert collection to JSON")
	}

	return utils.HandleResult(jsonBytes, local.OutputFilePath, local.logger)
}

// checkContainerOptimizedOS checks if the node-agent DaemonSet is running.
func (local *Checker) checkContainerOptimizedOS() error {
	topic := formatTopic(consts.PreflightCheckTopicContainerOptimizedOS)

	daemonSet, err := commonkube.GetDaemonSet(local.kubeClient, metav1.NamespaceDefault, consts.AppNamePreflightContainerOptimizedOS)
	if err != nil {
		return wrapInternalError(topic, errors.Wrapf(err,
			"failed to retrieve DaemonSet %q in namespace %q. Please ensure the preflight DaemonSet is deployed correctly",
			consts.AppNamePreflightContainerOptimizedOS, metav1.NamespaceDefault))
	}

	if !commonkube.IsDaemonSetReady(daemonSet) {
		local.collection.Log.Error = append(local.collection.Log.Error, wrapMsgWithTopic(topic, fmt.Sprintf(
			"daemonSet %q is not ready in namespace %q.\nPlease check its pod status",
			consts.AppNamePreflightContainerOptimizedOS, metav1.NamespaceDefault)))
	}
	return nil
}

// checkMultipathService checks if the multipathd service is running.
func (local *Checker) checkMultipathService() error {
	logrus.Info("Checking multipathd service status")
	topic := formatTopic(consts.PreflightCheckTopicMultipathService)

	_, err := local.packageManager.GetServiceStatus("multipathd.service")
	switch {
	case err == nil:
		// Exit code 0: Service is running
		msg := "multipathd.service is running. Please refer to https://longhorn.io/kb/troubleshooting-volume-with-multipath/ for more information."
		local.collection.Log.Warn = append(local.collection.Log.Warn, wrapMsgWithTopic(topic, msg))
		return nil

	case isExitCode(err, 3), isExitCode(err, 4):
		// systemctl
		// Exit code 3: Inactive
		// Exit code 4: Not found
		local.collection.Log.Info = append(local.collection.Log.Info, wrapMsgWithTopic(topic, "multipathd.service is not running"))

	default:
		// Unexpected internal error
		return wrapInternalError(topic, fmt.Errorf("failed to check multipathd.service: %w", err))
	}

	_, err = local.packageManager.GetServiceStatus("multipathd.socket")
	switch {
	case err == nil:
		msg := "multipathd.service is inactive, but it can still be activated by multipathd.socket."
		local.collection.Log.Warn = append(local.collection.Log.Warn, wrapMsgWithTopic(topic, msg))
		return nil

	case isExitCode(err, 3), isExitCode(err, 4):
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, "neither multipathd.service nor multipathd.socket is running"))
	default:
		// Internal/systemctl failure
		return wrapInternalError(topic, fmt.Errorf("failed to check multipathd.socket: %w", err))
	}

	return nil
}

// checkIscsidService checks if the iscsid service is running.
func (local *Checker) checkIscsidService() error {
	logrus.Info("Checking iscsid service status")
	topic := formatTopic(consts.PreflightCheckTopicIscsidService)

	_, err := local.packageManager.GetServiceStatus("iscsid.service")
	switch {
	case err == nil:
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, "Service iscsid is running"))
		return nil
	case isExitCode(err, 3), isExitCode(err, 4):
		// systemctl
		// Exit code 3: Inactive
		// Exit code 4: Unit not found
	default:
		return wrapInternalError(topic, fmt.Errorf("failed to check iscsid.service: %w", err))
	}

	_, err = local.packageManager.GetServiceStatus("iscsid.socket")
	switch {
	case err == nil:
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, "Service iscsid is inactive, but it can still be activated by iscsid.socket"))
		return nil
	case isExitCode(err, 3), isExitCode(err, 4):
		// systemctl
		// Exit code 3: Inactive
		// Exit code 4: Unit not found
		// These are considered expected results — proceed to check socket
	default:
		return wrapInternalError(topic, fmt.Errorf("failed to check iscsid.socket: %w", err))
	}

	local.collection.Log.Error = append(local.collection.Log.Error,
		wrapMsgWithTopic(topic, "neither iscsid.service nor iscsid.socket is running"))

	return nil
}

// checkHugePages checks if HugePages is enabled.
func (local *Checker) checkHugePages() error {
	logrus.Info("Checking if HugePages is enabled")
	topic := formatTopic(consts.PreflightCheckTopicHugePages)

	if local.HugePageSize == 0 {
		logrus.Error("HUGEMEM environment variable is not set")
		return nil
	}

	pages := local.HugePageSize >> 1

	ok, hugePagesTotalNum, requiredHugePages, err := local.isHugePagesTotalEqualOrLargerThan(pages)
	if err != nil {
		return wrapInternalError(topic, errors.Wrap(err, "failed to check HugePages"))
	}
	if !ok {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, fmt.Sprintf("HugePages are insufficient. Required 2MiB HugePages: %v pages, Available: %v pages", requiredHugePages, hugePagesTotalNum)))
		return nil
	}

	local.collection.Log.Info = append(local.collection.Log.Info, wrapMsgWithTopic(topic, "HugePages is enabled"))
	return nil
}

func (local *Checker) isHugePagesTotalEqualOrLargerThan(requiredHugePages int) (bool, int, int, error) {
	output, err := local.packageManager.Execute([]string{}, "grep", []string{"HugePages_Total", "/proc/meminfo"}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return false, 0, 0, errors.Wrap(err, "failed to get total number of HugePages")
	}
	line := strings.Split(output, "\n")[0]
	hugePagesTotal := strings.TrimSpace(strings.Split(line, ":")[1])

	hugePagesTotalNum, err := strconv.Atoi(hugePagesTotal)
	if err != nil {
		return false, 0, 0, errors.Wrap(err, "failed to convert HugePages total to a number")
	}

	return hugePagesTotalNum >= requiredHugePages, hugePagesTotalNum, requiredHugePages, nil
}

// CheckCpuInstructionSet checks if the CPU instruction set is supported.
func (local *Checker) checkCpuInstructionSet(instructionSets map[string][]string) error {
	logrus.Info("Checking CPU instruction set")
	topic := formatTopic(consts.PreflightCheckTopicSPDK, consts.PreflightCheckTopicCpuInstructionSet)

	arch := runtime.GOARCH
	logrus.Infof("Detected CPU architecture: %v", arch)

	sets, ok := instructionSets[arch]
	if !ok {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, fmt.Sprintf("CPU model is not supported: %v", arch)))
		return nil
	}

	var internalError = map[string]any{}

	for _, set := range sets {
		_, err := local.packageManager.Execute([]string{}, "grep", []string{set, "/proc/cpuinfo"}, commontypes.ExecuteNoTimeout)
		if err != nil {
			if isExitCode(err, 1) { // expected not-installed case
				local.collection.Log.Error = append(local.collection.Log.Error,
					wrapMsgWithTopic(topic, fmt.Sprintf("%s is unsupported. %v", set, err)))
			} else {
				internalError[set] = err
			}
		} else {
			local.collection.Log.Info = append(local.collection.Log.Info,
				wrapMsgWithTopic(topic, fmt.Sprintf("%s is supported", set)))
		}
	}

	if len(internalError) > 0 {
		return wrapAggregatedInternalError(topic, "Failed to grep CPU instruction sets:", internalError)
	}

	return nil
}

// checkPackagesInstalled checks if the packages are installed.
func (local *Checker) checkPackagesInstalled(spdkDependent bool) error {
	var topic string

	packages := local.packages
	if spdkDependent {
		topic = formatTopic(consts.PreflightCheckTopicSPDK, consts.PreflightCheckTopicPackages)
		packages = local.spdkDepPackages
	} else {
		topic = formatTopic(consts.PreflightCheckTopicPackages)
	}

	if len(packages) == 0 {
		return nil
	}

	logrus.Info("Checking if required packages are installed")

	var internalError = map[string]any{}

	for _, pkg := range packages {
		_, err := local.packageManager.CheckPackageInstalled(pkg)
		if err != nil {
			if isExitCode(err, 1) || errors.Is(err, pkgmgr.PackageNotInstalledError) {
				local.collection.Log.Error = append(local.collection.Log.Error,
					wrapMsgWithTopic(topic, fmt.Sprintf("%s is not installed. %v", pkg, err)))
			} else {
				internalError[pkg] = err
			}
		} else {
			local.collection.Log.Info = append(local.collection.Log.Info,
				wrapMsgWithTopic(topic, fmt.Sprintf("%s is installed", pkg)))
		}
	}

	if len(internalError) > 0 {
		return wrapAggregatedInternalError(topic, "Failed to check packages:", internalError)
	}

	return nil
}

// checkModulesLoaded checks if the modules are loaded.
func (local *Checker) checkModulesLoaded(spdkDependent bool) error {
	var topic string

	modules := local.modules
	if spdkDependent {
		modules = local.spdkDepModules
		topic = formatTopic(consts.PreflightCheckTopicSPDK, consts.PreflightCheckTopicKernelModules)

		if local.UserspaceDriver != "" {
			modules = append(modules, local.UserspaceDriver)
		}
	} else {
		topic = formatTopic(consts.PreflightCheckTopicKernelModules)
	}

	if len(modules) == 0 {
		return nil
	}

	logrus.Info("Checking if required modules are loaded")

	var internalError = map[string]any{}

	for _, mod := range modules {
		logrus.Infof("Checking if module %s is loaded", mod)

		err := local.packageManager.CheckModLoaded(mod)
		if err != nil {
			if isExitCode(err, 1) || errors.Is(err, pkgmgr.PackageNotInstalledError) {
				local.collection.Log.Error = append(local.collection.Log.Error,
					wrapMsgWithTopic(topic, fmt.Sprintf("%s is not loaded. %v", mod, err)))
			} else {
				internalError[mod] = err
			}
		} else {
			local.collection.Log.Info = append(local.collection.Log.Info,
				wrapMsgWithTopic(topic, fmt.Sprintf("%s is loaded", mod)))
		}
	}

	if len(internalError) > 0 {
		return wrapAggregatedInternalError(topic, "Failed to check packages:", internalError)
	}

	return nil
}

// checkNFSv4Support checks if NFS4 is supported on the host.
func (local *Checker) checkNFSv4Support() error {
	logrus.Info("Checking if NFS4 (either 4.0, 4.1 or 4.2) is supported")
	topic := formatTopic(consts.PreflightCheckTopicNFS)

	// check kernel capability
	var isKernelSupport = false

	kernelVersion, err := utils.GetKernelVersion()
	if err != nil {
		return wrapInternalError(topic, fmt.Errorf("failed to detect kernel version: %v", err))
	}
	hostBootDir := filepath.Join(consts.VolumeMountHostDirectory, commontypes.SysBootDirectory)
	kernelConfigMap, err := commonsys.GetBootKernelConfigMap(hostBootDir, kernelVersion)
	if err != nil {
		return wrapInternalError(topic, fmt.Errorf("failed to read kernel config: %v", err))
	}
	for configItem, module := range map[string]string{
		"CONFIG_NFS_V4_2": "nfs",
		"CONFIG_NFS_V4_1": "nfs",
		"CONFIG_NFS_V4":   "nfs",
	} {
		if configVal, exist := kernelConfigMap[configItem]; !exist {
			continue
		} else if configVal == "y" {
			isKernelSupport = true
			break
		} else if configVal == "m" {
			// Check if the module is loaded
			moduleLoaded, err := utils.IsModuleLoaded(module)
			if err != nil {
				logrus.Debugf("Failed to check if module %s is loaded: %v", module, err)
				continue
			}
			if moduleLoaded {
				isKernelSupport = true
				break
			}
		}
	}

	if !isKernelSupport {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, "kernel does not support NFSv4 (4.0/4.1/4.2)"))
		return nil

	}

	// check default NFS protocol version
	var isSupportedNFSVersion bool

	hostEtcDir := filepath.Join(consts.VolumeMountHostDirectory, commontypes.SysEtcDirectory)
	nfsMajor, nfsMinor, err := commonnfs.GetSystemDefaultNFSVersion(hostEtcDir)
	if err == nil {
		isSupportedNFSVersion = nfsMajor == 4 && (nfsMinor == 0 || nfsMinor == 1 || nfsMinor == 2)
	} else if errors.Is(err, commontypes.ErrNotConfigured) {
		// NFSv4 by default
		isSupportedNFSVersion = true
	} else {
		return wrapInternalError(topic, fmt.Errorf("failed to read NFS mount config: %v", err))
	}

	if !isSupportedNFSVersion {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, "NFSv4 is supported, but default protocol version is not 4, 4.1, or 4.2.  Please refer to the NFS mount configuration manual page for more information: man 5 nfsmount.conf"))
	}

	local.collection.Log.Info = append(local.collection.Log.Info, wrapMsgWithTopic(topic, "NFS4 is supported"))
	return nil
}

// checkKubeDNS checks if the DNS deployment in the Kubernetes cluster
// has multiple replicas and logs warnings if it does not.
//
// It retrieves the deployment in the "kube-system" namespace with a
// "kube-app: kube-dns" label and checks the number of replicas specified in
// the deployment spec. If the number of replicas is less than 2, it logs a
// warning indicating that Kube DNS is not set to run with multiple replicas.
// Additionally, it checks the number of ready replicas in the deployment
// status and logs a warning if there are fewer than 2 ready replicas.
//
// https://github.com/longhorn/longhorn/issues/9752
func (local *Checker) checkKubeDNS() error {
	logrus.Info("Checking if CoreDNS has multiple replicas")
	topic := formatTopic(consts.PreflightCheckTopicKubeDNS)

	deployments, err := commonkube.ListDeployments(local.kubeClient, metav1.NamespaceSystem, map[string]string{consts.KubeAppLabel: consts.KubeAppValueDNS})
	if err != nil {
		return wrapInternalError(topic, fmt.Errorf("failed to list Kube DNS with label %s=%s: %v",
			consts.KubeAppLabel, consts.KubeAppValueDNS, err))
	}

	if len(deployments.Items) != 1 {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, fmt.Sprintf(
				"found %d deployments with label %s=%s; expected exactly 1",
				len(deployments.Items), consts.KubeAppLabel, consts.KubeAppValueDNS)))
		return nil
	}

	deployment := deployments.Items[0]

	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas < 2 {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, fmt.Sprintf("Kube DNS %q is set with fewer than 2 replicas; consider increasing replica count for high availability", deployment.Name)))
		return nil
	}

	if deployment.Status.ReadyReplicas < 2 {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, fmt.Sprintf("Kube DNS %q has fewer than 2 ready replicas; some replicas may not be running or ready", deployment.Name)))
		return nil
	}

	local.collection.Log.Info = append(local.collection.Log.Info,
		wrapMsgWithTopic(topic,
			fmt.Sprintf("Kube DNS %q is set with %d replicas and %d ready replicas", deployment.Name, *deployment.Spec.Replicas, deployment.Status.ReadyReplicas)))

	return nil
}

func wrapMsgWithTopic(topic, msg string) string {
	return fmt.Sprintf("%s %s", topic, msg)
}

func wrapInternalError(topic string, err error) error {
	return fmt.Errorf("%s%s %w", topic, formatTopic(consts.PreflightCheckTopicInternalError), err)
}

func wrapAggregatedInternalError(topic, msg string, items map[string]any) error {
	return wrapInternalError(topic, errors.New(wrapMultItems(msg, items)))
}

// wrapMultItems aggregates multiple related errors under a common topic.
// It formats the errors into a user-friendly bullet list and returns a single wrapped error.
func wrapMultItems(msg string, items map[string]any) string {
	// Example usage:
	//
	//	return wrapMultItems("[Packages]", "The following packages are not installed:", map[string]error{
	//	    "nvme-cli": errors.New("command not found"),
	//	    "sg3_utils": errors.New("exit status 1"),
	//	})
	//
	// Sample output:
	//
	//	[Packages] The following packages are not installed:
	//	  (1) nvme-cli: command not found
	//	  (2) sg3_utils: exit status 1

	var msgBuilder strings.Builder
	msgBuilder.WriteString(msg)

	index := 1
	for set, content := range items {
		if content == nil {
			msgBuilder.WriteString(fmt.Sprintf("  (%d) %s", index, set))
		} else {
			msgBuilder.WriteString(fmt.Sprintf("  (%d) %s: %v", index, set, content))
		}
		index++
	}

	return msgBuilder.String()
}

func isExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}

func formatTopic(topics ...string) string {
	s := ""
	for _, topic := range topics {
		s += "[" + topic + "]"
	}
	return s
}
