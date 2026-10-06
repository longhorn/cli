package diagnose

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"k8s.io/utils/ptr"

	appsv1 "k8s.io/api/apps/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lhtypes "github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

// checkDaemonSet returns a check verifying that the DaemonSet pods are ready on all the nodes they are scheduled to.
func (remote *Diagnoser) checkDaemonSet(name string) checkFunc {
	return func(ctx context.Context, result *Result) error {
		daemonSet, err := remote.kubeClient.AppsV1().DaemonSets(remote.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to get DaemonSet %v", name)
		}

		evaluateDaemonSet(daemonSet, result)
		return nil
	}
}

// checkDeployment returns a check verifying that the Deployment has ready pods.
func (remote *Diagnoser) checkDeployment(name string) checkFunc {
	return func(ctx context.Context, result *Result) error {
		deployment, err := remote.kubeClient.AppsV1().Deployments(remote.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to get Deployment %v", name)
		}

		evaluateDeployment(deployment, result)
		return nil
	}
}

// checkService returns a check verifying that the Service has ready endpoints.
func (remote *Diagnoser) checkService(name string) checkFunc {
	return func(ctx context.Context, result *Result) error {
		if _, err := remote.kubeClient.CoreV1().Services(remote.Namespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
			return errors.Wrapf(err, "failed to get Service %v", name)
		}

		endpointSlices, err := remote.kubeClient.DiscoveryV1().EndpointSlices(remote.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%v=%v", discoveryv1.LabelServiceName, name),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to list EndpointSlices of Service %v", name)
		}

		evaluateEndpointSlices(name, endpointSlices.Items, result)
		return nil
	}
}

// checkNodes verifies that the Longhorn nodes are ready.
func (remote *Diagnoser) checkNodes(ctx context.Context, result *Result) error {
	nodes, err := remote.longhornClient.ListNodes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list Longhorn nodes")
	}

	evaluateNodes(nodes.Items, result)
	return nil
}

// checkEngineImages verifies that the engine images are deployed on all nodes.
func (remote *Diagnoser) checkEngineImages(ctx context.Context, result *Result) error {
	engineImages, err := remote.longhornClient.ListEngineImages(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list EngineImages")
	}

	evaluateEngineImages(engineImages.Items, result)
	return nil
}

// checkInstanceManagers verifies that the instance managers are running.
func (remote *Diagnoser) checkInstanceManagers(ctx context.Context, result *Result) error {
	instanceManagers, err := remote.longhornClient.ListInstanceManagers(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list InstanceManagers")
	}

	evaluateInstanceManagers(instanceManagers.Items, result)
	return nil
}

// checkVolumes verifies that no Longhorn volume is faulted.
func (remote *Diagnoser) checkVolumes(ctx context.Context, result *Result) error {
	volumes, err := remote.longhornClient.ListVolumes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list Longhorn volumes")
	}

	evaluateVolumes(volumes.Items, result)
	return nil
}

// checkCSIDriver verifies that the Longhorn CSIDriver is registered.
func (remote *Diagnoser) checkCSIDriver(ctx context.Context, result *Result) error {
	_, err := remote.kubeClient.StorageV1().CSIDrivers().Get(ctx, lhtypes.LonghornDriverName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get CSIDriver %v", lhtypes.LonghornDriverName)
	}

	result.infof("CSIDriver %v is registered", lhtypes.LonghornDriverName)
	return nil
}

func evaluateDaemonSet(daemonSet *appsv1.DaemonSet, result *Result) {
	desired := daemonSet.Status.DesiredNumberScheduled
	ready := daemonSet.Status.NumberReady

	switch {
	case desired == 0:
		result.errorf("DaemonSet %v has no pods scheduled", daemonSet.Name)
	case ready < desired:
		result.errorf("DaemonSet %v has %d/%d pods ready", daemonSet.Name, ready, desired)
	default:
		result.infof("DaemonSet %v has %d/%d pods ready", daemonSet.Name, ready, desired)
	}
}

