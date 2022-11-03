package probe

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/davecgh/go-spew/spew"
	"github.com/superorbital/kubectl-probe/pkg/api"
)

type Factory struct{}

func (f *Factory) CreateSource(cfg *api.SourceConfig) (api.Source, error) {
	panic("not implemented")
}

func (f *Factory) CreateDestination(cfg *api.DestinationConfig) (api.Destination, error) {
	panic("not implemented")
}

func Source(cfg *api.FromKinds) *api.SourceConfig {
	panic("not implemented")
}

func Destination(source *api.SourceConfig, cfg *api.ToKinds) *api.DestinationConfig {
	panic("not implemented")
}

func SourceCmd(ctx context.Context, cfg api.SourceConfig) error {
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

func DestinationCmd(ctx context.Context, cfg api.DestinationConfig) error {
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
func handleRequest(conn net.Conn, cfg api.SourceConfig) error {
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
