// Package flagdemo is an app-owned feature decision service used by the release smoke.
package flagdemo

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goforj/feature"
)

// Service borrows the injected facade and owns only a test request gate.
type Service struct {
	features *feature.Features
	captured atomic.Bool
	release  chan struct{}
	once     sync.Once
}

// Decision describes one request captured before a potentially concurrent source update.
type Decision struct {
	PID        int      `json:"pid"`
	FirstTheme string   `json:"first_theme"`
	Theme      string   `json:"theme"`
	Checkout   bool     `json:"checkout"`
	Columns    int      `json:"columns"`
	Revisions  []string `json:"revisions"`
}

// OpenEngine supplies explicit file-source configuration at the application's ownership boundary.
func OpenEngine() (*feature.Client, error) {
	path := os.Getenv("FEATURE_FLAGS_FILE")
	if path == "" {
		return nil, errors.New("feature smoke: FEATURE_FLAGS_FILE is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return feature.Open(ctx, feature.NewFileSource(path))
}

// NewFeatures separates the application read interface from native source lifecycle ownership.
func NewFeatures(engine *feature.Client) *feature.Features { return feature.New(engine) }

// NewService receives the same facade instance used by every controller request.
func NewService(features *feature.Features) *Service {
	return &Service{features: features, release: make(chan struct{})}
}

// Decide freezes all request decisions while an optional gate permits a source update in flight.
func (s *Service) Decide(ctx context.Context, hold bool) (Decision, error) {
	snapshot, err := s.features.Snapshot()
	if err != nil {
		return Decision{}, err
	}
	request := snapshot.For(feature.Target{Key: "tenant:acme", Attributes: map[string]any{"plan": "pro"}})
	first := request.String(ctx, "theme", "fallback")
	if hold {
		s.captured.Store(true)
		select {
		case <-s.release:
		case <-ctx.Done():
			return Decision{}, ctx.Err()
		}
	}
	theme := request.String(ctx, "theme", "fallback")
	checkout := request.Evaluate(ctx, "checkout", false)
	type layout struct {
		Columns int `json:"columns"`
	}
	columns := feature.ObjectAs(ctx, request, "layout", layout{Columns: 1})
	if err := errors.Join(first.Err, theme.Err, checkout.Err, columns.Err); err != nil {
		return Decision{}, err
	}
	return Decision{PID: os.Getpid(), FirstTheme: first.Value, Theme: theme.Value, Checkout: checkout.Value, Columns: columns.Value.Columns, Revisions: []string{first.Revision, theme.Revision, checkout.Revision, columns.Revision}}, nil
}

// Release finishes the single held request without changing feature configuration.
func (s *Service) Release() { s.once.Do(func() { close(s.release) }) }
