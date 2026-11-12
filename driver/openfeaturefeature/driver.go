// Package openfeaturefeature adapts an OpenFeature SDK client without bypassing
// its hooks, context merging, provider events, or native evaluation behavior.
// New borrows a client; Open owns a dedicated isolated API and provider lifecycle.
package openfeaturefeature

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

// Client is the SDK evaluation boundary; calling providers directly is intentionally unsupported.
type Client interface {
	BooleanValueDetails(context.Context, string, bool, openfeature.EvaluationContext, ...openfeature.Option) (openfeature.BooleanEvaluationDetails, error)
}

// evaluationAPI is the lifecycle boundary used only by an owning adapter.
type evaluationAPI interface {
	SetProviderAndWait(context.Context, openfeature.FeatureProvider, ...openfeature.APIOption) error
	NewClient(...openfeature.APIOption) *openfeature.Client
	Shutdown(context.Context) error
}

// Driver preserves the configured SDK's behavior and supports concurrent use.
// A generic provider can perform network I/O and does not imply frozen snapshots.
type Driver struct {
	client              Client
	close               func(context.Context) error
	closed              atomic.Bool
	once                sync.Once
	closeErr            error
	trackingUnavailable bool
}

// New borrows an initialized SDK client without changing provider bindings or lifecycle.
func New(client Client) *Driver { return &Driver{client: client} }

// Open owns an isolated API, waits for provider initialization, and starts no global binding.
// The SDK's isolated API is experimental and contained behind this optional module.
// The SDK can invoke shutdown before initialization returns when ctx is canceled.
// Stateful providers must synchronize those lifecycle methods. For providers without
// that guarantee, initialize and manage the SDK yourself and borrow it through New.
func Open(ctx context.Context, provider openfeature.FeatureProvider) (*Driver, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return open(ctx, provider, isolated.NewAPI())
}

// open closes partially initialized providers and allows direct lifecycle failure tests.
func open(ctx context.Context, provider openfeature.FeatureProvider, api evaluationAPI) (*Driver, error) {
	if err := ctx.Err(); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Shutdown(cleanup)
		return nil, err
	}
	err := api.SetProviderAndWait(ctx, provider)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Shutdown(cleanup)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, feature.ErrUnavailable
	}
	_, tracking := provider.(openfeature.Tracker)
	if capability, ok := provider.(interface{ SupportsTracking() bool }); ok {
		tracking = tracking && capability.SupportsTracking()
	}
	return &Driver{client: api.NewClient(), close: api.Shutdown, trackingUnavailable: !tracking}, nil
}

// Close disables the adapter once and closes only the isolated API owned by Open.
// Borrowed clients, other domains, and the process-global API are never closed.
// Concurrent callers wait for the same cleanup and receive the same safe result.
func (d *Driver) Close() error {
	d.once.Do(func() {
		d.closed.Store(true)
		if d.close != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := d.close(ctx); err != nil {
				d.closeErr = feature.ErrUnavailable
			}
		}
	})
	return d.closeErr
}

// Resolve evaluates through the SDK with an owned context and explicit caller fallback.
func (d *Driver) Resolve(ctx context.Context, key string, fallback bool, target feature.Target) feature.Decision {
	result := feature.Decision{Value: fallback, Reason: feature.ReasonError}
	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if d.closed.Load() {
		result.Err = feature.ErrClosed
		return result
	}
	owned, err := target.Clone()
	if err != nil {
		result.Err = err
		return result
	}
	if owned.Attributes == nil {
		owned.Attributes = make(map[string]any)
	}
	if owned.Kind != "" {
		owned.Attributes["kind"] = owned.Kind
	}
	// The explicit field is authoritative over an attribute with the SDK's reserved name.
	delete(owned.Attributes, openfeature.TargetingKey)
	details, err := d.client.BooleanValueDetails(ctx, key, fallback, openfeature.NewEvaluationContext(owned.Key, owned.Attributes))
	if err != nil || details.ResolutionDetail.ErrorCode != "" || details.ResolutionDetail.Reason == openfeature.ErrorReason {
		result.Err = resolutionError(details.ResolutionDetail.ErrorCode)
		if result.Err == feature.ErrNotFound {
			result.Reason = feature.ReasonMissing
		}
		if ctx.Err() != nil {
			result.Err = ctx.Err()
			result.Reason = feature.ReasonError
		}
		return result
	}
	result.Value, result.Variant = details.Value, details.ResolutionDetail.Variant
	switch details.ResolutionDetail.Reason {
	case openfeature.DisabledReason:
		result.Reason = feature.ReasonDisabled
	case openfeature.TargetingMatchReason:
		result.Reason = feature.ReasonRule
	case openfeature.SplitReason:
		result.Reason = feature.ReasonSplit
	case openfeature.DefaultReason:
		result.Reason = feature.ReasonDefault
	case openfeature.StaticReason:
		result.Reason = feature.ReasonStatic
	default:
		result.Reason = feature.ReasonUnknown
	}
	return result
}

// resolutionError maps structured SDK error codes rather than retaining provider prose.
func resolutionError(code openfeature.ErrorCode) error {
	switch code {
	case openfeature.FlagNotFoundCode:
		return feature.ErrNotFound
	case openfeature.TypeMismatchCode:
		return feature.ErrTypeMismatch
	case openfeature.TargetingKeyMissingCode, openfeature.InvalidContextCode:
		return feature.ErrInvalidSubject
	default:
		return feature.ErrUnavailable
	}
}
