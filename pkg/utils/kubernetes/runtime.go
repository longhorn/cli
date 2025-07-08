package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/util/interrupt"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"

	"github.com/longhorn/cli/pkg/types"
)

// WaitForDaemonSetContainersReady waits for the containers in the given DaemonSet to be ready.
func WaitForDaemonSetContainersReady(ctx context.Context, logger *logrus.Entry, kubeClient *kubeclient.Clientset, daemonSet *appsv1.DaemonSet, containerName string, maxConditionToleration *int) error {
	logger.Debug("Waiting for DaemonSet container to be ready")

	conditionFunc := func(pod *corev1.Pod) bool {
		return commonkube.IsPodContainerInState(pod, containerName, commonkube.IsContainerReady)
	}
	return waitForDaemonSetContainers(ctx, logger, kubeClient, daemonSet, containerName, conditionFunc, maxConditionToleration)
}

// WaitForDaemonSetContainersExit waits for the containers in the given DaemonSet to exit.
func WaitForDaemonSetContainersExit(ctx context.Context, logger *logrus.Entry, kubeClient *kubeclient.Clientset, daemonSet *appsv1.DaemonSet, containerName string, maxConditionToleration *int) error {
	logger.Debug("Waiting for DaemonSet container to exit")

	conditionFunc := func(pod *corev1.Pod) bool {
		isInitializing := commonkube.IsPodContainerInState(pod, containerName, commonkube.IsContainerInitializing)
		if isInitializing {
			return false
		}

		isWaitingCrashloopBackoff := commonkube.IsPodContainerInState(pod, containerName, commonkube.IsContainerWaitingCrashLoopBackOff)
		isCompleted := commonkube.IsPodContainerInState(pod, containerName, commonkube.IsContainerCompleted)
		return !isWaitingCrashloopBackoff && isCompleted
	}
	return waitForDaemonSetContainers(ctx, logger, kubeClient, daemonSet, containerName, conditionFunc, maxConditionToleration)
}

func waitForDaemonSetContainers(ctx context.Context, logger *logrus.Entry, kubeClient *kubeclient.Clientset, daemonSet *appsv1.DaemonSet, containerName string, conditionFunc func(pod *corev1.Pod) bool, maxConditionToleration *int) error {
	isPodScheduled := false

	return wait.PollUntilContextCancel(ctx, time.Second, false, func(ctx context.Context) (bool, error) {
		if maxConditionToleration != nil && *maxConditionToleration < 0 {
			return true, errors.Errorf("exceeded maximum tolerated condition for DaemonSet container %s", containerName)
		}

		pods, err := kubeClient.CoreV1().Pods(daemonSet.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fields.SelectorFromSet(daemonSet.Spec.Selector.MatchLabels).String(),
		})
		if err != nil {
			logger.WithError(err).Trace("Failed to list pods")
			return false, err
		}

		if !isPodScheduled {
			logger.Trace("Waiting for DaemonSet to schedule pods")

			daemonSet, err = kubeClient.AppsV1().DaemonSets(daemonSet.Namespace).Get(ctx, daemonSet.Name, metav1.GetOptions{})
			if err != nil {
				logger.WithError(err).Trace("Failed to get DaemonSet")
				return false, err
			}

			desiredNumberScheduled := daemonSet.Status.DesiredNumberScheduled
			// Return true and consider the desired condition satisfied if there are no nodes to run DaemonSet pods; otherwise, this function will hang indefinitely.
			if desiredNumberScheduled == 0 {
				return true, nil
			}

			// Check if pod count is equal to DaemonSet pod count
			if len(pods.Items) != int(daemonSet.Status.DesiredNumberScheduled) {
				return false, nil
			}

			isPodScheduled = true
		}

		for _, pod := range pods.Items {
			logger.WithField("pod", pod.Name).Trace("Checking pod container condition")

			if commonkube.IsPodContainerInState(&pod, containerName, commonkube.IsContainerWaitingCrashLoopBackOff) {
				logger.Debug("Pod container is in crashloopbackoff")

				*maxConditionToleration = -1

				return false, errors.Errorf("pod container is in crash loop. View the logs using \"kubectl -n %s logs %s -c %s\"", pod.Namespace, pod.Name, containerName)
			}

			if !conditionFunc(&pod) {
				logger.Tracef("Waiting for pod container condition to be met, tolerating %v", *maxConditionToleration)

				if maxConditionToleration != nil {
					*maxConditionToleration--
				}

				return false, nil
			}
		}

		return true, nil
	})
}

