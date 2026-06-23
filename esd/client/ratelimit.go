package client

import (
	"sync"
	"time"
)

// responseTimeTracker computes an exponential moving average (EMA) of API
// response durations and signals when the caller should pause before the next
// request.
//
// When the server begins responding more slowly than its own EMA baseline,
// the excess latency is returned as a suggested pause. This gives the server
// time to recover before the next request arrives without imposing a fixed
// static delay.
type responseTimeTracker struct {
	mu          sync.Mutex
	ema         time.Duration
	alpha       float64
	initialized bool
}

// newResponseTimeTracker returns a tracker using alpha=0.2.
func newResponseTimeTracker() *responseTimeTracker {
	return &responseTimeTracker{alpha: 0.2}
}

// record adds a response duration sample and returns the adaptive delay the
// caller should sleep before issuing the next request.
//
// When the observed duration exceeds 2× the current EMA, the server is under
// measurable pressure. The excess (d − ema) is returned as the suggested
// pause, capped at adaptiveDelayMax.
func (r *responseTimeTracker) record(d time.Duration) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		r.ema = d
		r.initialized = true
		return 0
	}

	r.ema = time.Duration(float64(d)*r.alpha + float64(r.ema)*(1-r.alpha))

	if d <= 2*r.ema {
		return 0
	}

	excess := d - r.ema
	if excess > adaptiveDelayMax {
		return adaptiveDelayMax
	}
	return excess
}
