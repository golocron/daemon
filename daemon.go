// Package daemon provides a convenient way to execute a process.
package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

// Errors returned by daemon.
var (
	ErrInvalidProc = errors.New("invalid process")
	ErrInvalidChan = errors.New("invalid channel")
)

// Runner represents a service which can be Run.
type Runner interface {
	Run() error
}

// Shutdowner represents a service supporting graceful shutdown.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
	Timeout() time.Duration
}

// Closer represents a service supporting force close when graceful is failed.
type Closer interface {
	Close() error
}

// Service is a basic process wrapper.
type Service struct {
	Cmd func() error
}

// NewService returns a new service created from a passed function.
func NewService(f func() error) *Service {
	return &Service{
		Cmd: f,
	}
}

// Run wraps Cmd.
func (s *Service) Run() error {
	return s.Cmd()
}

// Daemon represents a service to run as a daemon.
//
// It must be initialized with New before use.
type Daemon struct {
	ctx     context.Context
	cancel  context.CancelFunc
	proc    Runner
	errChan chan error
	sigChan chan os.Signal
}

// Options holds options for creating a daemon.
//
// ErrChan and SigChan, if set, must have buffer greater than 1.
type Options struct {
	Proc    Runner
	ErrChan chan error
	SigChan chan os.Signal
}

// New returns an instance of a Daemon.
func New(proc Runner) *Daemon {
	return NewWithOptions(&Options{Proc: proc})
}

// NewWithOptions creates an instance of Daemon based on given opt.
func NewWithOptions(opt *Options) *Daemon {
	if opt.Proc == nil {
		panic(ErrInvalidProc)
	}

	var errChan chan error
	if opt.ErrChan != nil {
		if cap(opt.ErrChan) < 1 {
			panic(errors.Wrap(ErrInvalidChan, "buffer size must greater than 0"))
		}

		errChan = opt.ErrChan
	} else {
		errChan = make(chan error, 1)
	}

	var sigChan chan os.Signal
	if opt.SigChan != nil {
		if cap(opt.SigChan) < 1 {
			panic(errors.Wrap(ErrInvalidChan, "buffer size must greater than 0"))
		}

		sigChan = opt.SigChan
	} else {
		sigChan = make(chan os.Signal, 1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &Daemon{
		ctx:     ctx,
		cancel:  cancel,
		proc:    opt.Proc,
		errChan: errChan,
		sigChan: sigChan,
	}

	return d
}

// Ctx returns a new context derived from the daemon.
func (d *Daemon) Ctx() (context.Context, func()) {
	return context.WithCancel(d.ctx)
}

// Start starts Daemon and listens for signals to stop.
func (d *Daemon) Start() error {
	if d.proc == nil {
		return ErrInvalidProc
	}

	signal.Notify(d.sigChan, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	// Run the service.
	go runProcess(d.errChan, d.proc)

	// Listen channels for events.
	select {
	case err := <-d.errChan:
		d.cancel()
		return err

	case <-d.sigChan:
		d.cancel()
		if shut, ok := d.proc.(Shutdowner); ok {
			ctx, cancel := context.WithTimeout(context.Background(), shut.Timeout())
			defer cancel()

			if err := shut.Shutdown(ctx); err != nil {
				if cl, ok := shut.(Closer); ok {
					if err := cl.Close(); err != nil {
						return err
					}
				}

				return err
			}
		}
	}

	return nil
}

// runProcess calls Run on the given Runner.
func runProcess(errChan chan<- error, srv Runner) {
	errChan <- srv.Run()
}