// Workload provide functions for workloads.
type Workload struct {
	logger     *logrus.Entry
	KubeClient *kubeclient.Clientset

	Kind      string
	Namespace string
	Name      string

	// Selector is used to filter pods.
	// For example, to filter pod with label app=foo, use the selector "app=foo".
	// For multiple selectors, use "app=foo,app=bar".
	LabelSelectors string
}

// NewWorkload returns a new Workload.
func NewWorkload(kubeClient *kubeclient.Clientset, obj interface{}, kind, labelSelector string) (*Workload, error) {
	obj, ok := obj.(runtime.Object)
	if !ok {
		return nil, errors.Errorf("Failed to convert obj (%v) to runtime.Object", obj)
	}

	objMeta, err := commonkube.GetObjMetaAccesser(obj)
	if err != nil {
		return nil, err
	}

	namespace := objMeta.GetNamespace()
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}

	name := objMeta.GetName()

	logger := logrus.WithFields(logrus.Fields{
		"kind":            kind,
		"namespace":       namespace,
		"name":            name,
		"lable-selectors": labelSelector,
	})
	return &Workload{
		logger:         logger,
		KubeClient:     kubeClient,
		Kind:           kind,
		Namespace:      namespace,
		Name:           name,
		LabelSelectors: labelSelector,
	}, nil
}

func (obj *Workload) isContainersRunning(containerStatuses []corev1.ContainerStatus) bool {
	log := obj.logger.WithField("pod", obj.Name)

	for _, status := range containerStatuses {
		log = log.WithField("container", status.Name)

		if status.State.Running != nil || status.State.Terminated != nil {
			log.Debug("Confirmed pod container is running")
			return true
		}

		if status.State.Waiting != nil && status.State.Waiting.Message != "" {
			log.Tracef("Waiting for pod container to be running: %s", status.State.Waiting.Message)
		} else {
			log.Trace("Waiting for pod container to be running")
		}
	}
	return false
}

// WaitForPodContainerRunning waits for a pod container to be running.
func (obj *Workload) WaitForPodContainerRunning(ctx context.Context, containerName string) error {
	conditionFunc := func(logger logrus.Entry, pod *corev1.Pod, containerName string) bool {
		var containerStatuses []corev1.ContainerStatus

		if pod.Status.InitContainerStatuses != nil {
			for _, status := range pod.Status.InitContainerStatuses {
				if status.Name == containerName {
					containerStatuses = append(containerStatuses, status)
				}
			}
		}
		if pod.Status.ContainerStatuses != nil {
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == containerName {
					containerStatuses = append(containerStatuses, status)
				}
			}
		}
		return obj.isContainersRunning(containerStatuses)
	}
	return waitForPodsContainerCondition(ctx, *obj.logger, obj.KubeClient, obj.LabelSelectors, containerName, conditionFunc)
}

func waitForPodsContainerCondition(ctx context.Context, logger logrus.Entry, kubeClient *kubeclient.Clientset, labelSelectors, containerName string, conditionFunc func(logger logrus.Entry, pod *corev1.Pod, containerName string) bool) error {
	return wait.PollUntilContextCancel(ctx, time.Second, false, func(ctx context.Context) (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(corev1.NamespaceAll).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelectors,
		})
		if err != nil {
			return false, err
		}

		if pods == nil || len(pods.Items) == 0 {
			logger.Trace("Waiting for pods to be created")
			return false, nil
		}

		for _, pod := range pods.Items {
			logger := logger.WithFields(logrus.Fields{
				"pod":       pod.Name,
				"container": containerName,
			})

			err := waitForPodContainerCondition(logger, kubeClient, &pod, containerName, conditionFunc)
			if err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

func waitForPodContainerCondition(logger *logrus.Entry, kubeClient *kubeclient.Clientset, pod *corev1.Pod, containerName string, conditionFunc func(logger logrus.Entry, pod *corev1.Pod, containerName string) bool) error {
	ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fieldSelector := fields.OneTermEqualSelector("metadata.name", pod.Name).String()

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return kubeClient.CoreV1().Pods(pod.Namespace).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return kubeClient.CoreV1().Pods(pod.Namespace).Watch(ctx, options)
		},
	}
	intr := interrupt.New(nil, cancel)
	err := intr.Run(func() error {
		_, err := watchtools.UntilWithSync(ctx, lw, &corev1.Pod{}, nil, func(ev watch.Event) (bool, error) {
			logger.Tracef("Received event %q with object %T", ev.Type, ev.Object)

			switch ev.Type {
			case watch.Deleted:
				return false, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "")
			}

			eventPod, ok := ev.Object.(*corev1.Pod)
			if !ok {
				return false, errors.Errorf("failed to convert to *corev1.Pod: %v", ev.Object)
			}

			return conditionFunc(*logger, eventPod, containerName), nil
		})
		return err
	})
	return err
}

