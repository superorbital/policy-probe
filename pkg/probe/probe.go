package probe

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/superorbital/kubectl-probe/pkg/api"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Factory struct {
	client *kubernetes.Clientset
	image  string
}

func (f *Factory) Create(ctx context.Context, cfg *api.TestCase) (api.Source, error) {
	var containers []v1.Container
	source := cfg.From.Probe.Source
	if source != nil {
		containers = append(containers, v1.Container{
			Name:  "source",
			Image: f.image,
			Args:  []string{"source"},
			Env: []v1.EnvVar{
				{Name: "address", Value: source.Address},
				{Name: "port", Value: strconv.Itoa(source.Port)},
				{Name: "protocol", Value: string(source.Protocol)},
				{Name: "message", Value: source.Message},
			},
		})
	}
	_, err := f.client.CoreV1().Pods(cfg.From.Probe.Namespace).Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "probe-",
			Namespace:    cfg.From.Probe.Namespace,
		},
		Spec: v1.PodSpec{
			Containers: containers,
		},
	}, metav1.CreateOptions{})
	return nil, err
}

func Run(ctx context.Context, cfg api.ProbeConfig) {
	fail := metrics.GetOrRegisterCounter("fail", metrics.DefaultRegistry)
	success := metrics.GetOrRegisterCounter("success", metrics.DefaultRegistry)

	for {
		err := func() error {
			conn, err := net.Dial(string(cfg.Protocol), fmt.Sprintf("%s:%d", cfg.Address, cfg.Port))
			if err != nil {
				return err
			}
			defer conn.Close()
			_, err = fmt.Fprintf(conn, cfg.Message)
			if err != nil {
				return err
			}
			success.Inc(1)
			return nil
		}()
		if err != nil {
			fail.Inc(1)
		}
		time.Sleep(cfg.Interval)
	}
}
