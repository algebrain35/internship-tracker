package crawler

import (
	"context"
	"sync"
)

// Group runs a collection of goroutines in a shared context.
// The first goroutine to return a non-nil error cancels the context,
// causing all others to stop at their next ctx.Done() check.
//
// This replicates the behaviour of golang.org/x/sync/errgroup without
// requiring an external dependency — and makes the pattern explicit.
type Group struct {
	cancel func()    // cancels the shared context
	wg     sync.WaitGroup
	mu     sync.Mutex
	err    error // first non-nil error seen
}

// NewGroup returns a Group and a derived context.
// When the first goroutine fails, the context is cancelled.
func NewGroup(ctx context.Context) (*Group, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	return &Group{cancel: cancel}, ctx
}

// Go runs fn in a new goroutine, tracking it in the wait group.
func (g *Group) Go(fn func() error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := fn(); err != nil {
			g.mu.Lock()
			if g.err == nil {
				g.err = err  // capture only the first error
				g.cancel()   // cancel everyone else
			}
			g.mu.Unlock()
		}
	}()
}

// Wait blocks until all goroutines have finished and returns the first error.
func (g *Group) Wait() error {
	g.wg.Wait()
	g.cancel() // no-op if already cancelled; cleans up context resources
	return g.err
}
