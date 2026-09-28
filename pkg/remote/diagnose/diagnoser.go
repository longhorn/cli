package diagnose

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	kubeclient "k8s.io/client-go/kubernetes"

	lhtypes "github.com/longhorn/longhorn-manager/types"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
	utilslonghorn "github.com/longhorn/cli/pkg/utils/longhorn"
)

// Diagnoser provides functions to diagnose the health of the Longhorn system.
// It only reads the cluster state and never creates or modifies any resource.
type Diagnoser struct {
	types.GlobalCmdOptions

	kubeClient     kubeclient.Interface
	longhornClient *utilslonghorn.LonghornClient
}

const checkTimeout = 30 * time.Second

// check is a single health check of the Longhorn system.
type check struct {
	name string
	run  checkFunc
}

// checkFunc records the findings of a check in the result.
// A returned error means the check could not be completed, and it is recorded as a failure.
type checkFunc func(ctx context.Context, result *Result) error

// checks returns the checks to run, ordered so that a component is checked before the components depending on it.
// To add a new check, implement a checkFunc and add it to this list.
func (remote *Diagnoser) checks() []check {
	return []check{
		{name: "LonghornManager", run: remote.checkDaemonSet(lhtypes.LonghornManagerDaemonSetName)},
		{name: "LonghornBackend", run: remote.checkService(consts.LonghornBackendServiceName)},
		{name: "LonghornNodes", run: remote.checkNodes},
		{name: "EngineImages", run: remote.checkEngineImages},
		{name: "InstanceManagers", run: remote.checkInstanceManagers},
		{name: "CSIDriver", run: remote.checkCSIDriver},
		{name: "CSIPlugin", run: remote.checkDaemonSet(lhtypes.CSIPluginName)},
		{name: "CSIAttacher", run: remote.checkDeployment(lhtypes.CSIAttacherName)},
		{name: "CSIProvisioner", run: remote.checkDeployment(lhtypes.CSIProvisionerName)},
		{name: "CSIResizer", run: remote.checkDeployment(lhtypes.CSIResizerName)},
		{name: "CSISnapshotter", run: remote.checkDeployment(lhtypes.CSISnapshotterName)},
		{name: "LonghornUI", run: remote.checkDeployment(lhtypes.LonghornUIDeploymentName)},
		{name: "Volumes", run: remote.checkVolumes},
	}
}

// Init initializes the Diagnoser.
func (remote *Diagnoser) Init() error {
	kubeClient, err := kubeutils.NewKubeClient("", remote.KubeConfigPath)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	longhornClient, err := utilslonghorn.NewLonghornClient(remote.KubeConfigPath, remote.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to initialize Longhorn client")
	}
	remote.longhornClient = longhornClient

	return nil
}

// Run executes the checks and returns their results.
func (remote *Diagnoser) Run() Results {
	ctx := context.Background()

	results := Results{}
	for _, check := range remote.checks() {
		logrus.WithField("check", check.name).Info("Running check")

		result := &Result{Name: check.name}
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		err := check.run(checkCtx, result)
		cancel()
		if err != nil {
			result.errorf("%v", err)
		}
		result.updateStatus()

		results = append(results, result)
	}

	return results
}
