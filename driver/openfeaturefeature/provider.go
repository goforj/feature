package openfeaturefeature

import (
	"context"
	"errors"
	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
	"sync"
	"sync/atomic"
)

// Provider exposes an injected evaluator to ordinary OpenFeature SDK consumers.
// It borrows the evaluator: shutdown disables this provider without closing its engine.
type Provider struct {
	features     *feature.Features
	closed       atomic.Bool
	initFailed   atomic.Bool
	evaluator    feature.Evaluator
	mu           sync.Mutex
	once         sync.Once
	started      bool
	subscription *feature.Subscription
	stop         chan struct{}
	done         chan struct{}
	events       chan openfeature.Event
}

// NewProvider exposes an initialized engine without global registration or a second update loop.
// Complete SDK registration before starting the native Run loop, and serialize later
// registrations with Refresh: SDK initialization events can overwrite concurrent health events.
func NewProvider(evaluator feature.Evaluator, options ...feature.Option) *Provider {
	return &Provider{features: feature.New(evaluator, options...), evaluator: evaluator, events: make(chan openfeature.Event, 16)}
}

// Metadata identifies this provider independently of its source or targeting data.
func (p *Provider) Metadata() openfeature.Metadata {
	return openfeature.Metadata{Name: "goforj.feature"}
}

// Hooks leaves application SDK hook configuration under the caller's ownership.
func (p *Provider) Hooks() []openfeature.Hook { return nil }

// Init preserves compatibility with SDKs using the legacy lifecycle interface.
func (p *Provider) Init(target openfeature.EvaluationContext) error {
	return p.InitWithContext(context.Background(), target)
}

// InitWithContext honors startup cancellation and rejects known native source failures.
// A failed initialization leaves the borrowed engine's last valid snapshot available.
// Serialize SDK registration with native refreshes so SDK initialization events cannot
// overwrite newer health events emitted during registration.
func (p *Provider) InitWithContext(ctx context.Context, _ openfeature.EvaluationContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.closed.Load() {
		return feature.ErrClosed
	}
	return p.startEvents(ctx)
}

// Shutdown disables the provider while leaving its borrowed evaluator usable.
func (p *Provider) Shutdown() { p.stopEvents() }

// ShutdownWithContext disables this provider without closing borrowed resources.
func (p *Provider) ShutdownWithContext(context.Context) error { p.Shutdown(); return nil }

// providerTarget preserves typed context while treating reserved identity fields explicitly.
func providerTarget(flat openfeature.FlattenedContext) (feature.Target, error) {
	target := feature.Target{Attributes: make(map[string]any)}
	for key, value := range flat {
		switch key {
		case openfeature.TargetingKey:
			text, ok := value.(string)
			if !ok {
				return feature.Target{}, feature.ErrInvalidSubject
			}
			target.Key = text
		case "kind":
			text, ok := value.(string)
			if !ok {
				return feature.Target{}, feature.ErrInvalidSubject
			}
			target.Kind = text
		default:
			target.Attributes[key] = value
		}
	}
	return target, nil
}

// providerDetail translates classifications without carrying source diagnostics into the SDK.
func providerDetail(reason feature.Reason, revision, variant string, err error) openfeature.ProviderResolutionDetail {
	detail := openfeature.ProviderResolutionDetail{Variant: variant, FlagMetadata: openfeature.FlagMetadata{}}
	if revision != "" {
		detail.FlagMetadata["revision"] = revision
	}
	if err != nil {
		detail.Reason = openfeature.ErrorReason
		switch {
		case errors.Is(err, feature.ErrNotFound):
			detail.ResolutionError = openfeature.NewFlagNotFoundResolutionError(feature.ErrNotFound.Error())
		case errors.Is(err, feature.ErrTypeMismatch):
			detail.ResolutionError = openfeature.NewTypeMismatchResolutionError(feature.ErrTypeMismatch.Error())
		case errors.Is(err, feature.ErrInvalidSubject):
			detail.ResolutionError = openfeature.NewInvalidContextResolutionError(feature.ErrInvalidSubject.Error())
		case errors.Is(err, feature.ErrClosed):
			detail.ResolutionError = openfeature.NewProviderNotReadyResolutionError(feature.ErrClosed.Error())
		default:
			detail.ResolutionError = openfeature.NewGeneralResolutionError("feature: evaluation failed")
		}
		return detail
	}
	switch reason {
	case feature.ReasonDefault:
		detail.Reason = openfeature.DefaultReason
	case feature.ReasonRule:
		detail.Reason = openfeature.TargetingMatchReason
	case feature.ReasonDisabled:
		detail.Reason = openfeature.DisabledReason
	case feature.ReasonSplit:
		detail.Reason = openfeature.SplitReason
	case feature.ReasonStatic:
		detail.Reason = openfeature.StaticReason
	default:
		detail.Reason = openfeature.UnknownReason
	}
	return detail
}

