package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"

	"github.com/superorbital/kubectl-probe/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

type Probe struct {
	client                 *kubernetes.Clientset
	config                 config.Probe
	podName, containerName string
}

func New(config config.Probe, client *kubernetes.Clientset) *Probe {
	return &Probe{
		config: config,
		client: client,
	}
}

func (p *Probe) Install(ctx context.Context) error {
	deployment, err := p.client.AppsV1().Deployments(p.config.From.Deployment.Namespace).Get(ctx, p.config.From.Deployment.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to find deployment: %w", err)
	}
	pods, err := p.client.CoreV1().Pods(p.config.From.Deployment.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) < 1 {
		return fmt.Errorf("could not find a matching pod")
	}

	pod := pods.Items[0]
	podJS, err := json.Marshal(pod)
	if err != nil {
		return fmt.Errorf("error creating JSON for pod: %w", err)
	}

	copied := pod.DeepCopy()

	p.podName = pod.Name
	p.containerName = fmt.Sprintf("probe-%s", rand.String(5))
	ec := &corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:    p.containerName,
			Image:   "alpine:latest",
			Command: []string{"nc"},
			Args:    []string{"-vz", p.config.To.Server.Host, strconv.Itoa(p.config.To.Server.Port)},
		},
	}
	copied.Spec.EphemeralContainers = append(copied.Spec.EphemeralContainers, *ec)
	debugJS, err := json.Marshal(copied)
	if err != nil {
		return fmt.Errorf("error creating JSON for debug container: %w", err)
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(podJS, debugJS, pod)
	if err != nil {
		return fmt.Errorf("error creating patch to add debug container: %w", err)
	}
	log.Printf("generated strategic merge patch for debug container: %s", patch)

	result, err := p.client.CoreV1().Pods(p.config.From.Deployment.Namespace).Patch(ctx, pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
	if err != nil {
		// The apiserver will return a 404 when the EphemeralContainers feature is disabled because the `/ephemeralcontainers` subresource
		// is missing. Unlike the 404 returned by a missing pod, the status details will be empty.
		if serr, ok := err.(*errors.StatusError); ok && serr.Status().Reason == metav1.StatusReasonNotFound && serr.ErrStatus.Details.Name == "" {
			return fmt.Errorf("ephemeral containers are disabled for this cluster (error from server: %w)", err)
		}

		// The Kind used for the /ephemeralcontainers subresource changed in 1.22. When presented with an unexpected
		// Kind the api server will respond with a not-registered error.
		if runtime.IsNotRegisteredError(err) {
			return fmt.Errorf("ephemeral containers are disabled for this cluster (error from server: %w)", err)
		}
	}
	_ = result
	log.Printf("successfully started ephemeral container")
	return nil
}

func (p *Probe) Wait(ctx context.Context) error {
	fieldSelector := fields.OneTermEqualSelector("metadata.name", p.podName).String()
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return p.client.CoreV1().Pods(p.config.From.Deployment.Namespace).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return p.client.CoreV1().Pods(p.config.From.Deployment.Namespace).Watch(ctx, options)
		},
	}
	_, err := watchtools.UntilWithSync(ctx, lw, &corev1.Pod{}, nil, func(ev watch.Event) (bool, error) {
		log.Printf("watch received event %q with object %T", ev.Type, ev.Object)
		switch ev.Type {
		case watch.Deleted:
			return false, errors.NewNotFound(schema.GroupResource{Resource: "pods"}, "")
		}

		pod, ok := ev.Object.(*corev1.Pod)
		if !ok {
			return false, fmt.Errorf("watch did not return a pod: %v", ev.Object)
		}

		s := getContainerStatusByName(pod, p.containerName)
		if s == nil {
			return false, nil
		}
		log.Printf("debug container status is %v", s)
		if s.State.Running != nil || s.State.Terminated != nil {
			return true, nil
		}
		log.Printf("container %s: %s", p.containerName, s.State.Waiting.Message)
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to watch pod: %v", err)
	}

	return nil
}

func (p *Probe) Logs(ctx context.Context) (io.ReadCloser, error) {
	return p.client.CoreV1().Pods(p.config.From.Deployment.Namespace).GetLogs(p.podName, &corev1.PodLogOptions{
		SinceSeconds: iptr(60),
		Follow:       true,
		Container:    p.containerName,
	}).Stream(ctx)
}

func getContainerStatusByName(pod *corev1.Pod, containerName string) *corev1.ContainerStatus {
	allContainerStatus := [][]corev1.ContainerStatus{pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses, pod.Status.EphemeralContainerStatuses}
	for _, statusSlice := range allContainerStatus {
		for i := range statusSlice {
			if statusSlice[i].Name == containerName {
				return &statusSlice[i]
			}
		}
	}
	return nil
}

func iptr(x int64) *int64 {
	return &x
}
