// Package daemon provides a convenient way to run a blocking service.
package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run runs svc and awaits for signals to stop.
//
// If graceful shutdown fails, and svc supports force-closing, it will be closed.
func Run(ctx context.Context, svc service) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	rctx, rcancel := context.WithCancel(ctx)

	defer func() {
		signal.Stop(sigs)
		rcancel()

		close(sigs)
	}()

	errs := make(chan error, 1)

	go runSvc(rctx, errs, svc)

	defer func() { close(errs) }()

	select {
	case err := <-errs:
		rcancel()
		return err

	case <-sigs:
		rcancel()

		sctx, cancel := context.WithTimeout(ctx, svc.Timeout())
		defer cancel()

		if err := svc.Shutdown(sctx); err != nil {
			if cl, ok := svc.(closer); ok {
				return cl.Close()
			}

			return err
		}
	}

	return nil
}

type service interface {
	runner
	Shutdown(ctx context.Context) error
	Timeout() time.Duration
}

type runner interface {
	Run(ctx context.Context) error
}

type closer interface {
	Close() error
}

// Service provides a way to wrap a blocking operation and run it with Daemon.
//
// Once initialised, an instance of Service must not be changed.
//
// RunFn is assumed to be blocking.
// ShutFn is expected to gracefully shutdown what RunFn started.
type Service struct {
	ShutTimeout time.Duration
	RunFn       func(ctx context.Context) error
	ShutFn      func(ctx context.Context) error
	CloseFn     func() error
}

func (s *Service) Run(ctx context.Context) error {
	if s.RunFn == nil {
		return nil
	}

	return s.RunFn(ctx)
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s.ShutFn == nil {
		return nil
	}

	return s.ShutFn(ctx)
}

func (s *Service) Timeout() time.Duration {
	return s.ShutTimeout
}

func (s *Service) Close() error {
	if s.CloseFn == nil {
		return nil
	}

	return s.CloseFn()
}

func runSvc(ctx context.Context, errChan chan<- error, svc runner) {
	errChan <- svc.Run(ctx)
}
