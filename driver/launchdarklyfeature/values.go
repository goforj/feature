package launchdarklyfeature

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/goforj/feature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"math"
)

// valueClient is optional so existing borrowed boolean-only clients remain usable.
type valueClient interface {
	JSONVariationDetailCtx(context.Context, string, ldcontext.Context, ldvalue.Value) (ldvalue.Value, ldreason.EvaluationDetail, error)
}

// resolveValue preserves native evaluation while validating portable type and precision boundaries.
func resolveValue[T any](d *Driver, ctx context.Context, key string, fallback T, kind feature.ValueType, target feature.Target) feature.Result[T] {
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
	contextTarget, err := target.Clone()
	if err != nil {
		result.Err = err
		return result
	}
	native, err := toContext(contextTarget)
	if err != nil {
		result.Err = err
		return result
	}
	client, ok := d.client.(valueClient)
	if !ok {
		result.Err = feature.ErrUnsupported
		return result
	}
	value, detail, err := client.JSONVariationDetailCtx(ctx, key, native, ldvalue.CopyArbitraryValue(owned))
	if err != nil || detail.Reason.GetKind() == ldreason.EvalReasonError {
		result.Err = resolutionError(detail.Reason.GetErrorKind())
		if result.Err == feature.ErrNotFound {
			result.Reason = feature.ReasonMissing
		}
		if ctx.Err() != nil {
			result.Err = ctx.Err()
			result.Reason = feature.ReasonError
		}
		return result
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(value.JSONString())))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		result.Err = feature.ErrInvalidValue
		return result
	}
	if kind == feature.TypeInteger {
		wire, ok := raw.(json.Number)
		var number float64
		if ok {
			number, _ = wire.Float64()
		}
		if !ok || math.Trunc(number) != number || math.Abs(number) > 9007199254740991 {
			result.Err = feature.ErrTypeMismatch
			return result
		}
		raw = int64(number)
	}
	raw, err = feature.CopyValue(raw, kind)
	if err != nil {
		result.Err = err
		return result
	}
	result.Value = raw.(T)
	result.Reason = feature.ReasonUnknown
	switch detail.Reason.GetKind() {
	case ldreason.EvalReasonOff:
		result.Reason = feature.ReasonDisabled
	case ldreason.EvalReasonTargetMatch, ldreason.EvalReasonRuleMatch:
		result.Reason = feature.ReasonRule
	case ldreason.EvalReasonFallthrough:
		result.Reason = feature.ReasonDefault
	}
	if detail.VariationIndex.IsDefined() {
		result.Variant = detail.VariationIndex.String()
	}
	return result
}

// ResolveString returns the SDK's typed decision with an explicit fallback.
func (d *Driver) ResolveString(ctx context.Context, key string, fallback string, target feature.Target) feature.Result[string] {
	return resolveValue(d, ctx, key, fallback, feature.TypeString, target)
}

// ResolveInt returns the SDK's typed decision with an explicit fallback.
func (d *Driver) ResolveInt(ctx context.Context, key string, fallback int64, target feature.Target) feature.Result[int64] {
	return resolveValue(d, ctx, key, fallback, feature.TypeInteger, target)
}

// ResolveFloat returns the SDK's typed decision with an explicit fallback.
func (d *Driver) ResolveFloat(ctx context.Context, key string, fallback float64, target feature.Target) feature.Result[float64] {
	return resolveValue(d, ctx, key, fallback, feature.TypeFloat, target)
}

// ResolveObject returns the SDK's typed decision with an explicit fallback.
func (d *Driver) ResolveObject(ctx context.Context, key string, fallback any, target feature.Target) feature.Result[any] {
	return resolveValue(d, ctx, key, fallback, feature.TypeObject, target)
}
