package daemon

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
)

func TestRunWithSigCn(t *testing.T) {
	type tcGiven struct {
		svc      service
		notifyFn func(cn chan os.Signal)
	}

	tests := []struct {
		name  string
		given tcGiven
		exp   error
	}{
		{
			name: "service_error",
			given: tcGiven{
				svc: &Service{
					RunFn: func(ctx context.Context) error {
						return errors.New("something_went_wrong")
					},
				},
			},
			exp: errors.New("something_went_wrong"),
		},

		{
			name: "sigint_shutdown_timeout",
			given: tcGiven{
				svc: &Service{
					ShutTimeout: 200 * time.Millisecond,

					ShutFn: func(ctx context.Context) error {
						time.Sleep(400 * time.Millisecond)

						return errors.New("something_went_wrong")
					},
				},

				notifyFn: func(cn chan os.Signal) {
					cn <- syscall.SIGINT
				},
			},
			exp: context.DeadlineExceeded,
		},

		{
			name: "sigint_shutdown_error",
			given: tcGiven{
				svc: &Service{
					ShutTimeout: 200 * time.Millisecond,

					ShutFn: func(ctx context.Context) error {
						return errors.New("something_went_wrong")
					},
				},

				notifyFn: func(cn chan os.Signal) {
					cn <- syscall.SIGINT
				},
			},
			exp: errors.New("something_went_wrong"),
		},

		{
			name: "sigint_close_error",
			given: tcGiven{
				svc: &ServiceClosing{
					Service: &Service{
						ShutTimeout: 200 * time.Millisecond,

						ShutFn: func(ctx context.Context) error {
							return errors.New("something_went_wrong_shutdown")
						},
					},

					CloseFn: func() error {
						return errors.New("something_went_wrong_close")
					},
				},

				notifyFn: func(cn chan os.Signal) {
					cn <- syscall.SIGINT
				},
			},
			exp: errors.New("something_went_wrong_close"),
		},

		{
			name: "sigterm_clean",
			given: tcGiven{
				svc: &ServiceClosing{
					Service: &Service{
						ShutTimeout: 200 * time.Millisecond,
					},

					CloseFn: func() error {
						return errors.New("unexpected_close")
					},
				},

				notifyFn: func(cn chan os.Signal) {
					cn <- syscall.SIGTERM
				},
			},
		},
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			ctx := context.Background()

			cn := make(chan os.Signal, 1)
			errCn := make(chan error, 1)

			go func() {
				errCn <- runWithSigCn(ctx, tests[i].given.svc, cn)
				close(errCn)
			}()

			if fn := tests[i].given.notifyFn; fn != nil {
				fn(cn)
			}

			var actual error

			// Limit the waiting time if misconfigured a test case.
			tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			t.Cleanup(cancel)

			select {
			case <-tctx.Done():
			case actual = <-errCn:
			}

			should.Equal(t, tests[i].exp, actual)
		})
	}
}
