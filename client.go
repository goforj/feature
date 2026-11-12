package feature

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Source supplies a full document; Client serializes calls to the borrowed source.
// Implementations must honor cancellation, must not mutate returned data during
// Refresh, and must not call back into the owning client's Refresh or Status.
type Source interface {
	Load(context.Context) (Document, error)
}

// Status reports process-local freshness and bounded aggregate evaluation counts.
type Status struct {
	Revision    string
	LastAttempt time.Time
	LastSuccess time.Time
	LastError   error
	Evaluations uint64
	Fallbacks   uint64
}

// Client owns immutable snapshots and borrows a required Source without closing it.
// Construct clients with Open; a zero Client is not initialized.
type Client struct {
	source      Source
	gate        chan struct{}
	state       atomic.Pointer[compiled]
	running     atomic.Bool
	mu          sync.Mutex
	status      Status
	evaluations atomic.Uint64
	fallbacks   atomic.Uint64
}

// Open loads and validates the initial document synchronously without starting workers.
func Open(ctx context.Context, source Source) (*Client, error) {
	client := &Client{source: source, gate: make(chan struct{}, 1)}
	if err := client.Refresh(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

// Snapshot captures the complete current revision without waiting for source I/O.
func (c *Client) Snapshot() *View {
	return &View{state: c.state.Load()}
}

// Enabled returns a local decision and counts fallback failures without recording keys.
func (c *Client) Enabled(ctx context.Context, key string, fallback bool, subject Subject) bool {
	return c.Evaluate(ctx, key, fallback, subject).Value
}

// Evaluate explains a decision against the latest published snapshot.
func (c *Client) Evaluate(ctx context.Context, key string, fallback bool, subject Subject) Decision {
	decision := c.Snapshot().Evaluate(ctx, key, fallback, subject)
	c.evaluations.Add(1)
	if decision.Err != nil {
		c.fallbacks.Add(1)
	}
	return decision
}

// Refresh serializes loads and atomically publishes only complete valid documents.
// Source failures are redacted; context errors and document errors stay classified.
func (c *Client) Refresh(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	// Acquisition and cancellation may both be ready, so recheck before invoking a source.
	if err := ctx.Err(); err != nil {
		return err
	}
	attempt := time.Now()
	doc, err := c.source.Load(ctx)
	if err != nil {
		if errors.Is(err, ErrInvalidDocument) {
			err = ErrInvalidDocument
		} else {
			err = ErrSource
		}
	} else {
		var next *compiled
		next, err = compile(doc)
		if err == nil {
			if err = ctx.Err(); err == nil {
				c.state.Store(next)
			}
		}
	}
	if err != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	c.mu.Lock()
	c.status.LastAttempt = attempt
	c.status.LastError = err
	if err == nil {
		c.status.LastSuccess = time.Now()
		c.status.Revision = c.state.Load().revision
	}
	c.mu.Unlock()
	return err
}

// Run refreshes on ticks until cancellation, retaining valid state after load errors.
// The caller owns the goroutine and must wait for Run to return during shutdown.
func (c *Client) Run(ctx context.Context, interval time.Duration) error {
	if interval < 10*time.Millisecond {
		return ErrInvalidInterval
	}
	if !c.running.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	defer c.running.Store(false)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = c.Refresh(ctx)
		}
	}
}

// Status returns a detached freshness record; view evaluations are intentionally uncounted.
func (c *Client) Status() Status {
	c.mu.Lock()
	status := c.status
	c.mu.Unlock()
	status.Evaluations = c.evaluations.Load()
	status.Fallbacks = c.fallbacks.Load()
	return status
}
