package preflight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeclient "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
)

const maxV2ReportBytes = 256 * 1024

func decodeV2Report(data []byte) (*types.NodeCollection, error) {
	if len(data) == 0 || len(data) > maxV2ReportBytes {
		return nil, fmt.Errorf("missing or oversized V2 report")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSONValue(decoder, 0); err != nil {
		return nil, fmt.Errorf("invalid or ambiguous V2 report")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("V2 report contains trailing content")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("V2 report must be an object")
	}
	for name := range fields {
		if name != "dataEngineMode" && name != "log" {
			return nil, fmt.Errorf("V2 report has a noncanonical field")
		}
	}
	var logs map[string]json.RawMessage
	if err := json.Unmarshal(fields["log"], &logs); err != nil {
		return nil, fmt.Errorf("V2 report log must be an object")
	}
	for name := range logs {
		if name != "info" && name != "warn" && name != "error" {
			return nil, fmt.Errorf("V2 report log has a noncanonical field")
		}
	}
	var report types.NodeCollection
	if err := decoder.Decode(&report); err != nil {
		return nil, fmt.Errorf("V2 report has an invalid schema")
	}
	if report.DataEngineMode != string(types.DataEngineModeV2Only) || report.Log == nil || len(report.Log.Info) == 0 {
		return nil, fmt.Errorf("V2 report is missing capability or check evidence")
	}
	return &report, nil
}

func uniqueJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return fmt.Errorf("duplicate JSON key")
			}
			keys[name] = true
			if err := uniqueJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	end, err := decoder.Token()
	if err != nil || (delimiter == '{' && end != json.Delim('}')) || (delimiter == '[' && end != json.Delim(']')) {
		return fmt.Errorf("incomplete JSON value")
	}
	return nil
}

type v2ReportLogReader func(context.Context, string, string) ([]byte, error)

type v2PodEvidence struct {
	UID        string
	Name, Node string
	Spec       corev1.PodSpec
	Owners     []metav1.OwnerReference
	Instances  []corev1.ContainerStatus
}

// The legacy collector intentionally tolerates missing logs. V2 qualification
// uses a separate complete inventory and brackets logs against exact instances.
func collectV2Reports(ctx context.Context, client kubeclient.Interface, expected *appsv1.DaemonSet, readLog v2ReportLogReader) (string, error) {
	return collectV2OperationReports(ctx, client, expected, readLog, consts.SubCmdCheck)
}

func collectV2OperationReports(ctx context.Context, client kubeclient.Interface, expected *appsv1.DaemonSet, readLog v2ReportLogReader, operation string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if expected == nil || expected.UID == "" || expected.Generation <= 0 || expected.Spec.Selector == nil {
		return "", fmt.Errorf("V2 collection requires a newly created identified DaemonSet")
	}
	beforeDS, beforePods, err := snapshotV2Checker(ctx, client, expected, operation)
	if err != nil {
		return "", err
	}
	nodes := map[string]*types.LogCollection{}
	qualified := true
	total := 0
	for _, pod := range beforePods {
		data, err := readLog(ctx, expected.Namespace, pod.Name)
		if err != nil {
			return "", fmt.Errorf("failed to read a complete V2 report for node %s: %w", pod.Node, err)
		}
		total += len(data)
		if total > 32*1024*1024 {
			return "", fmt.Errorf("V2 report collection exceeds total limit")
		}
		report, err := decodeV2Report(data)
		if err != nil {
			return "", fmt.Errorf("invalid V2 report for node %s: %w", pod.Node, err)
		}
		nodes[pod.Node] = report.Log
		qualified = qualified && len(report.Log.Error) == 0
	}
	afterDS, afterPods, err := snapshotV2Checker(ctx, client, expected, operation)
	if err != nil {
		return "", err
	}
	if beforeDS.Status.DesiredNumberScheduled != afterDS.Status.DesiredNumberScheduled || !reflect.DeepEqual(beforePods, afterPods) {
		return "", fmt.Errorf("V2 checker membership or execution changed while collecting reports")
	}
	output, err := yaml.Marshal(nodes)
	if err != nil {
		return "", err
	}
	if !qualified {
		return string(output), fmt.Errorf("V2-only host preflight is not qualified; inspect the complete report")
	}
	return string(output), nil
}

