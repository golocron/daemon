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

	return runWithSigCn(ctx, svc, sigs)
}

func runWithSigCn(ctx context.Context, svc service, sigs chan os.Signal) error {
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	rctx, rcancel := context.WithCancel(ctx)

	defer func() {
		signal.Stop(sigs)
		rcancel()

		close(sigs)
	}()

	errs := make(chan error, 1)

	go func() {
		runSvc(rctx, errs, svc)
		close(errs)
	}()

	select {
	case err := <-errs:
		rcancel()
		return err

	case <-sigs:
		rcancel()

		<-errs

		sctx, cancel := context.WithTimeout(ctx, svc.Timeout())
		defer cancel()

		serrs := make(chan error, 1)
		go func() {
			serrs <- svc.Shutdown(sctx)
			close(serrs)
		}()

		select {
		case <-sctx.Done():
			return sctx.Err()

		case serr := <-serrs:
			if serr != nil {
				if cl, ok := svc.(closer); ok {
					return cl.Close()
				}
			}

			return serr
		}
	}

	// Unreachable.
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

// Service provides a way to wrap a blocking operation and run it.
//
// Once initialised, an instance of Service must not be changed.
//
// RunFn is assumed to be blocking.
// ShutFn is expected to gracefully shutdown what RunFn started.
type Service struct {
	ShutTimeout time.Duration
	RunFn       func(ctx context.Context) error
	ShutFn      func(ctx context.Context) error
}

// Run runs s.
//
// The supplied implementation for RunFn should regularly check ctx if it's done or not.
//
// When ctx is cancelled, RunFn must return.
func (s *Service) Run(ctx context.Context) error {
	if s.RunFn == nil {
		tck := time.NewTicker(100 * time.Millisecond)
		defer func() { tck.Stop() }()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tck.C:
				continue
			}
		}
	}

	return s.RunFn(ctx)
}

// Shutdown shuts down s.
//
// Service is given a chance to finish any in-flight operations, for the duration of s.Timeout().
// It's expected to succefully finish before ctx times out, and report success with nil.
//
// If it is not possible to finish succesfully, ctx times out, return a non-nil error.
func (s *Service) Shutdown(ctx context.Context) error {
	if s.ShutFn == nil {
		return nil
	}

	return s.ShutFn(ctx)
}

// Timeout determines how long s.Shutdown() is given to finish.
func (s *Service) Timeout() time.Duration {
	return s.ShutTimeout
}

// ServiceClosing allows specifying a force-closing step when Service.Shutdown() returns an error.
type ServiceClosing struct {
	*Service
	CloseFn func() error
}

// Close is the final chance for s to finish.
//
// It's expected to return as soon as possible.
func (s *ServiceClosing) Close() error {
	if s.CloseFn == nil {
		return nil
	}

	return s.CloseFn()
}

func runSvc(ctx context.Context, errChan chan<- error, svc runner) {
	errChan <- svc.Run(ctx)
}
