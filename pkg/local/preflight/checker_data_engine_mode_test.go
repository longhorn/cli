package preflight

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/local/preflight/packagemanager/mocks"
	"github.com/longhorn/cli/pkg/types"

	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
	remote "github.com/longhorn/cli/pkg/remote/preflight"
)

type fixtureNFSFacts struct{ reads int }

func (facts *fixtureNFSFacts) KernelVersion() (string, error) {
	facts.reads++
	return "fixture", nil
}
func (facts *fixtureNFSFacts) BootKernelConfig(string, string) (map[string]string, error) {
	facts.reads++
	return map[string]string{"CONFIG_NFS_V4": "y"}, nil
}
func (*fixtureNFSFacts) ProcKernelConfig(string) (map[string]string, error) {
	return nil, nil
}
func (*fixtureNFSFacts) ModuleLoaded(string) (bool, error) { return true, nil }
func (facts *fixtureNFSFacts) DefaultNFSVersion(string) (int, int, error) {
	facts.reads++
	return 4, 1, nil
}

func TestV2OnlyCheckerForcesSPDKWithoutAnyISCSIProbe(t *testing.T) {
	for _, missingNVMe := range []bool{false, true} {
		for _, collectionOnly := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{false: "healthy", true: "missing-nvme"}[missingNVMe], map[bool]string{false: "direct", true: "collection"}[collectionOnly]}, "/"), func(t *testing.T) {
				t.Setenv(consts.EnvCurrentNodeID, "fixture-node")
				manager := new(mocks.MockPackageManager)
				manager.On("GetServiceStatus", "multipathd.service").Return("", nil).Once()
				for _, name := range []string{"nfs-common", "cryptsetup", "dmsetup"} {
					manager.On("CheckPackageInstalled", name).Return("", nil).Once()
				}
				for _, name := range []string{"nfs", "dm_crypt", "nvme_tcp", "uio_pci_generic", "vfio_pci", "ublk_drv"} {
					var moduleError error
					if missingNVMe && name == "nvme_tcp" {
						moduleError = pkgmgr.ErrPackageNotInstalled
					}
					manager.On("CheckModLoaded", name).Return(moduleError).Once()
				}
				manager.On("Modprobe", "ublk_drv", []string{"--dry-run"}).Return("", nil).Once()
				manager.On("Execute", []string{}, "cat", []string{"/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages"}, v2SPDKReadTimeout).Return("1024\n", nil).Once()
				manager.On("Execute", []string{}, "sh", mock.Anything, mock.Anything).Return("/sys/kernel/iommu_groups/0\n", nil).Once()
				instruction := "sse4_2"
				if runtime.GOARCH == "arm64" {
					instruction = "asimd"
				}
				manager.On("Execute", []string{}, "grep", mock.MatchedBy(func(args []string) bool {
					return strings.Contains(strings.Join(args, " "), instruction)
				}), mock.Anything).Return("", nil).Once()
				facts := &fixtureNFSFacts{}
				server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
					response.Header().Set("Content-Type", "application/json")
					var object interface{}
					switch request.URL.Path {
					case "/api/v1/nodes/fixture-node":
						object = &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "fixture-node"}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{corev1.ResourceName("hugepages-2Mi"): resource.MustParse("2Gi")}}}
					case "/apis/apps/v1/namespaces/kube-system/deployments":
						object = &appsv1.DeploymentList{Items: []appsv1.Deployment{{ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: "kube-system", Labels: map[string]string{"k8s-app": "kube-dns"}}, Spec: appsv1.DeploymentSpec{Replicas: ptr.To(int32(2))}, Status: appsv1.DeploymentStatus{ReadyReplicas: 2}}}}
					default:
						t.Errorf("unexpected API read %s", request.URL.Path)
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
				checker := &Checker{CheckerCmdOptions: remote.CheckerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048}, CollectionOnly: collectionOnly,
					logger: logrus.NewEntry(logrus.New()), osRelease: "ubuntu", packageManager: manager, nfsFacts: facts,
					collection: types.NodeCollection{Log: &types.LogCollection{}}, OutputFilePath: filepath.Join(t.TempDir(), "report.json"),
					kubeClient: client}
				if err := checker.configureDependencies(pkgmgr.PackageManagerApt); err != nil {
					t.Fatal(err)
				}
				err = checker.Run()
				if (err != nil) != (missingNVMe && !collectionOnly) {
					t.Fatalf("qualification/transport verdict changed: %v", err)
				}
				if err := checker.Output(); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(checker.OutputFilePath)
				if err != nil {
					t.Fatal(err)
				}
				var report map[string]json.RawMessage
				if err := json.Unmarshal(data, &report); err != nil || string(report["dataEngineMode"]) != `"v2-only"` {
					t.Fatal("persisted report is not bound to V2-only checks")
				}
				if (len(checker.collection.Log.Error) != 0) != missingNVMe || facts.reads != 3 {
					t.Fatal("mandatory NVMe or native NFS checks were skipped")
				}
				if !strings.Contains(strings.Join(checker.collection.Log.Info, " "), "cryptsetup") || !strings.Contains(strings.Join(checker.collection.Log.Warn, " "), "multipathd") {
					t.Fatal("encryption or multipath checks disappeared")
				}
				manager.AssertExpectations(t)
			})
		}
	}
}