func snapshotV2Checker(ctx context.Context, client kubeclient.Interface, expected *appsv1.DaemonSet, operation string) (*appsv1.DaemonSet, []v2PodEvidence, error) {
	ds, err := client.AppsV1().DaemonSets(expected.Namespace).Get(ctx, expected.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	if ds.UID != expected.UID || ds.Generation != expected.Generation || ds.DeletionTimestamp != nil || !reflect.DeepEqual(ds.Spec, expected.Spec) || ds.Status.ObservedGeneration < ds.Generation || ds.Status.DesiredNumberScheduled <= 0 || ds.Status.DesiredNumberScheduled > 4096 || ds.Status.CurrentNumberScheduled != ds.Status.DesiredNumberScheduled || ds.Status.UpdatedNumberScheduled != ds.Status.DesiredNumberScheduled || ds.Status.NumberMisscheduled != 0 {
		return nil, nil, fmt.Errorf("V2 checker DaemonSet identity, specification or membership is unqualified")
	}
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil || selector.Empty() {
		return nil, nil, fmt.Errorf("V2 checker requires a specific selector")
	}
	evidence := []v2PodEvidence{}
	seenNodes, seenUIDs, seenNames := map[string]bool{}, map[string]bool{}, map[string]bool{}
	continuation, resourceVersion := "", ""
	seenContinuations := map[string]bool{}
	total := 0
	for page := 0; ; page++ {
		if page >= 32 {
			return nil, nil, fmt.Errorf("V2 Pod inventory exceeds page limit")
		}
		pods, err := client.CoreV1().Pods(ds.Namespace).List(ctx, metav1.ListOptions{Limit: 256, Continue: continuation})
		if err != nil {
			return nil, nil, err
		}
		if pods.ResourceVersion == "" || (resourceVersion != "" && pods.ResourceVersion != resourceVersion) {
			return nil, nil, fmt.Errorf("V2 Pod inventory is not a complete consistent snapshot")
		}
		resourceVersion = pods.ResourceVersion
		total += len(pods.Items)
		if total > 8192 {
			return nil, nil, fmt.Errorf("V2 Pod inventory exceeds object limit")
		}
		for _, pod := range pods.Items {
			owned, controllerCount := false, 0
			for _, owner := range pod.OwnerReferences {
				if owner.Controller != nil && *owner.Controller {
					controllerCount++
					owned = owner.UID == ds.UID && owner.Name == ds.Name && owner.Kind == "DaemonSet" && owner.APIVersion == "apps/v1"
				}
			}
			matches := selector.Matches(labels.Set(pod.Labels))
			if !owned && !matches {
				// A non-controlling reference must not hide an owned-looking Pod.
				for _, owner := range pod.OwnerReferences {
					if owner.UID == ds.UID {
						return nil, nil, fmt.Errorf("V2 checker Pod has an ambiguous owner")
					}
				}
				continue
			}
			if !owned || !matches || controllerCount != 1 || pod.UID == "" || pod.Name == "" || pod.Namespace != ds.Namespace || pod.Spec.NodeName == "" || pod.DeletionTimestamp != nil || seenNodes[pod.Spec.NodeName] || seenUIDs[string(pod.UID)] || seenNames[pod.Name] {
				return nil, nil, fmt.Errorf("V2 checker Pod inventory has missing, foreign or duplicate execution")
			}
			instances, err := v2PodInstances(ds, &pod, operation)
			if err != nil {
				return nil, nil, err
			}
			seenNodes[pod.Spec.NodeName], seenUIDs[string(pod.UID)], seenNames[pod.Name] = true, true, true
			evidence = append(evidence, v2PodEvidence{UID: string(pod.UID), Name: pod.Name, Node: pod.Spec.NodeName, Spec: pod.Spec, Owners: pod.OwnerReferences, Instances: instances})
		}
		continuation = pods.Continue
		if continuation == "" {
			break
		}
		if seenContinuations[continuation] {
			return nil, nil, fmt.Errorf("V2 Pod inventory repeated a continuation")
		}
		seenContinuations[continuation] = true
	}
	if len(evidence) != int(ds.Status.DesiredNumberScheduled) {
		return nil, nil, fmt.Errorf("V2 checker is missing a report-producing Pod")
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].Node < evidence[j].Node })
	return ds, evidence, nil
}

