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
	client *kubernetes.Clientset
	config config.Probe
}

type InstalledProbe struct {
	client *kubernetes.Clientset
	podName,
	containerName,
	namespace string
}

func New(config config.Probe, client *kubernetes.Clientset) *Probe {
	return &Probe{
		config: config,
		client: client,
	}
}

func (p *Probe) Install(ctx context.Context) (*InstalledProbe, error) {
	switch {
	case p.config.From.Deployment != nil:
		// install ephemeral pod
		podRef, err := p.getPod(ctx, p.config.From.Deployment)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch deployment: %w", err)
		}
		install, err := p.installEphemeralPod(ctx, podRef)
		if err != nil {
			return nil, fmt.Errorf("failed to install ephemeral pod: %w", err)
		}
		return install, nil
	case p.config.From.Pod != nil:
		install, err := p.installEphemeralPod(ctx, p.config.From.Pod)
		if err != nil {
			return nil, fmt.Errorf("failed to install ephemeral pod: %w", err)
		}
		return install, nil
	case p.config.From.Probe != nil:
		// create probe pod
		panic("todo")
	default:
		panic("wtf")
	}
}

func (p *InstalledProbe) Wait(ctx context.Context) error {
	fieldSelector := fields.OneTermEqualSelector("metadata.name", p.podName).String()
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return p.client.CoreV1().Pods(p.namespace).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return p.client.CoreV1().Pods(p.namespace).Watch(ctx, options)
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

func (p *InstalledProbe) Logs(ctx context.Context) (io.ReadCloser, error) {
	return p.client.CoreV1().Pods(p.namespace).GetLogs(p.podName, &corev1.PodLogOptions{
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

func (p *Probe) getPod(ctx context.Context, d *config.ObjectSpec) (*config.ObjectSpec, error) {
	switch {
	case d.LabelSelector != nil:
		deployments, err := p.client.AppsV1().Deployments(d.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(d.LabelSelector),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments: %w", err)
		}
		if len(deployments.Items) < 1 {
			return nil, fmt.Errorf("no deployments found for selector")
		}
		// TODO: test all matching deployments?
		return &config.ObjectSpec{
			Namespace:     d.Namespace,
			LabelSelector: deployments.Items[0].Spec.Selector,
		}, nil
	case d.Name != nil:
		deployment, err := p.client.AppsV1().Deployments(d.Namespace).Get(ctx, *d.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &config.ObjectSpec{
			Namespace:     d.Namespace,
			LabelSelector: deployment.Spec.Selector,
		}, nil
	default:
		return nil, fmt.Errorf("either labelSelector or name must be specified")
	}
}

func (p *Probe) installEphemeralPod(ctx context.Context, d *config.ObjectSpec) (*InstalledProbe, error) {
	var pod *corev1.Pod
	switch {
	case d.LabelSelector != nil:
		pods, err := p.client.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(d.LabelSelector),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list pods: %w", err)
		}
		if len(pods.Items) < 1 {
			return nil, fmt.Errorf("could not find a matching pod")
		}
		// TODO: test all matching pods?
		pod = &pods.Items[0]
	case d.Name != nil:
		var err error
		pod, err = p.client.CoreV1().Pods(d.Namespace).Get(ctx, *d.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get pod: %w", err)
		}
	default:
		return nil, fmt.Errorf("either labelSelector or name must be specified")
	}

	podJS, err := json.Marshal(pod)
	if err != nil {
		return nil, fmt.Errorf("error creating JSON for pod: %w", err)
	}

	copied := pod.DeepCopy()

	ec := &corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:    fmt.Sprintf("probe-%s", rand.String(5)),
			Image:   "alpine:latest",
			Command: []string{"nc"},
			Args:    []string{"-vz", p.config.To.Server.Host, strconv.Itoa(p.config.To.Server.Port)},
		},
	}
	copied.Spec.EphemeralContainers = append(copied.Spec.EphemeralContainers, *ec)
	debugJS, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("error creating JSON for debug container: %w", err)
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(podJS, debugJS, pod)
	if err != nil {
		return nil, fmt.Errorf("error creating patch to add debug container: %w", err)
	}
	log.Printf("generated strategic merge patch for debug container: %s", patch)

	_, err = p.client.CoreV1().Pods(d.Namespace).Patch(ctx, pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
	if err != nil {
		return nil, fmt.Errorf("error adding ephemeral container to pod: %w", err)
	}

	return &InstalledProbe{
		client:        p.client,
		podName:       pod.Name,
		containerName: ec.Name,
		namespace:     pod.Namespace,
	}, nil
}