func TestV2OnlyCheckerDependencyProfilesRemoveOnlyISCSI(t *testing.T) {
	for _, profile := range []pkgmgr.PackageManagerType{pkgmgr.PackageManagerApt, pkgmgr.PackageManagerYum, pkgmgr.PackageManagerZypper, pkgmgr.PackageManagerTransactionalUpdate, pkgmgr.PackageManagerBinary, pkgmgr.PackageManagerPacman} {
		checker := &Checker{CheckerCmdOptions: remote.CheckerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048}}
		if err := checker.configureDependencies(profile); err != nil {
			t.Fatal(err)
		}
		for _, name := range append(append([]string{}, checker.packages...), checker.modules...) {
			if name == "open-iscsi" || name == "iscsi-initiator-utils" || name == "iscsi_tcp" {
				t.Fatalf("profile %s retains iSCSI dependency %s", profile, name)
			}
		}
		if len(checker.packages) != 3 || len(checker.modules) != 2 || len(checker.services) != 1 || len(checker.spdkDepModules) != 3 {
			t.Fatalf("profile %s removed dependencies beyond iSCSI", profile)
		}
	}
}

func TestV2HugepagesDoNotTrustKernelDefaultSizeOrCachedCapacity(t *testing.T) {
	manager := new(mocks.MockPackageManager)
	manager.On("Execute", []string{}, "grep", []string{"HugePages_Total", "/proc/meminfo"}, mock.Anything).Return("HugePages_Total: 1024\n", nil).Maybe()
	manager.On("Execute", []string{}, "cat", []string{"/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages"}, mock.Anything).Return("0\n", nil).Maybe()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/nodes/fixture-node" {
			t.Errorf("unexpected read %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "fixture-node"}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{corev1.ResourceName("hugepages-2Mi"): resource.MustParse("2Gi")}}})
	}))
	t.Cleanup(server.Close)
	t.Setenv(consts.EnvCurrentNodeID, "fixture-node")
	client, err := kubeclient.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	checker := Checker{CheckerCmdOptions: remote.CheckerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048}, packageManager: manager, kubeClient: client, collection: types.NodeCollection{Log: &types.LogCollection{}}}
	if err := checker.checkHugePages(); err != nil {
		t.Fatal(err)
	}
	if len(checker.collection.Log.Error) == 0 {
		t.Fatal("kernel default-size pages and stale Node capacity qualified missing actual 2MiB pages")
	}
}