func v2PodInstances(ds *appsv1.DaemonSet, pod *corev1.Pod, operation string) ([]corev1.ContainerStatus, error) {
	template := ds.Spec.Template.Spec
	if !validV2CheckerTemplate(template, operation) || len(pod.Spec.InitContainers) != 2 || len(pod.Spec.EphemeralContainers) != 0 || len(pod.Spec.Containers) != len(template.Containers) || pod.Spec.ServiceAccountName != template.ServiceAccountName || pod.Spec.HostPID != template.HostPID {
		return nil, fmt.Errorf("V2 checker Pod execution does not match its DaemonSet")
	}
	for _, volume := range template.Volumes {
		found := false
		for _, actual := range pod.Spec.Volumes {
			if actual.Name == volume.Name {
				found = reflect.DeepEqual(actual, volume)
			}
		}
		if !found {
			return nil, fmt.Errorf("V2 checker output or host volume changed")
		}
	}
	serviceAccountVolume, valid := v2AdditionalVolumes(template.Volumes, pod.Spec.Volumes)
	if !valid {
		return nil, fmt.Errorf("V2 checker has an unrecognized additional volume")
	}
	instances := make([]corev1.ContainerStatus, 0, 2)
	for index, expected := range template.InitContainers {
		actual := pod.Spec.InitContainers[index]
		if !sameV2ContainerExecution(expected, actual, serviceAccountVolume) {
			return nil, fmt.Errorf("V2 checker or reporter execution changed")
		}
		var status *corev1.ContainerStatus
		for _, candidate := range pod.Status.InitContainerStatuses {
			if candidate.Name == expected.Name {
				if status != nil {
					return nil, fmt.Errorf("duplicate V2 checker container status")
				}
				copy := candidate
				status = &copy
			}
		}
		if status == nil || status.State.Terminated == nil || status.ImageID == "" || status.State.Terminated.ExitCode != 0 || status.State.Terminated.ContainerID == "" || status.State.Terminated.StartedAt.IsZero() || status.State.Terminated.FinishedAt.IsZero() || status.State.Terminated.FinishedAt.Before(&status.State.Terminated.StartedAt) || (status.ContainerID != "" && status.ContainerID != status.State.Terminated.ContainerID) {
			return nil, fmt.Errorf("V2 checker or reporter did not complete successfully")
		}
		if index == 1 && status.State.Terminated.StartedAt.Before(&instances[0].State.Terminated.FinishedAt) {
			return nil, fmt.Errorf("V2 reporter ran before its checker finished")
		}
		instances = append(instances, *status)
	}
	for index, expected := range template.Containers {
		if !sameV2ContainerExecution(expected, pod.Spec.Containers[index], serviceAccountVolume) {
			return nil, fmt.Errorf("V2 checker sidecar execution changed")
		}
	}
	return instances, nil
}

