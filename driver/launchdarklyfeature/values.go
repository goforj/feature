package launchdarklyfeature

import (
	"context"
	"encoding/json"
	"math"

	"github.com/goforj/feature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// stringClient preserves native type checks and hook evidence for string flags.
type stringClient interface {
	StringVariationDetailCtx(context.Context, string, ldcontext.Context, string) (string, ldreason.EvaluationDetail, error)
}

// numberClient uses floating-point evaluation so integer precision can be checked before conversion.
type numberClient interface {
	Float64VariationDetailCtx(context.Context, string, ldcontext.Context, float64) (float64, ldreason.EvaluationDetail, error)
}

// objectClient is optional so existing borrowed boolean-only clients remain usable.
type objectClient interface {
	JSONVariationDetailCtx(context.Context, string, ldcontext.Context, ldvalue.Value) (ldvalue.Value, ldreason.EvaluationDetail, error)
}

// nativeValue prefers native type checks while retaining compatibility with borrowed JSON-only clients.
func nativeValue(client Client, ctx context.Context, key string, target ldcontext.Context, fallback any, kind feature.ValueType) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	switch kind {
	case feature.TypeString:
		if native, ok := client.(stringClient); ok {
			value, detail, err := native.StringVariationDetailCtx(ctx, key, target, fallback.(string))
			return ldvalue.String(value), detail, err
		}
	case feature.TypeInteger, feature.TypeFloat:
		if native, ok := client.(numberClient); ok {
			number, ok := fallback.(float64)
			if !ok {
				number = float64(fallback.(int64))
			}
			value, detail, err := native.Float64VariationDetailCtx(ctx, key, target, number)
			return ldvalue.Float64(value), detail, err
		}
	}
	if native, ok := client.(objectClient); ok {
		return native.JSONVariationDetailCtx(ctx, key, target, ldvalue.CopyArbitraryValue(fallback))
	}
	return ldvalue.Null(), ldreason.EvaluationDetail{}, feature.ErrUnsupported
}

// copyNativeValue bounds allocations while preserving the SDK's float64 representation of JSON numbers.
func copyNativeValue(value ldvalue.Value, depth int, budget *int) (any, error) {
	if value.Type() == ldvalue.RawType {
		raw := value.AsRaw()
		// Six bytes cover the longest JSON escape for each byte in an owned string.
		if len(raw) > 6*feature.MaxValueBytes {
			return nil, feature.ErrInvalidValue
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, feature.ErrInvalidValue
		}
		value = ldvalue.CopyArbitraryValue(decoded)
	}
	*budget += 16
	if depth > 8 || *budget > feature.MaxValueBytes || value.Count() > feature.MaxSubjects {
		return nil, feature.ErrInvalidValue
	}
	switch value.Type() {
	case ldvalue.ArrayType:
		items := make([]any, value.Count())
		for index := range items {
			item, err := copyNativeValue(value.GetByIndex(index), depth+1, budget)
			if err != nil {
				return nil, err
			}
			items[index] = item
		}
		return items, nil
	case ldvalue.ObjectType:
		items := make(map[string]any, value.Count())
		for _, key := range value.Keys(nil) {
			*budget += len(key)
			item, err := copyNativeValue(value.GetByKey(key), depth+1, budget)
			if err != nil {
				return nil, err
			}
			items[key] = item
		}
		return items, nil
	case ldvalue.StringType:
		*budget += len(value.StringValue())
		if *budget > feature.MaxValueBytes {
			return nil, feature.ErrInvalidValue
		}
	}
	return value.AsArbitraryValue(), nil
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
	value, detail, err := nativeValue(d.client, ctx, key, native, owned, kind)
	if err == feature.ErrUnsupported {
		result.Err = err
		return result
	}
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
	budget := 0
	raw, err := copyNativeValue(value, 0, &budget)
	if err != nil {
		result.Err = err
		return result
	}
	if kind == feature.TypeInteger {
		number, ok := raw.(float64)
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