// GetPodsLogByContainer retrieves logs of the specified container. within the given pods.
// Optionally, it can:
// - add prefixes to the log lines
// - only retrieve logs of failed containers
// - retrieve the last N lines of the logs
func (obj *Workload) GetPodsLogByContainer(ctx context.Context, logger *logrus.Entry, containerName string, addPrefix, onlyFailed bool, tailLines *int64) (*types.PodCollections, error) {
	pods, err := obj.KubeClient.CoreV1().Pods(obj.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: obj.LabelSelectors,
	})
	if err != nil {
		return nil, err
	}

	collections := &types.PodCollections{
		Pods: make(map[string]*types.PodInfo),
	}
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			logger.Tracef("Pod %s is being deleted", pod.Name)
			continue
		}

		log := logger.WithFields(logrus.Fields{
			"pod":       pod.Name,
			"container": containerName,
		})

		containerLog, err := getPodContainerLogs(ctx, log, obj.KubeClient, &pod, containerName, addPrefix, onlyFailed, tailLines)
		if err != nil {
			log.WithError(err).Warn("Failed to get pod container log")
			continue
		}

		collections.Pods[pod.Name] = &types.PodInfo{
			Node: pod.Spec.NodeName,
			Log:  containerLog,
		}
	}
	return collections, nil
}

func getPodContainerLogs(ctx context.Context, logger *logrus.Entry, kubeClient *kubeclient.Clientset, pod *corev1.Pod, containerName string, addPrefix, onlyFailed bool, tailLines *int64) (string, error) {
	// Create a pipe for streaming logs
	reader, writer := io.Pipe()
	defer func() {
		_ = writer.Close()
	}()

	// Create a logger specific to this operation
	log := logger.WithFields(logrus.Fields{
		"pod":       pod.Name,
		"container": containerName,
	})

	// Define options for fetching pod logs
	podLogOpts := &corev1.PodLogOptions{
		Container: containerName,
		TailLines: tailLines,
	}

	// Fetch logs from Kubernetes API
	req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = podLogs.Close()
	}()

	// Start a goroutine to copy logs from the stream to the pipe
	go func() {
		defer func() {
			_ = podLogs.Close()
		}()
		defer func() {
			_ = writer.Close()
		}()

		_, err := io.Copy(writer, podLogs)
		if err != nil {
			log.WithError(err).Warn("Failed to copy logs from pod")
		}
	}()

	// Accumulate logs with optional prefix
	accumulatedLogs, err := accumulatePodContainerLogs(ctx, pod.Name, containerName, reader, addPrefix, log)
	if err != nil {
		return "", err
	}

	// Check if only logs of failed containers are required
	if onlyFailed {
		isContainerFailed := func() bool {
			isContainerReady := commonkube.IsPodContainerInState(pod, containerName, commonkube.IsContainerReady)
			isContainerCompleted := commonkube.IsPodContainerInState(pod, containerName, commonkube.IsContainerCompleted)
			isContainerWaitingCrashloopBackoff := commonkube.IsPodContainerInState(pod, containerName, commonkube.IsContainerWaitingCrashLoopBackOff)
			return (!isContainerReady && !isContainerCompleted) || isContainerWaitingCrashloopBackoff
		}()

		if !isContainerFailed {
			log.Debug("Failed to find the failed container")
			return "", nil
		}
	}

	return accumulatedLogs, nil
}

// accumulateLogs reads logs from the reader, applies prefix if required,
// and accumulates them into a single string.
func accumulatePodContainerLogs(ctx context.Context, podName, containerName string, reader io.Reader, addPrefix bool, log *logrus.Entry) (string, error) {
	// Make the reader non-blocking
	buf := bufio.NewReader(reader)
	var accumulatedLogs strings.Builder

	prefix := ""
	if addPrefix {
		prefix = fmt.Sprintf("[pod/%s/%s] ", podName, containerName)
	}

	for {
		select {
		case <-ctx.Done():
			log.Trace("Context done while reading logs from pod")
			return accumulatedLogs.String(), nil
		default:
			line, err := buf.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					log.WithError(err).Warn("Failed to read log line from pod")
				}
				return accumulatedLogs.String(), nil
			}

			if addPrefix {
				line = fmt.Sprintf("%s%s", prefix, line)
			}

			accumulatedLogs.WriteString(line)
		}
	}
}
