package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/longhorn/cli/pkg/types"
)

type monitorDaemonSetContainerConditionFunc func(ctx context.Context, logger *logrus.Entry, kubeClient *kubeclient.Clientset, daemonSet *appsv1.DaemonSet, containerName string, maxConditionToleration *int) error

// MonitorDaemonSetContainer monitors the specified container within the given DaemonSet until a certain condition is met.
// The condition is defined by the monitorDaemonSetContainerConditionFunc.
// Returns nil on success, or an error if the condition check fails or the timeout is reached.
func MonitorDaemonSetContainer(kubeClient *kubeclient.Clientset, daemonSet *appsv1.DaemonSet, containerName string, conditionFunc monitorDaemonSetContainerConditionFunc, maxConditionToleration *int) error {
	selector := fmt.Sprintf("app=%s", daemonSet.Labels["app"])
	workload, err := NewWorkload(kubeClient, daemonSet, "DaemonSet", selector)
	if err != nil {
		return err
	}

	log := logrus.WithFields(logrus.Fields{
		"kind":      "DaemonSet",
		"namespace": daemonSet.Namespace,
		"name":      daemonSet.Name,
		"container": containerName,
	})

	ctx, cancel := context.WithTimeoutCause(context.Background(), time.Hour, errors.Errorf("timed out waiting for the DaemonSet %s container %s condition", daemonSet.Name, containerName))
	defer cancel()

	doneCh := make(chan struct{})
	errCh := make(chan error, 1)
	defer close(errCh)

	go func() {
		defer close(doneCh)

		err = conditionFunc(ctx, log, kubeClient, daemonSet, containerName, maxConditionToleration)
		if err != nil {
			errCh <- errors.Wrap(err, "failed DaemonSet condition check")
		}
	}()

	// Check if any errors occurred in goroutines
	select {
	case err := <-errCh:
		log.Debug("Getting DaemonSet pods container logs")
		podsLog, _err := workload.GetPodsLogByContainer(ctx, log, containerName, true, true, nil)
		if _err != nil {
			return errors.Wrap(_err, "failed to get DaemonSet pods container logs")
		}

		if podsLog == nil {
			return nil
		}

		for podName, collection := range podsLog.Pods {
			log := log.WithFields(logrus.Fields{
				"pod":  podName,
				"node": collection.Node,
			})
			log.Trace("Beginning of pod logs >>>>>")
			log.Debug(collection.Log)
			log.Trace("<<<<< End of pod logs")
		}
		return err
	case <-doneCh:
		return nil
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "Timed out waiting for container %s to be running", containerName)
	}
}

// GetDaemonSetPodCollections retrieves the logs of the specified container within the given DaemonSet.
// Optionally, it can:
// - add prefixes to the log lines
// - only retrieve logs of failed containers
// - retrieve the last N lines of the logs
func GetDaemonSetPodCollections(kubeClient *kubeclient.Clientset, daemonSet *appsv1.DaemonSet, containerName string, addPrefix, onlyFailed bool, tailLines *int64) (*types.PodCollections, error) {
	selector := fmt.Sprintf("app=%s", daemonSet.Labels["app"])
	workload, err := NewWorkload(kubeClient, daemonSet, "DaemonSet", selector)
	if err != nil {
		return nil, err
	}

	log := logrus.WithFields(logrus.Fields{
		"kind":      "DaemonSet",
		"namespace": daemonSet.Namespace,
		"name":      daemonSet.Name,
		"container": containerName,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Debug("Getting DaemonSet pods container logs")
	collections, err := workload.GetPodsLogByContainer(ctx, log, containerName, addPrefix, onlyFailed, tailLines)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get DaemonSet pods container logs")
	}

	return collections, nil
}

// ParseNodeSelector parses a node selector string (e.g., "key1=value1,key2=value2")
// and returns it as a map[string]string. This map can be used directly in a DaemonSet spec.
// It returns an error if the input is malformed (e.g., missing '=' or empty key/value).
func ParseNodeSelector(nodeSelectorRaw string) (map[string]string, error) {
	if strings.TrimSpace(nodeSelectorRaw) == "" {
		return nil, nil
	}

	nodeSelector := make(map[string]string)
	pairs := strings.Split(nodeSelectorRaw, ",")

	for _, pair := range pairs {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid key-value pair: %q (expected format key=value)", pair)
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		if key == "" || value == "" {
			return nil, fmt.Errorf("key and value must be non-empty in pair: %q", pair)
		}

		nodeSelector[key] = value
	}

	return nodeSelector, nil
}

func GetImagePullSecrets(imagePullSecret string) []corev1.LocalObjectReference {
	if imagePullSecret != "" {
		return []corev1.LocalObjectReference{{Name: imagePullSecret}}
	}
	return nil
}
