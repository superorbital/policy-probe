package probe

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/davecgh/go-spew/spew"
	"github.com/superorbital/kubectl-probe/pkg/api"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Factory struct {
	client *kubernetes.Clientset
	image  string
}

func (f *Factory) Create(ctx context.Context, cfg *api.ProbeSpec) (api.Source, error) {
	var containers []v1.Container
	if cfg.Source != nil {
		containers = append(containers, v1.Container{
			Name:  "source",
			Image: f.image,
			Args:  []string{"source"},
			Env: []v1.EnvVar{
				{Name: "address", Value: cfg.Source.Address},
				{Name: "port", Value: strconv.Itoa(cfg.Source.Port)},
				{Name: "protocol", Value: string(cfg.Source.Protocol)},
				{Name: "message", Value: cfg.Source.Message},
			},
		})
	}
	if cfg.Sink != nil {
		containers = append(containers, v1.Container{
			Name:  "sink",
			Image: f.image,
			Args:  []string{"destination"},
			Env: []v1.EnvVar{
				{Name: "address", Value: cfg.Sink.Address},
				{Name: "port", Value: strconv.Itoa(cfg.Sink.Port)},
				{Name: "protocol", Value: string(cfg.Sink.Protocol)},
				{Name: "message", Value: cfg.Sink.Message},
			},
		})
	}
	_, err := f.client.CoreV1().Pods(cfg.Namespace).Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "probe-",
			Namespace:    cfg.Namespace,
		},
		Spec: v1.PodSpec{
			Containers: containers,
		},
	}, metav1.CreateOptions{})
	return nil, err
}

func SourceCmd(ctx context.Context, cfg api.ProbeConfig) error {
	ln, err := net.Listen(string(cfg.Protocol), fmt.Sprintf("%s:%d", cfg.Address, cfg.Port))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	return handleRequest(conn, cfg)
}

func SinkCmd(ctx context.Context, cfg api.ProbeConfig) error {
	conn, err := net.Dial(string(cfg.Protocol), fmt.Sprintf("%s:%d", cfg.Address, cfg.Port))
	if err != nil {
		return fmt.Errorf("failed to dial address: %w", err)
	}
	defer conn.Close()
	for {
		_, err = fmt.Fprintf(conn, cfg.Message)
		if err != nil {
			return err
		}
	}
}

// Handles incoming requests.
func handleRequest(conn net.Conn, cfg api.ProbeConfig) error {
	log.Println("Accepted new connection")
	defer conn.Close()
	defer log.Println("Closed connection")

	for {
		buf := make([]byte, 1024)
		size, err := conn.Read(buf)
		if err != nil {
			return err
		}
		data := buf[:size]
		if string(data) != cfg.Message {
			log.Printf("got %q", string(data))
			return fmt.Errorf("received unexpected message\n%v", spew.Sdump(data))
		} else {
			log.Println("Received expected message")
			return nil
		}
	}
}