func validV2CheckerTemplate(template corev1.PodSpec, operation string) bool {
	if operation != consts.SubCmdCheck && operation != consts.SubCmdInstall {
		return false
	}
	if len(template.InitContainers) != 2 || template.InitContainers[0].Name != consts.ContainerNameInit || template.InitContainers[1].Name != consts.ContainerNameOutput {
		return false
	}
	checker, reporter := template.InitContainers[0], template.InitContainers[1]
	if !reflect.DeepEqual(checker.Command, []string{consts.CmdLonghornctlLocal, operation, consts.SubCmdPreflight, "--" + consts.CmdOptDataEngineMode, string(types.DataEngineModeV2Only)}) || len(checker.Args) != 0 || !reflect.DeepEqual(reporter.Command, []string{"cat", "/shared/output.json"}) || len(reporter.Args) != 0 {
		return false
	}
	environment := map[string]corev1.EnvVar{}
	for _, variable := range checker.Env {
		if _, exists := environment[variable.Name]; exists {
			return false
		}
		environment[variable.Name] = variable
	}
	mode, collection, node := environment[consts.EnvDataEngineMode], environment[consts.EnvPreflightCollectionOnly], environment[consts.EnvCurrentNodeID]
	collectionValue := "true"
	if operation == consts.SubCmdInstall {
		collectionValue = ""
	}
	return mode.Value == string(types.DataEngineModeV2Only) && mode.ValueFrom == nil && collection.Value == collectionValue && collection.ValueFrom == nil && node.Value == "" && node.ValueFrom != nil && node.ValueFrom.FieldRef != nil && node.ValueFrom.FieldRef.FieldPath == "spec.nodeName"
}

func sameV2ContainerExecution(expected, actual corev1.Container, serviceAccountVolume string) bool {
	if expected.Name != actual.Name || expected.Image != actual.Image || expected.WorkingDir != actual.WorkingDir || expected.Stdin != actual.Stdin || expected.TTY != actual.TTY || len(actual.VolumeDevices) != 0 || !reflect.DeepEqual(expected.Command, actual.Command) || !reflect.DeepEqual(expected.Args, actual.Args) || !reflect.DeepEqual(expected.EnvFrom, actual.EnvFrom) || !reflect.DeepEqual(expected.SecurityContext, actual.SecurityContext) || !reflect.DeepEqual(expected.Lifecycle, actual.Lifecycle) || len(expected.Env) != len(actual.Env) {
		return false
	}
	seen := map[string]bool{}
	for index, variable := range actual.Env {
		if seen[variable.Name] {
			return false
		}
		seen[variable.Name] = true
		left, right := expected.Env[index].DeepCopy(), variable.DeepCopy()
		for _, value := range []*corev1.EnvVar{left, right} {
			if value.ValueFrom != nil && value.ValueFrom.FieldRef != nil && value.ValueFrom.FieldRef.APIVersion == "" {
				value.ValueFrom.FieldRef.APIVersion = "v1"
			}
		}
		if !reflect.DeepEqual(left, right) {
			return false
		}
	}
	for _, mount := range expected.VolumeMounts {
		found := false
		for _, actualMount := range actual.VolumeMounts {
			if actualMount.Name == mount.Name {
				found = reflect.DeepEqual(mount, actualMount)
			}
		}
		if !found {
			return false
		}
	}
	for _, mount := range actual.VolumeMounts {
		found := false
		for _, expectedMount := range expected.VolumeMounts {
			if reflect.DeepEqual(mount, expectedMount) {
				found = true
			}
		}
		if !found && (serviceAccountVolume == "" || !reflect.DeepEqual(mount, corev1.VolumeMount{Name: serviceAccountVolume, MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true})) {
			return false
		}
	}
	return true
}

