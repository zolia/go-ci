package exithandler

import "context"

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
