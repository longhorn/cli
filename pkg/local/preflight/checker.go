package preflight

import (
	"encoding/json"
	"fmt"
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
		if err := checkTaskFn(); err != nil {
			local.collection.Log.Error = append(local.collection.Log.Error, err.Error())
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
	const topic = consts.PreflightCheckTopicContainerOptimizedOS

	daemonSet, err := commonkube.GetDaemonSet(local.kubeClient, metav1.NamespaceDefault, consts.AppNamePreflightContainerOptimizedOS)
	if err != nil {
		return wrapErrorWithTopic(topic, errors.Wrapf(err,
			"failed to retrieve DaemonSet %q in namespace %q. Please ensure the preflight DaemonSet is deployed correctly",
			consts.AppNamePreflightContainerOptimizedOS, metav1.NamespaceDefault))
	}

	if !commonkube.IsDaemonSetReady(daemonSet) {
		return wrapErrorWithTopic(topic, errors.Errorf(
			"daemonSet %q is not ready in namespace %q.\nPlease check its pod status",
			consts.AppNamePreflightContainerOptimizedOS, metav1.NamespaceDefault))
	}
	return nil
}

// checkMultipathService checks if the multipathd service is running.
func (local *Checker) checkMultipathService() error {
	logrus.Info("Checking multipathd service status")
	const topic = consts.PreflightCheckTopicMultipathService

	_, err := local.packageManager.GetServiceStatus("multipathd.service")
	if err == nil {
		msg := "multipathd.service is running. Please refer to https://longhorn.io/kb/troubleshooting-volume-with-multipath/ for more information."
		local.collection.Log.Warn = append(local.collection.Log.Warn, wrapMsgWithTopic(topic, msg))
		return nil
	}

	_, err = local.packageManager.GetServiceStatus("multipathd.socket")
	if err == nil {
		msg := "multipathd.service is inactive, but it can still be activated by multipathd.socket."
		local.collection.Log.Warn = append(local.collection.Log.Warn, wrapMsgWithTopic(topic, msg))
		return nil
	}

	return nil
}

// checkIscsidService checks if the iscsid service is running.
func (local *Checker) checkIscsidService() error {
	logrus.Info("Checking iscsid service status")
	const topic = consts.PreflightCheckTopicIscsidService

	_, err := local.packageManager.GetServiceStatus("iscsid.service")
	if err == nil {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, "Service iscsid is running"))
		return nil
	}

	_, err = local.packageManager.GetServiceStatus("iscsid.socket")
	if err == nil {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, "Service iscsid is inactive, but it can still be activated by iscsid.socket"))
		return nil
	}

	return wrapErrorWithTopic(topic, fmt.Errorf("neither iscsid.service nor iscsid.socket is running"))
}

// checkHugePages checks if HugePages is enabled.
func (local *Checker) checkHugePages() error {
	logrus.Info("Checking if HugePages is enabled")
	const topic = consts.PreflightCheckTopicHugePages

	if local.HugePageSize == 0 {
		logrus.Error("HUGEMEM environment variable is not set")
		return nil
	}

	pages := local.HugePageSize >> 1

	ok, hugePagesTotalNum, requiredHugePages, err := local.isHugePagesTotalEqualOrLargerThan(pages)
	if err != nil {
		return wrapErrorWithTopic(topic, errors.Wrap(err, "failed to check HugePages"))
	}
	if !ok {
		return wrapErrorWithTopic(topic, fmt.Errorf("HugePages are insufficient. Required 2MiB HugePages: %v pages, Available: %v pages", requiredHugePages, hugePagesTotalNum))
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
	topic := consts.PreflightCheckTopicSPDK + consts.PreflightCheckTopicCpuInstructionSet

	arch := runtime.GOARCH
	logrus.Infof("Detected CPU architecture: %v", arch)

	sets, ok := instructionSets[arch]
	if !ok {
		return wrapErrorWithTopic(topic, fmt.Errorf("CPU model is not supported: %v", arch))
	}

	unsupported := map[string]interface{}{}
	supported := map[string]interface{}{}
	for _, set := range sets {
		_, err := local.packageManager.Execute([]string{}, "grep", []string{set, "/proc/cpuinfo"}, commontypes.ExecuteNoTimeout)
		if err != nil {
			unsupported[set] = err
		} else {
			supported[set] = nil
		}
	}

	if len(supported) > 0 {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMultiInfos(topic, "The following CPU instruction sets are supported:", supported))
	}

	if len(unsupported) > 0 {
		return wrapMultiErrors(topic, "The following CPU instruction sets are missing or unsupported:\n", unsupported)
	}

	return nil
}

