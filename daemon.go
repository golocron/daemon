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

var (
	// ErrInvalidProc indicates nil process.
	ErrInvalidProc = errors.New("invalid process")
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

// Run wraps Cmd.
func (s Service) Run() error {
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
type Options struct {
	Proc    Runner
	ErrChan chan error
	SigChan chan os.Signal
}

// New returns an instance of a Daemon.
func New(opt *Options) (*Daemon, error) {
	if opt.Proc == nil {
		return nil, ErrInvalidProc
	}

	ctx, cancel := context.WithCancel(context.Background())

	errChan := opt.ErrChan
	if errChan == nil {
		errChan = make(chan error, 1)
	}

	sigChan := opt.SigChan
	if sigChan == nil {
		sigChan = make(chan os.Signal, 1)
	}

	d := &Daemon{
		ctx:     ctx,
		cancel:  cancel,
		proc:    opt.Proc,
		errChan: errChan,
		sigChan: sigChan,
	}

	return d, nil
}

// SetProcess sets the process to be run.
func (d *Daemon) SetProcess(proc Runner) {
	d.proc = proc
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
