package exithandler

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Service represents a stoppable service
type Service interface {
	Name() string
	Start() error
	Stop(ctx context.Context) error
}

// WrapServices notifies and shuts down the service on OS signals or err
func WrapServices(services ...Service) error {
	mainCtx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGKILL,
	)
	defer stop()
	g, gCtx := errgroup.WithContext(mainCtx)
	for _, srv := range services {
		func(serv Service) {
			g.Go(func() error {
				log.Tracef("starting service: %s", serv.Name())
				return serv.Start()
			})
			g.Go(func() error {
				<-gCtx.Done()
				log.Tracef("stopping service: %s", serv.Name())
				return serv.Stop(gCtx)
			})
		}(srv)
	}
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed to shutdown: %w", err)
	}
	log.Tracef("all services stopped")
	return nil
}

// StartFunc is a function type that matches the signature of the Start method.
type StartFunc func() error

// ShutdownFunc is a function type that matches the signature of the Shutdown method.
type ShutdownFunc func(context.Context) error

// GenericService is a wrapper that allows any function matching the StartFunc signature
// to be used as a Service.
type GenericService struct {
	name         string
	startFunc    StartFunc
	shutdownFunc ShutdownFunc
}

// NewService returns a new GenericService.
func NewService(name string, start StartFunc, shutdown ShutdownFunc) *GenericService {
	return &GenericService{
		name:         name,
		startFunc:    start,
		shutdownFunc: shutdown,
	}
}

var _ Service = (*GenericService)(nil)

// Name returns the stored name.
func (gs *GenericService) Name() string {
	return gs.name
}

// Start calls the stored StartFunc.
func (gs *GenericService) Start() error {
	return gs.startFunc()
}

// Stop calls the stored ShutdownFunc.
func (gs *GenericService) Stop(ctx context.Context) error {
	return gs.shutdownFunc(ctx)
}
