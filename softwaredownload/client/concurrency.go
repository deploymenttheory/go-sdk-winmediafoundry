package client

import "context"

// semaphore is a buffered-channel based concurrency limiter.
// A nil semaphore means unlimited concurrent requests (default).
type semaphore struct {
	ch chan struct{}
}

// newSemaphore creates a semaphore that allows at most n concurrent holders.
func newSemaphore(n int) *semaphore {
	return &semaphore{ch: make(chan struct{}, n)}
}

// acquire blocks until a slot is available or ctx is cancelled.
func (s *semaphore) acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release returns a slot to the semaphore.
func (s *semaphore) release() {
	select {
	case <-s.ch:
	default:
	}
}
