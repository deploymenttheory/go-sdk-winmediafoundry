// Package events provides a thread-safe fan-out event bus for build lifecycle
// events, plus a WebhookEmitter that POSTs events to an HTTP endpoint.
package events

import (
	"context"
	"sync"

	"github.com/deploymenttheory/go-sdk-uupdump/catalog"
)

// Bus is a thread-safe, fan-out EventEmitter. Handlers are called
// synchronously in the order they were registered, within the goroutine
// that calls Emit. Long-running handlers should spawn their own goroutines.
type Bus struct {
	mu       sync.RWMutex
	handlers map[uint64]catalog.EventHandler
	next     uint64
}

// NewBus creates a new, empty Bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[uint64]catalog.EventHandler),
	}
}

// Subscribe registers h and returns a function that removes it.
// The returned function is idempotent.
func (b *Bus) Subscribe(h catalog.EventHandler) (unsubscribe func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	b.handlers[id] = h
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		delete(b.handlers, id)
		b.mu.Unlock()
	}
}

// Emit broadcasts e to all currently registered handlers. Panics in a handler
// are recovered and do not prevent other handlers from running.
func (b *Bus) Emit(ctx context.Context, e catalog.BuildEvent) {
	b.mu.RLock()
	handlers := make([]catalog.EventHandler, 0, len(b.handlers))
	for _, h := range b.handlers {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		func() {
			defer func() { recover() }() //nolint:errcheck
			h.HandleEvent(ctx, e)
		}()
	}
}
