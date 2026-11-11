package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
)

func TestV2ReportRejectsMissingAmbiguousAndPartialEvidence(t *testing.T) {
	for _, input := range []string{
		``, `{}`, `{"dataEngineMode":"v2-only","log":null}`,
		`{"dataEngineMode":"v2-only","log":{}}`,
		`{"dataEngineMode":"v1-capable","log":{"info":["ok"]}}`,
		`{"dataEngineMode":"v2-only","dataEngineMode":"v2-only","log":{"info":["ok"]}}`,
		`{"dataEngineMode":"v2-only","log":{"info":["ok"],"info":["ok"]}}`,
		`{"dataEngineMode":"v2-only","log":{"info":["ok"],"unknown":true}}`,
		`{"dataEngineMode":"v2-only","DataEngineMode":"v2-only","log":{"info":["ok"]}}`,
		`{"dataEngineMode":"v2-only","log":{"Info":["ok"]}}`,
		`{"dataEngineMode":"v2-only","log":{"info":["ok"]}} {}`,
		`{"dataEngineMode":"v2-only","log":{"info":["ok"]}`,
		strings.Repeat(" ", maxV2ReportBytes+1),
	} {
		if _, err := decodeV2Report([]byte(input)); err == nil {
			t.Fatal("accepted missing, ambiguous, partial or unbounded V2 report")
		}
	}
	for _, input := range []string{
		`{"dataEngineMode":"v2-only","log":{"info":["qualified"]}}`,
		`{"dataEngineMode":"v2-only","log":{"info":["checks ran"],"error":["NVMe missing"]}}`,
	} {
		if _, err := decodeV2Report([]byte(input)); err != nil {
			t.Fatalf("lost complete report: %v", err)
		}
	}
}

type v2CollectionFixture struct {
	mu                 sync.Mutex
	ds                 *appsv1.DaemonSet
	pods               []corev1.Pod
	paginate, failList bool
}

func newV2CollectionFixture(t *testing.T) (*v2CollectionFixture, *kubeclient.Clientset, *appsv1.DaemonSet) {
	t.Helper()
	checker := Checker{CheckerCmdOptions: CheckerCmdOptions{DataEngineMode: string(types.DataEngineModeV2Only), GlobalCmdOptions: types.GlobalCmdOptions{Namespace: "test", Image: "fixture/cli:current"}, HugePageSize: 2048}, appName: "checker"}
	ds := checker.newDaemonSet()
	ds.UID, ds.Generation, ds.ResourceVersion = "ds-uid", 1, "1"
	ds.Status = appsv1.DaemonSetStatus{ObservedGeneration: 1, DesiredNumberScheduled: 1, CurrentNumberScheduled: 1, UpdatedNumberScheduled: 1}
	// The expected object is the API Create response, not the pre-serialization
	// request: omitted empty slices have already become nil on that response.
	encoded, err := json.Marshal(ds)
	if err != nil {
		t.Fatal(err)
	}
	var admitted appsv1.DaemonSet
	if err := json.Unmarshal(encoded, &admitted); err != nil {
		t.Fatal(err)
	}
	ds = &admitted
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "checker-node", Namespace: "test", UID: "pod-uid", Labels: map[string]string{"app": "checker"}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: ptr.To(true)}}}, Spec: *ds.Spec.Template.Spec.DeepCopy()}
	pod.Spec.NodeName = "node-1"
	for index, name := range []string{consts.ContainerNameInit, consts.ContainerNameOutput} {
		start := metav1.NewTime(time.Unix(100+int64(index)*2, 0))
		end := metav1.NewTime(start.Add(time.Second))
		id := fmt.Sprintf("containerd://instance-%d", index)
		pod.Status.InitContainerStatuses = append(pod.Status.InitContainerStatuses, corev1.ContainerStatus{Name: name, Image: ds.Spec.Template.Spec.InitContainers[index].Image, ImageID: "sha256:fixture", ContainerID: id, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, StartedAt: start, FinishedAt: end, ContainerID: id}}})
	}
	fixture := &v2CollectionFixture{ds: ds.DeepCopy(), pods: []corev1.Pod{pod}}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		response.Header().Set("Content-Type", "application/json")
		var object interface{}
		switch request.URL.Path {
		case "/apis/apps/v1/namespaces/test/daemonsets/checker":
			object = fixture.ds
		case "/api/v1/namespaces/test/pods":
			if request.URL.Query().Get("labelSelector") != "" {
				t.Error("V2 collector used an incomplete label-filtered inventory")
			}
			if fixture.failList {
				http.Error(response, "inventory unavailable", http.StatusForbidden)
				return
			}
			list := &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "10"}, Items: fixture.pods}
			if fixture.paginate && request.URL.Query().Get("continue") == "" {
				list.Items = nil
				list.Continue = "next"
			}
			object = list
		default:
			t.Errorf("unexpected API request %s", request.URL.Path)
			http.NotFound(response, request)
			return
		}
		if err := json.NewEncoder(response).Encode(object); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client, err := kubeclient.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return fixture, client, ds
}