func evaluateDeployment(deployment *appsv1.Deployment, result *Result) {
	desired := ptr.Deref(deployment.Spec.Replicas, 1)
	ready := deployment.Status.ReadyReplicas
	message := fmt.Sprintf("Deployment %v has %d/%d pods ready", deployment.Name, ready, desired)

	switch {
	case desired == 0:
		result.errorf("Deployment %v is scaled down to 0 replicas", deployment.Name)
	case ready == 0:
		result.errorf("%v", message)
	case ready < desired:
		result.warnf("%v", message)
	default:
		result.infof("%v", message)
	}
}

func evaluateEndpointSlices(serviceName string, endpointSlices []discoveryv1.EndpointSlice, result *Result) {
	// Dual-stack Services have an EndpointSlice per address type for the same pod.
	ready := map[string]struct{}{}
	for _, endpointSlice := range endpointSlices {
		for _, endpoint := range endpointSlice.Endpoints {
			// A nil ready condition is interpreted as ready by the API.
			if !ptr.Deref(endpoint.Conditions.Ready, true) {
				continue
			}

			key := strings.Join(endpoint.Addresses, ",")
			if endpoint.TargetRef != nil {
				key = fmt.Sprintf("%v/%v/%v", endpoint.TargetRef.Kind, endpoint.TargetRef.Namespace, endpoint.TargetRef.Name)
			}
			ready[key] = struct{}{}
		}
	}

	if len(ready) == 0 {
		result.errorf("Service %v has no ready endpoints", serviceName)
		return
	}
	result.infof("Service %v has %d ready endpoints", serviceName, len(ready))
}

func evaluateNodes(nodes []longhorn.Node, result *Result) {
	if len(nodes) == 0 {
		result.errorf("No Longhorn node is found")
		return
	}

	ready := 0
	for _, node := range nodes {
		readyCondition := lhtypes.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
		if readyCondition.Status != longhorn.ConditionStatusTrue {
			result.errorf("Node %v is not ready: %v", node.Name, describeCondition(readyCondition))
			continue
		}
		ready++

		for _, condition := range node.Status.Conditions {
			if condition.Type != longhorn.NodeConditionTypeReady && condition.Status == longhorn.ConditionStatusFalse {
				result.warnf("Node %v condition %v is false: %v", node.Name, condition.Type, describeCondition(condition))
			}
		}

		diskNames := make([]string, 0, len(node.Status.DiskStatus))
		for diskName := range node.Status.DiskStatus {
			diskNames = append(diskNames, diskName)
		}
		sort.Strings(diskNames)

		for _, diskName := range diskNames {
			disk := node.Status.DiskStatus[diskName]
			if disk == nil {
				continue
			}

			diskReadyCondition := lhtypes.GetCondition(disk.Conditions, longhorn.DiskConditionTypeReady)
			if diskReadyCondition.Status != longhorn.ConditionStatusTrue {
				result.warnf("Disk %v on node %v is not ready: %v", diskName, node.Name, describeCondition(diskReadyCondition))
				continue
			}

			diskSchedulableCondition := lhtypes.GetCondition(disk.Conditions, longhorn.DiskConditionTypeSchedulable)
			if diskSchedulableCondition.Status == longhorn.ConditionStatusFalse {
				result.warnf("Disk %v on node %v is not schedulable: %v", diskName, node.Name, describeCondition(diskSchedulableCondition))
			}
		}
	}

	result.infof("%d/%d nodes are ready", ready, len(nodes))
}

