package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/superorbital/kubectl-probe/pkg/api"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type Factory struct {
	client     *kubernetes.Clientset
	probeImage string
}

func NewFactory(client *kubernetes.Clientset, probeImage string) *Factory {
	return &Factory{
		client:     client,
		probeImage: probeImage,
	}
}

func (f *Factory) Create(ctx context.Context, cfg *api.TestCase) (api.Source, error) {
	podRef, err := f.getPod(ctx, cfg.From.Deployment)
	if err != nil {
		return nil, err
	}
	return f.installEphemeralPod(ctx, podRef, &cfg.To)
}

func (f *Factory) getPod(ctx context.Context, d *api.ObjectSpec) (*api.ObjectSpec, error) {
	switch {
	case d.LabelSelector != nil:
		deployments, err := f.client.AppsV1().Deployments(d.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(d.LabelSelector),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments: %w", err)
		}
		if len(deployments.Items) < 1 {
			return nil, fmt.Errorf("no deployments found for selector")
		}
		// TODO: test all matching deployments?
		return &api.ObjectSpec{
			Namespace:     d.Namespace,
			LabelSelector: deployments.Items[0].Spec.Selector,
		}, nil
	case d.Name != nil:
		deployment, err := f.client.AppsV1().Deployments(d.Namespace).Get(ctx, *d.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("unable to get deployment: %w", err)
		}
		return &api.ObjectSpec{
			Namespace:     d.Namespace,
			LabelSelector: deployment.Spec.Selector,
		}, nil
	default:
		return nil, fmt.Errorf("either labelSelector or name must be specified")
	}
}

type Probe struct {
	pod           *v1.Pod
	client        *kubernetes.Clientset
	containerName string
}

type message struct {
	Fail    int
	Success int
}

func (p *Probe) AssertReachable(ctx context.Context) error {
	return p.assert(ctx, func(old, new message) (bool, error) {
		if new.Fail > old.Fail {
			return false, fmt.Errorf("probe was not able to reach destination")
		}
		if new.Success > old.Success {
			return true, nil
		}
		return false, nil
	})
}

func (p *Probe) AssertUnreachable(ctx context.Context) error {
	return p.assert(ctx, func(old, new message) (bool, error) {
		if new.Success > old.Success {
			return false, fmt.Errorf("probe was able to reach destination")
		}
		if new.Fail > old.Fail {
			return true, nil
		}
		return false, nil
	})
}

func (p *Probe) assert(ctx context.Context, test func(message, message) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	err := wait.PollImmediateUntil(time.Second, func() (done bool, err error) {
		pod, err := p.client.CoreV1().Pods(p.pod.Namespace).Get(ctx, p.pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, c := range pod.Status.EphemeralContainerStatuses {
			if c.Name == p.containerName {
				done = c.State.Running != nil
			}
		}
		return
	}, ctx.Done())
	if err != nil {
		return err
	}
	stream, err := p.client.CoreV1().Pods(p.pod.Namespace).GetLogs(p.pod.Name, &v1.PodLogOptions{
		SinceSeconds: iptr(60),
		Follow:       true,
		Container:    p.containerName,
	}).Stream(ctx)
	if err != nil {
		return err
	}
	var prior message
	return wait.PollImmediateUntil(time.Second, func() (done bool, err error) {
		var msg message
		if err = json.NewDecoder(stream).Decode(&msg); err != nil {
			return false, fmt.Errorf("failed to decode stream: %w", err)
		}
		zap.L().Debug("received probe message", zap.Any("message", msg))
		return test(prior, msg)
	}, ctx.Done())
}

func (f *Factory) installEphemeralPod(ctx context.Context, d *api.ObjectSpec, to *api.ProbeConfig) (*Probe, error) {
	var pod *v1.Pod
	switch {
	case d.LabelSelector != nil:
		pods, err := f.client.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(d.LabelSelector),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list pods: %w", err)
		}
		if len(pods.Items) < 1 {
			return nil, fmt.Errorf("could not find a matching pod")
		}
		// TODO: find if any pods already have the probe
		pod = &pods.Items[0]
	case d.Name != nil:
		var err error
		pod, err = f.client.CoreV1().Pods(d.Namespace).Get(ctx, *d.Name, metav1.GetOptions{})
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

	args := []string{"probe"}
	if zap.L().Core().Enabled(zap.DebugLevel) {
		args = append(args, "--debug")
	}
	ec := &v1.EphemeralContainer{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:  fmt.Sprintf("probe-%s", rand.String(5)),
			Image: f.probeImage, // TODO: configurable
			Args:  args,
			Env:   to.ToEnv(),
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

	pod, err = f.client.CoreV1().Pods(d.Namespace).Patch(ctx, pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
	if err != nil {
		return nil, fmt.Errorf("error adding ephemeral container to pod: %w", err)
	}
	probe := &Probe{
		pod:           pod,
		client:        f.client,
		containerName: ec.Name,
	}
	zap.L().Debug("created probe", zap.String("probe", probe.containerName), zap.String("pod", pod.Name), zap.String("namespace", pod.Namespace))

	return probe, nil
}

func Run(ctx context.Context, cfg api.ProbeConfig) {
	fail := metrics.GetOrRegisterCounter("fail", metrics.DefaultRegistry)
	success := metrics.GetOrRegisterCounter("success", metrics.DefaultRegistry)
	tick := time.NewTicker(cfg.Interval)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			ping(cfg, success, fail)
		case <-ctx.Done():
			return
		}
	}
}

func ping(cfg api.ProbeConfig, success, fail metrics.Counter) {
	conn, err := net.DialTimeout(cfg.Protocol.String(), cfg.Address, 10*time.Second)
	if err != nil {
		zap.L().Debug("dial failed", zap.Error(err))
		fail.Inc(1)
		return
	}
	defer conn.Close()
	_, err = fmt.Fprintf(conn, cfg.Message)
	if err != nil {
		zap.L().Debug("write failed", zap.Error(err))
		fail.Inc(1)
		return
	}
	zap.L().Debug("probe succeeded", zap.Error(err))
	success.Inc(1)
}

func iptr(x int64) *int64 {
	return &x
}