func TestV2CollectorBindsCompleteCurrentExecution(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after func(*v2CollectionFixture)
		logError      bool
		valid         bool
	}{
		{name: "complete", valid: true},
		{name: "native-service-account-projection", before: addV2FixtureServiceAccount, valid: true},
		{name: "complete-paginated", before: func(f *v2CollectionFixture) { f.paginate = true }, valid: true},
		{name: "missing-pod", before: func(f *v2CollectionFixture) { f.pods = nil }},
		{name: "lost-labels", before: func(f *v2CollectionFixture) { f.pods[0].Labels = nil }},
		{name: "foreign-controller", before: func(f *v2CollectionFixture) { f.pods[0].OwnerReferences[0].UID = "foreign" }},
		{name: "noncontroller-owner", before: func(f *v2CollectionFixture) { f.pods[0].OwnerReferences[0].Controller = ptr.To(false) }},
		{name: "terminating", before: func(f *v2CollectionFixture) { now := metav1.Now(); f.pods[0].DeletionTimestamp = &now }},
		{name: "duplicate-node", before: func(f *v2CollectionFixture) {
			p := f.pods[0].DeepCopy()
			p.Name = "duplicate"
			p.UID = "other"
			f.pods = append(f.pods, *p)
			f.ds.Status.DesiredNumberScheduled = 2
			f.ds.Status.CurrentNumberScheduled = 2
			f.ds.Status.UpdatedNumberScheduled = 2
		}},
		{name: "unbound-executable", before: func(f *v2CollectionFixture) { f.pods[0].Spec.InitContainers[0].Command = []string{"old-checker"} }},
		{name: "restartable-installer", before: func(f *v2CollectionFixture) {
			f.pods[0].Spec.InitContainers[0].RestartPolicy = ptr.To(corev1.ContainerRestartPolicyAlways)
		}},
		{name: "injected-liveness-probe", before: func(f *v2CollectionFixture) {
			f.pods[0].Spec.InitContainers[0].LivenessProbe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"false"}}}}
		}},
		{name: "reporter-shadowed-output", before: func(f *v2CollectionFixture) {
			f.pods[0].Spec.Volumes = append(f.pods[0].Spec.Volumes, corev1.Volume{Name: "stale", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "stale"}}}})
			f.pods[0].Spec.InitContainers[1].VolumeMounts = append(f.pods[0].Spec.InitContainers[1].VolumeMounts, corev1.VolumeMount{Name: "stale", MountPath: "/shared/output.json", SubPath: "report.json"})
		}},
		{name: "service-account-shadowed-output", before: func(f *v2CollectionFixture) {
			addV2FixtureServiceAccount(f)
			f.pods[0].Spec.InitContainers[1].VolumeMounts[1].MountPath = "/shared/output.json"
		}},
		{name: "failed-checker", before: func(f *v2CollectionFixture) { f.pods[0].Status.InitContainerStatuses[0].State.Terminated.ExitCode = 1 }},
		{name: "missing-container-id", before: func(f *v2CollectionFixture) {
			f.pods[0].Status.InitContainerStatuses[0].State.Terminated.ContainerID = ""
		}},
		{name: "early-reporter", before: func(f *v2CollectionFixture) {
			f.pods[0].Status.InitContainerStatuses[1].State.Terminated.StartedAt = metav1.NewTime(time.Unix(99, 0))
		}},
		{name: "list-error", before: func(f *v2CollectionFixture) { f.failList = true }},
		{name: "log-error", logError: true},
		{name: "replacement-pod", after: func(f *v2CollectionFixture) { f.pods[0].UID = k8stypes.UID("replacement") }},
		{name: "new-container-attempt", after: func(f *v2CollectionFixture) { f.pods[0].Status.InitContainerStatuses[0].RestartCount++ }},
		{name: "changed-spec", after: func(f *v2CollectionFixture) { f.ds.Spec.Template.Spec.InitContainers[0].Image = "other" }},
		{name: "inventory-disappears", after: func(f *v2CollectionFixture) { f.pods = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, client, ds := newV2CollectionFixture(t)
			if test.before != nil {
				test.before(fixture)
			}
			output, err := collectV2Reports(t.Context(), client, ds, func(context.Context, string, string) ([]byte, error) {
				if test.logError {
					return nil, fmt.Errorf("logs unavailable")
				}
				if test.after != nil {
					fixture.mu.Lock()
					test.after(fixture)
					fixture.mu.Unlock()
				}
				return []byte(`{"dataEngineMode":"v2-only","log":{"info":["checks completed"]}}`), nil
			})
			if (err == nil) != test.valid || (test.valid && !strings.Contains(output, "node-1:")) || (!test.valid && output != "") {
				t.Fatalf("incorrect execution qualification: output=%q err=%v", output, err)
			}
		})
	}
}