// evaluateProvider keeps lifecycle and reserved context validation identical for every type.
func evaluateProvider[T any](p *Provider, ctx context.Context, key string, fallback T, flat openfeature.FlattenedContext, call func(*feature.Scope) feature.Result[T]) openfeature.GenericResolutionDetail[T] {
	result := feature.Result[T]{Value: fallback, Reason: feature.ReasonError}
	if err := ctx.Err(); err != nil {
		result.Err = err
	} else if p.closed.Load() {
		result.Err = feature.ErrClosed
	} else {
		target, err := providerTarget(flat)
		if err != nil {
			result.Err = err
		} else {
			result = call(p.features.For(target))
		}
	}
	return openfeature.GenericResolutionDetail[T]{Value: result.Value, ProviderResolutionDetail: providerDetail(result.Reason, result.Revision, result.Variant, result.Err)}
}

// BooleanEvaluation implements OpenFeature boolean resolution through the portable engine.
func (p *Provider) BooleanEvaluation(ctx context.Context, key string, fallback bool, flat openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	return evaluateProvider(p, ctx, key, fallback, flat, func(scope *feature.Scope) feature.Result[bool] {
		value := scope.Evaluate(ctx, key, fallback)
		return feature.Result[bool]{Value: value.Value, Reason: value.Reason, Revision: value.Revision, Variant: value.Variant, Err: value.Err}
	})
}

// StringEvaluation implements the SDK provider contract without a competing evaluator.
func (p *Provider) StringEvaluation(ctx context.Context, key string, fallback string, flat openfeature.FlattenedContext) openfeature.StringResolutionDetail {
	return evaluateProvider(p, ctx, key, fallback, flat, func(scope *feature.Scope) feature.Result[string] { return scope.String(ctx, key, fallback) })
}

// IntEvaluation implements the SDK provider contract without a competing evaluator.
func (p *Provider) IntEvaluation(ctx context.Context, key string, fallback int64, flat openfeature.FlattenedContext) openfeature.IntResolutionDetail {
	return evaluateProvider(p, ctx, key, fallback, flat, func(scope *feature.Scope) feature.Result[int64] { return scope.Int(ctx, key, fallback) })
}

// FloatEvaluation implements the SDK provider contract without a competing evaluator.
func (p *Provider) FloatEvaluation(ctx context.Context, key string, fallback float64, flat openfeature.FlattenedContext) openfeature.FloatResolutionDetail {
	return evaluateProvider(p, ctx, key, fallback, flat, func(scope *feature.Scope) feature.Result[float64] { return scope.Float(ctx, key, fallback) })
}

// ObjectEvaluation implements the SDK provider contract without a competing evaluator.
func (p *Provider) ObjectEvaluation(ctx context.Context, key string, fallback any, flat openfeature.FlattenedContext) openfeature.InterfaceResolutionDetail {
	return evaluateProvider(p, ctx, key, fallback, flat, func(scope *feature.Scope) feature.Result[any] { return scope.Object(ctx, key, fallback) })
}

var _ openfeature.FeatureProvider = (*Provider)(nil)
var _ openfeature.ContextAwareStateHandler = (*Provider)(nil)