// checkPackagesInstalled checks if the packages are installed.
func (local *Checker) checkPackagesInstalled(spdkDependent bool) error {
	topic := consts.PreflightCheckTopicPackages

	packages := local.packages
	if spdkDependent {
		topic = consts.PreflightCheckTopicSPDK + topic
		packages = local.spdkDepPackages
	}

	if len(packages) == 0 {
		return nil
	}

	logrus.Info("Checking if required packages are installed")

	nonInstalled := map[string]interface{}{}
	installed := map[string]interface{}{}
	for _, pkg := range packages {
		_, err := local.packageManager.CheckPackageInstalled(pkg)
		if err != nil {
			nonInstalled[pkg] = err
		} else {
			installed[pkg] = nil
		}
	}

	if len(installed) > 0 {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMultiInfos(topic, "The following packages are installed:", installed))
	}

	if len(nonInstalled) > 0 {
		return wrapMultiErrors(topic, "The following packages are not installed:\n", nonInstalled)
	}

	return nil
}

// checkModulesLoaded checks if the modules are loaded.
func (local *Checker) checkModulesLoaded(spdkDependent bool) error {
	topic := consts.PreflightCheckTopicKernelModules

	modules := local.modules
	if spdkDependent {
		modules = local.spdkDepModules
		topic = consts.PreflightCheckTopicSPDK + topic

		if local.UserspaceDriver != "" {
			modules = append(modules, local.UserspaceDriver)
		}
	}

	if len(modules) == 0 {
		return nil
	}

	logrus.Info("Checking if required modules are loaded")

	notLoaded := map[string]interface{}{}
	loaded := map[string]interface{}{}
	for _, mod := range modules {
		logrus.Infof("Checking if module %s is loaded", mod)

		err := local.packageManager.CheckModLoaded(mod)
		if err != nil {
			notLoaded[mod] = err
		} else {
			loaded[mod] = nil
		}
	}

	if len(loaded) > 0 {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMultiInfos(topic, "The following kernel modules are loaded:", loaded))
	}

	if len(notLoaded) > 0 {
		return wrapMultiErrors(topic, "The following kernel modules are not loaded:\n", notLoaded)
	}

	return nil
}

// checkNFSv4Support checks if NFS4 is supported on the host.
func (local *Checker) checkNFSv4Support() error {
	logrus.Info("Checking if NFS4 (either 4.0, 4.1 or 4.2) is supported")
	const topic = consts.PreflightCheckTopicNFS

	// check kernel capability
	var isKernelSupport = false

	kernelVersion, err := utils.GetKernelVersion()
	if err != nil {
		return wrapErrorWithTopic(topic, fmt.Errorf("failed to detect kernel version: %v", err))
	}
	hostBootDir := filepath.Join(consts.VolumeMountHostDirectory, commontypes.SysBootDirectory)
	kernelConfigMap, err := commonsys.GetBootKernelConfigMap(hostBootDir, kernelVersion)
	if err != nil {
		return wrapErrorWithTopic(topic, fmt.Errorf("failed to read kernel config: %v", err))
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
		return wrapErrorWithTopic(topic, fmt.Errorf("kernel does not support NFSv4 (4.0/4.1/4.2)"))
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
		return wrapErrorWithTopic(topic, fmt.Errorf("failed to read NFS mount config: %v", err))
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
	const topic = consts.PreflightCheckTopicKubeDNS

	deployments, err := commonkube.ListDeployments(local.kubeClient, metav1.NamespaceSystem, map[string]string{consts.KubeAppLabel: consts.KubeAppValueDNS})
	if err != nil {
		return wrapErrorWithTopic(topic,
			fmt.Errorf("failed to list Kube DNS with label %s=%s: %v",
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

func wrapErrorWithTopic(topic string, err error) error {
	return fmt.Errorf("%s: %w", topic, err)
}

func wrapMultiErrors(topic, msg string, items map[string]interface{}) error {
	return wrapErrorWithTopic(topic, errors.New(wrapMultItems(msg, items)))
}

func wrapMultiInfos(topic, msg string, items map[string]interface{}) string {
	return wrapMsgWithTopic(topic, wrapMultItems(msg, items))
}

// wrapMultiErrors aggregates multiple related errors under a common topic.
// It formats the errors into a user-friendly bullet list and returns a single wrapped error.
func wrapMultItems(msg string, items map[string]interface{}) string {
	// Example usage:
	//
	//	return wrapMultiErrors("[Packages]", "The following packages are not installed:\n", map[string]error{
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
			msgBuilder.WriteString(fmt.Sprintf("  (%d) %s: %v\n", index, set, content))
		}
		index++
	}

	return msgBuilder.String()
}
