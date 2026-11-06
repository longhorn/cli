package preflight

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"k8s.io/client-go/rest"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	kubeclient "k8s.io/client-go/kubernetes"
)

func TestV2RBACNeverAdoptsOrDeletesUnownedObjects(t *testing.T) {
	for _, test := range []struct {
		name, failResource string
		replacement        bool
		created            int
	}{
		{"complete", "", false, 3},
		{"account-exists", "serviceaccounts", false, 0},
		{"role-exists", "clusterroles", false, 1},
		{"binding-exists", "clusterrolebindings", false, 2},
		{"replacement-conflict", "", true, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			deletes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				parts := strings.Split(r.URL.Path, "/")
				resource := parts[len(parts)-1]
				if r.Method == http.MethodDelete {
					resource = parts[len(parts)-2]
				}
				if r.Method == http.MethodPost && resource == test.failResource {
					w.WriteHeader(http.StatusConflict)
					_ = json.NewEncoder(w).Encode(&metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: metav1.StatusReasonAlreadyExists, Code: 409})
					return
				}
				if r.Method == http.MethodDelete {
					deletes++
					var options metav1.DeleteOptions
					if err := json.NewDecoder(r.Body).Decode(&options); err != nil || options.Preconditions == nil || options.Preconditions.UID == nil || string(*options.Preconditions.UID) != "owned-"+resource {
						t.Errorf("cleanup missing created UID: %+v %v", options, err)
					}
					if test.replacement && resource == "clusterroles" {
						w.WriteHeader(http.StatusConflict)
						_ = json.NewEncoder(w).Encode(&metav1.Status{Status: "Failure", Reason: metav1.StatusReasonConflict, Code: 409})
						return
					}
					_ = json.NewEncoder(w).Encode(&metav1.Status{Status: "Success"})
					return
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected ownership API method: %s", r.Method)
					http.Error(w, "unexpected", 500)
					return
				}
				var object map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&object); err != nil {
					t.Error(err)
					return
				}
				metadata := object["metadata"].(map[string]interface{})
				metadata["uid"] = "owned-" + resource
				_ = json.NewEncoder(w).Encode(object)
			}))
			t.Cleanup(server.Close)
			client, err := kubeclient.NewForConfig(&rest.Config{Host: server.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json", AcceptContentTypes: "application/json"}})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := createV2RBAC(t.Context(), client, "test", "owned", nil)
			if (err != nil) != (test.failResource != "") {
				t.Fatalf("strict creation adopted existing resources: %v", err)
			}
			err = cleanupV2RBAC(t.Context(), client, receipt)
			if (err != nil) != test.replacement {
				t.Fatalf("cleanup hid replacement conflict: %v", err)
			}
			if deletes != test.created {
				t.Fatalf("deleted objects without successful creation receipts: got=%d want=%d", deletes, test.created)
			}
		})
	}
}

func TestV2CleanupStillRemovesOwnedRBACAfterDaemonSetFailure(t *testing.T) {
	for _, installer := range []bool{false, true} {
		for _, missingUID := range []bool{false, true} {
			t.Run(fmt.Sprintf("installer=%v/missingUID=%v", installer, missingUID), func(t *testing.T) {
				var mu sync.Mutex
				deletes := map[string]int{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					parts := strings.Split(r.URL.Path, "/")
					resource := parts[len(parts)-2]
					deletes[resource]++
					var options metav1.DeleteOptions
					if r.Method != http.MethodDelete || json.NewDecoder(r.Body).Decode(&options) != nil || options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID == "" {
						t.Error("cleanup attempted an unidentified deletion")
					}
					if resource == "daemonsets" {
						w.WriteHeader(http.StatusConflict)
						_ = json.NewEncoder(w).Encode(&metav1.Status{Status: "Failure", Reason: metav1.StatusReasonConflict, Code: 409})
						return
					}
					_ = json.NewEncoder(w).Encode(&metav1.Status{Status: "Success"})
				}))
				t.Cleanup(server.Close)
				client, err := kubeclient.NewForConfig(&rest.Config{Host: server.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json", AcceptContentTypes: "application/json"}})
				if err != nil {
					t.Fatal(err)
				}
				uid := k8stypes.UID("owned-ds")
				if missingUID {
					uid = ""
				}
				ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "test", UID: uid}}
				receipt := &v2RBACReceipt{Name: "owned", Namespace: "test", ServiceAccount: "sa-uid", ClusterRole: "role-uid", ClusterRoleBinding: "binding-uid"}
				if installer {
					checker := Installer{InstallerCmdOptions: InstallerCmdOptions{DataEngineMode: "v2-only"}, kubeClient: client, createdDaemonSet: ds, createdRBAC: receipt}
					err = checker.Cleanup()
				} else {
					checker := Checker{CheckerCmdOptions: CheckerCmdOptions{DataEngineMode: "v2-only"}, kubeClient: client, createdDaemonSet: ds, createdRBAC: receipt}
					err = checker.Cleanup()
				}
				if err == nil || deletes["serviceaccounts"] != 1 || deletes["clusterroles"] != 1 || deletes["clusterrolebindings"] != 1 {
					t.Fatalf("DaemonSet failure leaked owned RBAC: deleted=%v err=%v", deletes, err)
				}
				if missingUID && deletes["daemonsets"] != 0 {
					t.Fatal("deleted DaemonSet without UID")
				}
			})
		}
	}
}
