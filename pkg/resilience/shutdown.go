package resilience

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ShutdownCoordinator performs an ordered, bounded client shutdown.
type ShutdownCoordinator struct {
	teardown func(context.Context) error
	release  func(context.Context) error
	timeout  time.Duration
}

// NewShutdownCoordinator creates a signal-driven graceful shutdown handler.
func NewShutdownCoordinator(teardown, release func(context.Context) error, timeout time.Duration) *ShutdownCoordinator {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ShutdownCoordinator{teardown: teardown, release: release, timeout: timeout}
}

// Run waits for SIGINT, SIGTERM, or parent cancellation, then tears down the
// local tunnel and notifies the server to release the remote allocation.
func (c *ShutdownCoordinator) Run(ctx context.Context) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	return c.Wait(ctx, signals)
}

// Wait is the testable form of Run and waits on a caller-provided signal
// channel.
func (c *ShutdownCoordinator) Wait(ctx context.Context, signals <-chan os.Signal) error {
	select {
	case <-ctx.Done():
	case <-signals:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	var shutdownErrors []error
	if c.teardown != nil {
		if err := c.teardown(shutdownCtx); err != nil {
			shutdownErrors = append(shutdownErrors, err)
		}
	}
	if c.release != nil {
		if err := c.release(shutdownCtx); err != nil {
			shutdownErrors = append(shutdownErrors, err)
		}
	}
	return errors.Join(shutdownErrors...)
}