// Pod admission adds one API-token volume which is absent from the DaemonSet
// template. No other injected volume or mount may shadow the checker/report.
func v2AdditionalVolumes(expected, actual []corev1.Volume) (string, bool) {
	names := map[string]bool{}
	for _, volume := range expected {
		names[volume.Name] = true
	}
	serviceAccountVolume := ""
	seen := map[string]bool{}
	for _, volume := range actual {
		if seen[volume.Name] {
			return "", false
		}
		seen[volume.Name] = true
		if names[volume.Name] {
			continue
		}
		projection := volume.Projected
		if serviceAccountVolume != "" || !strings.HasPrefix(volume.Name, "kube-api-access-") || projection == nil || len(projection.Sources) != 3 || !reflect.DeepEqual(volume.VolumeSource, corev1.VolumeSource{Projected: projection}) {
			return "", false
		}
		serviceAccountVolume = volume.Name
		foundToken, foundCA, foundNamespace := false, false, false
		for _, source := range projection.Sources {
			switch {
			case source.ServiceAccountToken != nil:
				token := source.ServiceAccountToken
				if foundToken || token.Path != "token" || !reflect.DeepEqual(source, corev1.VolumeProjection{ServiceAccountToken: token}) {
					return "", false
				}
				foundToken = true
			case source.ConfigMap != nil:
				config := source.ConfigMap
				if foundCA || config.Name != "kube-root-ca.crt" || len(config.Items) != 1 || config.Items[0].Key != "ca.crt" || config.Items[0].Path != "ca.crt" || !reflect.DeepEqual(source, corev1.VolumeProjection{ConfigMap: config}) {
					return "", false
				}
				foundCA = true
			case source.DownwardAPI != nil:
				info := source.DownwardAPI
				if foundNamespace || len(info.Items) != 1 || info.Items[0].Path != "namespace" || info.Items[0].FieldRef == nil || info.Items[0].FieldRef.FieldPath != "metadata.namespace" || info.Items[0].ResourceFieldRef != nil || !reflect.DeepEqual(source, corev1.VolumeProjection{DownwardAPI: info}) {
					return "", false
				}
				foundNamespace = true
			default:
				return "", false
			}
		}
		if !foundToken || !foundCA || !foundNamespace {
			return "", false
		}
	}
	return serviceAccountVolume, true
}

func readV2PodReport(client kubeclient.Interface) v2ReportLogReader {
	return func(ctx context.Context, namespace, pod string) ([]byte, error) {
		limit := int64(maxV2ReportBytes + 1)
		stream, err := client.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{Container: consts.ContainerNameOutput, LimitBytes: &limit}).Stream(ctx)
		if err != nil {
			return nil, err
		}
		defer stream.Close()
		return io.ReadAll(io.LimitReader(stream, limit))
	}
}

// Bind the admitted Create response to this invocation's requested inputs.
// Otherwise an admission rewrite could lower the requested hugepage check or
// replace the fresh output volume before either inventory sample is taken.
func validateV2CreatedDaemonSet(request, created *appsv1.DaemonSet, operation string) error {
	if created == nil || created.UID == "" || created.Generation <= 0 || created.Name != request.Name || created.Namespace != request.Namespace {
		return fmt.Errorf("V2 DaemonSet creation returned no identified receipt")
	}
	left, right := request.Spec.Template.Spec, created.Spec.Template.Spec
	if !validV2CheckerTemplate(right, operation) || !reflect.DeepEqual(request.Spec.Selector, created.Spec.Selector) || !reflect.DeepEqual(left.NodeSelector, right.NodeSelector) || !reflect.DeepEqual(left.Affinity, right.Affinity) || !reflect.DeepEqual(left.Tolerations, right.Tolerations) || left.NodeName != right.NodeName || left.ServiceAccountName != right.ServiceAccountName || left.HostPID != right.HostPID || left.HostNetwork != right.HostNetwork || left.HostIPC != right.HostIPC || len(left.InitContainers) != len(right.InitContainers) || len(left.Containers) != len(right.Containers) || len(right.EphemeralContainers) != 0 || len(left.Volumes) != len(right.Volumes) {
		return fmt.Errorf("V2 DaemonSet admission changed required execution or node coverage")
	}
	for index, container := range left.InitContainers {
		if !sameV2ContainerExecution(container, right.InitContainers[index], "") {
			return fmt.Errorf("V2 DaemonSet admission changed checker inputs")
		}
	}
	for index, container := range left.Containers {
		if !sameV2ContainerExecution(container, right.Containers[index], "") {
			return fmt.Errorf("V2 DaemonSet admission changed companion execution")
		}
	}
	for index, volume := range left.Volumes {
		actual := right.Volumes[index].DeepCopy()
		expected := volume.DeepCopy()
		for _, item := range []*corev1.Volume{expected, actual} {
			if item.HostPath != nil && item.HostPath.Type != nil && *item.HostPath.Type == corev1.HostPathUnset {
				item.HostPath.Type = nil
			}
		}
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Errorf("V2 DaemonSet admission changed the host or output volume")
		}
	}
	return nil
}
