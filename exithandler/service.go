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

// NotifyServiceStopFunc is a function type that notifies about abrupt service stops
type NotifyServiceStopFunc func(reason string) error

type stopNotif struct {
	err     error
	service string
	reason  string
}

var errDone = errors.New("done")

// WrapServices notifies and shuts down the service on OS signals or err
func WrapServices(notify NotifyServiceStopFunc, services ...Service) error {
	mainCtx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	defer stop()

	// double the buffer size for post-start and post-stop notifications
	var notifyErr = make(chan stopNotif, len(services)*2)

	// blocking service run
	svcGroup := runServices(mainCtx, services, notifyErr)

	log.Tracef("waiting for service errors")
	firstErr := svcGroup.Wait()
	log.Tracef("service group stopped with first error: %s", firstErr)

	// all notifications have already been sent
	close(notifyErr)

	reason := buildShutdownMessage(notifyErr)

	log.Tracef("reason for service stop: %v", reason)

	notifiedErr := notify(reason)
	if notifiedErr != nil {
		log.Errorf("failed to notify about service stop: %v", notifiedErr)
	}

	log.Tracef("all services stopped")

	return nil
}

func runServices(mainCtx context.Context, services []Service, notifyErr chan stopNotif) *errgroup.Group {
	// a derived context that is cancelled if any function in the error group returns a non-nil error
	svcGroup, svcGroupCtx := errgroup.WithContext(mainCtx)
	for _, srv := range services {
		runService(svcGroup, svcGroupCtx, srv, notifyErr)
	}

	return svcGroup
}

func runService(svcGroup *errgroup.Group, svcGroupCtx context.Context, serv Service, notifyErr chan stopNotif) {
	// start service in a goroutine
	// and return the error to the group when any Start finishes
	svcGroup.Go(func() error {
		log.Tracef("[service] starting: %s", serv.Name())
		err := serv.Start()
		log.Tracef("[service] finished: %s", serv.Name())

		withError := "finished without error"
		if err != nil {
			withError = "finished with error"
		}
		notifyErr <- stopNotif{
			err:     err,
			service: serv.Name(),
			reason:  withError,
		}

		// service Starts "don't have to" return an error
		// either nil or non-nil, we want to stop the other services
		// we return errDone to signal the group to stop
		if err == nil {
			return errDone
		}

		return err
	})

	// stop service on group context cancellation
	// we put this in a separate goroutine to make sure
	// we don't attempt to stop a service that hasn't started or errored
	svcGroup.Go(func() error {
		// wait for group context to be cancelled
		// this will happen if any service.Start returns an error
		// or if the main context is cancelled by an OS signal
		<-svcGroupCtx.Done()
		log.Tracef("[service] stopping: %s", serv.Name())
		err := serv.Stop(svcGroupCtx)
		log.Tracef("[service] stopped: %s", serv.Name())

		withError := "stopped without error"
		if err != nil {
			withError = "stopped with error"
		}
		notifyErr <- stopNotif{
			service: serv.Name(),
			reason:  withError,
			err:     err,
		}

		return err
	})
}

func buildShutdownMessage(notifyErr chan stopNotif) string {
	var errStr string
	for nr := range notifyErr {
		if nr.err != nil {
			errStr += fmt.Sprintf("%s: %s %s\n", nr.service, nr.reason, nr.err.Error())
		} else {
			errStr += fmt.Sprintf("%s: %s\n", nr.service, nr.reason)
		}
	}
	return errStr
}