func addV2FixtureServiceAccount(f *v2CollectionFixture) {
	name := "kube-api-access-abcde"
	f.pods[0].Spec.Volumes = append(f.pods[0].Spec.Volumes, corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{DefaultMode: ptr.To(int32(420)), Sources: []corev1.VolumeProjection{
		{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token", ExpirationSeconds: ptr.To(int64(3607))}},
		{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"}, Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
		{DownwardAPI: &corev1.DownwardAPIProjection{Items: []corev1.DownwardAPIVolumeFile{{Path: "namespace", FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"}}}}},
	}}}})
	for index := range f.pods[0].Spec.InitContainers {
		f.pods[0].Spec.InitContainers[index].VolumeMounts = append(f.pods[0].Spec.InitContainers[index].VolumeMounts, corev1.VolumeMount{Name: name, ReadOnly: true, MountPath: "/var/run/secrets/kubernetes.io/serviceaccount"})
	}
	for index := range f.pods[0].Spec.Containers {
		f.pods[0].Spec.Containers[index].VolumeMounts = append(f.pods[0].Spec.Containers[index].VolumeMounts, corev1.VolumeMount{Name: name, ReadOnly: true, MountPath: "/var/run/secrets/kubernetes.io/serviceaccount"})
	}
}

func TestV2LogReadUsesTheFixedBoundedReporter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/api/v1/namespaces/test/pods/checker/log" || query.Get("container") != consts.ContainerNameOutput || query.Get("limitBytes") != fmt.Sprint(maxV2ReportBytes+1) || query.Get("follow") == "true" || query.Get("previous") == "true" || query.Get("timestamps") == "true" || query.Get("tailLines") != "" {
			t.Errorf("unqualified log request %s", r.URL)
		}
		_, _ = w.Write([]byte(strings.Repeat("x", maxV2ReportBytes+100)))
	}))
	t.Cleanup(server.Close)
	client, err := kubeclient.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	data, err := readV2PodReport(client)(t.Context(), "test", "checker")
	if err != nil || len(data) != maxV2ReportBytes+1 {
		t.Fatalf("log stream not bounded: length=%d err=%v", len(data), err)
	}
	if _, err := decodeV2Report(data); err == nil {
		t.Fatal("oversized log qualified")
	}
}

func TestV2CollectorReturnsFullReportWithFailedQualification(t *testing.T) {
	_, client, ds := newV2CollectionFixture(t)
	output, err := collectV2Reports(t.Context(), client, ds, func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"dataEngineMode":"v2-only","log":{"info":["checks completed"],"error":["NVMe missing"]}}`), nil
	})
	if err == nil || !strings.Contains(output, "NVMe missing") || !strings.Contains(output, "node-1:") {
		t.Fatalf("failed qualification lost its complete report: %q %v", output, err)
	}
}

func TestV2AdmissionCannotLowerRequestedChecksOrReplaceOutput(t *testing.T) {
	_, _, created := newV2CollectionFixture(t)
	request := created.DeepCopy()
	request.Spec.Template.Spec.InitContainers[1].Env = []corev1.EnvVar{}
	created.Spec.Template.Spec.Volumes[0].HostPath.Type = ptr.To(corev1.HostPathUnset)
	if err := validateV2CreatedDaemonSet(request, created, consts.SubCmdCheck); err != nil {
		t.Fatalf("native defaults were mistaken for an execution rewrite: %v", err)
	}
	for _, mutate := range []func(*appsv1.DaemonSet){
		func(ds *appsv1.DaemonSet) {
			ds.Spec.Template.Spec.InitContainers[0].RestartPolicy = ptr.To(corev1.ContainerRestartPolicyAlways)
		},
		func(ds *appsv1.DaemonSet) {
			ds.Spec.Template.Spec.InitContainers[0].StartupProbe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"false"}}}}
		},
		func(ds *appsv1.DaemonSet) {
			for index, variable := range ds.Spec.Template.Spec.InitContainers[0].Env {
				if variable.Name == consts.EnvHugePageSize {
					ds.Spec.Template.Spec.InitContainers[0].Env[index].Value = "2"
				}
			}
		},
		func(ds *appsv1.DaemonSet) {
			ds.Spec.Template.Spec.Volumes[1].VolumeSource = corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "stale-report"}}}
		},
		func(ds *appsv1.DaemonSet) { ds.Spec.Template.Spec.NodeSelector = map[string]string{"node": "other"} },
	} {
		changed := created.DeepCopy()
		mutate(changed)
		if err := validateV2CreatedDaemonSet(request, changed, consts.SubCmdCheck); err == nil {
			t.Fatal("admission rewrote the requested qualification")
		}
	}
}
