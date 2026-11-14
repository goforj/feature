package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/feature-release-smoke/internal/runtime"
	"github.com/goforj/feature"
)

// LifecycleRegistry owns the native refresh worker for the lifetime of this App.
type LifecycleRegistry struct {
	engine *feature.Client
	cancel context.CancelFunc
	done   chan error
}

// NewLifecycleRegistry receives the same engine used by the application facade.
func NewLifecycleRegistry(engine *feature.Client) *LifecycleRegistry {
	return &LifecycleRegistry{engine: engine, done: make(chan error, 1)}
}

// Register attaches app-owned source lifecycle without changing GoForj templates.
func (r *LifecycleRegistry) Register(lifecycle *runtime.Lifecycle) {
	lifecycle.On(runtime.Startup, r.Startup)
	lifecycle.On(runtime.Shutdown, r.Shutdown)
}

// Startup starts a worker whose lifetime is explicitly joined by Shutdown.
func (r *LifecycleRegistry) Startup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() { r.done <- r.engine.Run(runCtx, 20*time.Millisecond) }()
	return nil
}

// Shutdown cancels and joins the refresh worker within the App shutdown context.
func (r *LifecycleRegistry) Shutdown(ctx context.Context) error {
	r.cancel()
	select {
	case err := <-r.done:
		if !errors.Is(err, context.Canceled) {
			return err
		}
		fmt.Println("feature smoke: refresh worker joined")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