func evaluateEngineImages(engineImages []longhorn.EngineImage, result *Result) {
	if len(engineImages) == 0 {
		result.errorf("No EngineImage is found")
		return
	}

	for _, engineImage := range engineImages {
		if engineImage.Status.Incompatible {
			result.warnf("EngineImage %v (%v) is incompatible", engineImage.Name, engineImage.Spec.Image)
			continue
		}

		if engineImage.Status.State != longhorn.EngineImageStateDeployed {
			notDeployedNodes := []string{}
			for node, deployed := range engineImage.Status.NodeDeploymentMap {
				if !deployed {
					notDeployedNodes = append(notDeployedNodes, node)
				}
			}
			sort.Strings(notDeployedNodes)

			result.errorf("EngineImage %v (%v) is %v, not deployed on nodes %v",
				engineImage.Name, engineImage.Spec.Image, valueOrUnknown(string(engineImage.Status.State)), notDeployedNodes)
			continue
		}

		result.infof("EngineImage %v (%v) is deployed", engineImage.Name, engineImage.Spec.Image)
	}
}

func evaluateInstanceManagers(instanceManagers []longhorn.InstanceManager, result *Result) {
	if len(instanceManagers) == 0 {
		result.errorf("No InstanceManager is found")
		return
	}

	running := 0
	for i := range instanceManagers {
		instanceManager := &instanceManagers[i]

		checkInstanceManagerLabels(instanceManager, result)

		if instanceManager.Status.CurrentState != longhorn.InstanceManagerStateRunning {
			result.errorf("InstanceManager %v on node %v is %v",
				instanceManager.Name, instanceManager.Spec.NodeID, valueOrUnknown(string(instanceManager.Status.CurrentState)))
			continue
		}
		running++
	}

	result.infof("%d/%d instance managers are running", running, len(instanceManagers))
}

// checkInstanceManagerLabels verifies that the instance manager carries the
// labels the Longhorn controllers use to discover it (see
// types.GetInstanceManagerLabels in longhorn-manager). If any of these labels
// are missing or have unexpected values, the controllers are unable to find a
// valid instance manager, which prevents volume attachment and detachment.
func checkInstanceManagerLabels(instanceManager *longhorn.InstanceManager, result *Result) {
	expectedLabels := lhtypes.GetInstanceManagerLabels(
		instanceManager.Spec.NodeID, instanceManager.Spec.Image,
		instanceManager.Spec.Type, instanceManager.Spec.DataEngine,
	)

	missing := []string{}
	mismatched := []string{}
	for key, expectedValue := range expectedLabels {
		actualValue, ok := instanceManager.Labels[key]
		switch {
		case !ok:
			missing = append(missing, key)
		case actualValue != expectedValue:
			mismatched = append(mismatched, fmt.Sprintf("%s=%q (expected %q)", key, actualValue, expectedValue))
		}
	}
	slices.Sort(missing)
	slices.Sort(mismatched)

	for _, key := range missing {
		result.errorf("InstanceManager %v is missing critical label %q", instanceManager.Name, key)
	}
	for _, mismatch := range mismatched {
		result.errorf("InstanceManager %v has unexpected label value %v", instanceManager.Name, mismatch)
	}
}

func evaluateVolumes(volumes []longhorn.Volume, result *Result) {
	// Only the names of the unhealthy volumes are sorted, which are usually few.
	faulted, degraded := []string{}, []string{}
	for i := range volumes {
		switch volumes[i].Status.Robustness {
		case longhorn.VolumeRobustnessFaulted:
			faulted = append(faulted, volumes[i].Name)
		case longhorn.VolumeRobustnessDegraded:
			degraded = append(degraded, volumes[i].Name)
		}
	}
	slices.Sort(faulted)
	slices.Sort(degraded)

	for _, name := range faulted {
		result.errorf("Volume %v is faulted", name)
	}
	for _, name := range degraded {
		result.warnf("Volume %v is degraded", name)
	}

	result.infof("%d volumes are found, %d degraded, %d faulted", len(volumes), len(degraded), len(faulted))
}

// describeCondition returns the most informative description of the condition.
func describeCondition(condition longhorn.Condition) string {
	switch {
	case condition.Message != "":
		return condition.Message
	case condition.Reason != "":
		return condition.Reason
	default:
		return fmt.Sprintf("status is %v", valueOrUnknown(string(condition.Status)))
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
