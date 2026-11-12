package feature

import (
	"context"
	"time"
	"unicode/utf8"
)

// TypedEvaluator extends boolean drivers with all portable flag value types.
// Features supplies an owned object fallback and preserves a separate copy for failures.
type TypedEvaluator interface {
	Evaluator
	ResolveString(context.Context, string, string, Target) Result[string]
	ResolveInt(context.Context, string, int64, Target) Result[int64]
	ResolveFloat(context.Context, string, float64, Target) Result[float64]
	ResolveObject(context.Context, string, any, Target) Result[any]
}

// resolveTyped applies one validation, ownership, and fallback policy across typed reads.
func resolveTyped[T any](f *Features, ctx context.Context, key string, fallback T, kind ValueType, targetErr error, resolve func(TypedEvaluator, T) Result[T]) (result Result[T]) {
	started := time.Now()
	defer func() {
		f.observe(ctx, Observation{Operation: "evaluate", Key: key, Type: kind, Reason: result.Reason, Revision: result.Revision, Variant: result.Variant, Err: result.Err, Duration: time.Since(started)})
	}()
	result = Result[T]{Value: fallback, Reason: ReasonError}
	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if targetErr != nil {
		result.Err = targetErr
		return result
	}
	if key == "" || len(key) > MaxStringBytes || !utf8.ValidString(key) {
		result.Err = ErrInvalidKey
		return result
	}
	owned, err := CopyValue(fallback, kind)
	if err != nil {
		result.Err = ErrInvalidValue
		return result
	}
	result.Value = owned.(T)
	driver, ok := f.evaluator.(TypedEvaluator)
	if !ok {
		result.Err = ErrUnsupported
		return result
	}
	argument := result.Value
	if kind == TypeObject {
		// The validated fallback must survive mutations made by a custom evaluator.
		owned, _ := CloneValue(result.Value)
		argument = owned.(T)
	}
	decision := resolve(driver, argument)
	if decision.Err != nil {
		decision.Value = result.Value
		decision.Err = safeError(decision.Err)
		decision.Reason = ReasonError
		if decision.Err == ErrNotFound {
			decision.Reason = ReasonMissing
		}
		return decision
	}
	value, err := CopyValue(decision.Value, kind)
	if err != nil {
		decision.Value = result.Value
		decision.Err = ErrInvalidValue
		decision.Reason = ReasonError
		return decision
	}
	decision.Value = value.(T)
	if decision.Reason == "" {
		decision.Reason = ReasonUnknown
	}
	return decision
}

// String returns a typed decision with an explicit fallback.
func (f *Features) String(ctx context.Context, key string, fallback string) Result[string] {
	return resolveTyped(f, ctx, key, fallback, TypeString, nil, func(driver TypedEvaluator, owned string) Result[string] {
		return driver.ResolveString(ctx, key, owned, Target{})
	})
}

// String evaluates with the scope's owned targeting data and an explicit fallback.
func (s *Scope) String(ctx context.Context, key string, fallback string) Result[string] {
	return resolveTyped(s.features, ctx, key, fallback, TypeString, s.err, func(driver TypedEvaluator, owned string) Result[string] {
		return driver.ResolveString(ctx, key, owned, s.target)
	})
}

// Int returns a typed decision with an explicit fallback.
func (f *Features) Int(ctx context.Context, key string, fallback int64) Result[int64] {
	return resolveTyped(f, ctx, key, fallback, TypeInteger, nil, func(driver TypedEvaluator, owned int64) Result[int64] {
		return driver.ResolveInt(ctx, key, owned, Target{})
	})
}

// Int evaluates with the scope's owned targeting data and an explicit fallback.
func (s *Scope) Int(ctx context.Context, key string, fallback int64) Result[int64] {
	return resolveTyped(s.features, ctx, key, fallback, TypeInteger, s.err, func(driver TypedEvaluator, owned int64) Result[int64] {
		return driver.ResolveInt(ctx, key, owned, s.target)
	})
}

// Float returns a typed decision with an explicit fallback.
func (f *Features) Float(ctx context.Context, key string, fallback float64) Result[float64] {
	return resolveTyped(f, ctx, key, fallback, TypeFloat, nil, func(driver TypedEvaluator, owned float64) Result[float64] {
		return driver.ResolveFloat(ctx, key, owned, Target{})
	})
}

// Float evaluates with the scope's owned targeting data and an explicit fallback.
func (s *Scope) Float(ctx context.Context, key string, fallback float64) Result[float64] {
	return resolveTyped(s.features, ctx, key, fallback, TypeFloat, s.err, func(driver TypedEvaluator, owned float64) Result[float64] {
		return driver.ResolveFloat(ctx, key, owned, s.target)
	})
}

// Object returns a typed decision with an explicit fallback.
func (f *Features) Object(ctx context.Context, key string, fallback any) Result[any] {
	return resolveTyped(f, ctx, key, fallback, TypeObject, nil, func(driver TypedEvaluator, owned any) Result[any] {
		return driver.ResolveObject(ctx, key, owned, Target{})
	})
}

// Object evaluates with the scope's owned targeting data and an explicit fallback.
func (s *Scope) Object(ctx context.Context, key string, fallback any) Result[any] {
	return resolveTyped(s.features, ctx, key, fallback, TypeObject, s.err, func(driver TypedEvaluator, owned any) Result[any] {
		return driver.ResolveObject(ctx, key, owned, s.target)
	})
}
