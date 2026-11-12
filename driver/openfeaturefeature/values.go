package openfeaturefeature

import (
	"context"
	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
)

// valueClient extends borrowed boolean clients only when typed evaluation is requested.
type valueClient interface {
	StringValueDetails(context.Context, string, string, openfeature.EvaluationContext, ...openfeature.Option) (openfeature.StringEvaluationDetails, error)
	IntValueDetails(context.Context, string, int64, openfeature.EvaluationContext, ...openfeature.Option) (openfeature.IntEvaluationDetails, error)
	FloatValueDetails(context.Context, string, float64, openfeature.EvaluationContext, ...openfeature.Option) (openfeature.FloatEvaluationDetails, error)
	ObjectValueDetails(context.Context, string, any, openfeature.EvaluationContext, ...openfeature.Option) (openfeature.InterfaceEvaluationDetails, error)
}

// resolveValue preserves the SDK's hooks and context merging for every value type.
func resolveValue[T any](d *Driver, ctx context.Context, key string, fallback T, kind feature.ValueType, target feature.Target, call func(valueClient, T, openfeature.EvaluationContext) (openfeature.GenericEvaluationDetails[T], error)) feature.Result[T] {
	result := feature.Result[T]{Value: fallback, Reason: feature.ReasonError}
	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if d.closed.Load() {
		result.Err = feature.ErrClosed
		return result
	}
	owned, err := feature.CopyValue(fallback, kind)
	if err != nil {
		result.Err = feature.ErrInvalidValue
		return result
	}
	result.Value = owned.(T)
	native, err := evaluationContext(target)
	if err != nil {
		result.Err = err
		return result
	}
	client, ok := d.client.(valueClient)
	if !ok {
		result.Err = feature.ErrUnsupported
		return result
	}
	detail, err := call(client, result.Value, native)
	if err != nil || detail.ErrorCode != "" || detail.Reason == openfeature.ErrorReason {
		result.Err = resolutionError(detail.ErrorCode)
		if result.Err == feature.ErrNotFound {
			result.Reason = feature.ReasonMissing
		}
		if ctx.Err() != nil {
			result.Err = ctx.Err()
			result.Reason = feature.ReasonError
		}
		return result
	}
	value, err := feature.CopyValue(detail.Value, kind)
	if err != nil {
		result.Err = err
		return result
	}
	result.Value = value.(T)
	result.Variant = detail.Variant
	result.Reason = portableReason(detail.Reason)
	return result
}

// evaluationContext owns mutable SDK input and preserves the explicit identity fields.
func evaluationContext(target feature.Target) (openfeature.EvaluationContext, error) {
	owned, err := target.Clone()
	if err != nil {
		return openfeature.EvaluationContext{}, err
	}
	if owned.Attributes == nil {
		owned.Attributes = make(map[string]any)
	}
	if owned.Kind != "" {
		owned.Attributes["kind"] = owned.Kind
	}
	delete(owned.Attributes, openfeature.TargetingKey)
	return openfeature.NewEvaluationContext(owned.Key, owned.Attributes), nil
}

// portableReason maps structured SDK reasons independently of the resolved value type.
func portableReason(reason openfeature.Reason) feature.Reason {
	switch reason {
	case openfeature.DisabledReason:
		return feature.ReasonDisabled
	case openfeature.TargetingMatchReason:
		return feature.ReasonRule
	case openfeature.SplitReason:
		return feature.ReasonSplit
	case openfeature.DefaultReason:
		return feature.ReasonDefault
	case openfeature.StaticReason:
		return feature.ReasonStatic
	default:
		return feature.ReasonUnknown
	}
}

// ResolveString evaluates through the real SDK's typed client method.
func (d *Driver) ResolveString(ctx context.Context, key string, fallback string, target feature.Target) feature.Result[string] {
	return resolveValue(d, ctx, key, fallback, feature.TypeString, target, func(client valueClient, owned string, native openfeature.EvaluationContext) (openfeature.GenericEvaluationDetails[string], error) {
		return client.StringValueDetails(ctx, key, owned, native)
	})
}

// ResolveInt evaluates through the real SDK's typed client method.
func (d *Driver) ResolveInt(ctx context.Context, key string, fallback int64, target feature.Target) feature.Result[int64] {
	return resolveValue(d, ctx, key, fallback, feature.TypeInteger, target, func(client valueClient, owned int64, native openfeature.EvaluationContext) (openfeature.GenericEvaluationDetails[int64], error) {
		return client.IntValueDetails(ctx, key, owned, native)
	})
}

// ResolveFloat evaluates through the real SDK's typed client method.
func (d *Driver) ResolveFloat(ctx context.Context, key string, fallback float64, target feature.Target) feature.Result[float64] {
	return resolveValue(d, ctx, key, fallback, feature.TypeFloat, target, func(client valueClient, owned float64, native openfeature.EvaluationContext) (openfeature.GenericEvaluationDetails[float64], error) {
		return client.FloatValueDetails(ctx, key, owned, native)
	})
}

// ResolveObject evaluates through the real SDK's typed client method.
func (d *Driver) ResolveObject(ctx context.Context, key string, fallback any, target feature.Target) feature.Result[any] {
	return resolveValue(d, ctx, key, fallback, feature.TypeObject, target, func(client valueClient, owned any, native openfeature.EvaluationContext) (openfeature.GenericEvaluationDetails[any], error) {
		return client.ObjectValueDetails(ctx, key, owned, native)
	})
}
