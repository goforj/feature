package feature

import (
	"context"
	"errors"
	"math"
	"time"
	"unicode/utf8"
)

const (
	// MaxTargetBytes bounds an owned portable evaluation context before SDK conversion.
	MaxTargetBytes = 64 << 10
)

var (
	// ErrUnavailable identifies a provider failure without exposing provider error text.
	ErrUnavailable = errors.New("feature: provider unavailable")
	// ErrTypeMismatch identifies a flag whose configured type differs from the requested type.
	ErrTypeMismatch = errors.New("feature: flag type mismatch")
	// ErrClosed identifies evaluation through a closed driver.
	ErrClosed = errors.New("feature: driver closed")
)

// Target carries typed evaluation data independently of a vendor SDK.
// Attributes support nil, booleans, Go numeric primitives, strings, time.Time,
// map[string]any, map[string]string, []any, and []string. Kind is optional;
// drivers document how identity kinds and structured attributes are mapped.
// Existing Subject remains the native engine's string-only compatibility API.
type Target struct {
	Key        string
	Kind       string
	Attributes map[string]any
}

// Clone validates bounds and owns mutable context data without changing scalar types.
func (t Target) Clone() (Target, error) {
	if !validString(t.Key) || len(t.Kind) > MaxKeyBytes || !utf8.ValidString(t.Kind) {
		return Target{}, ErrInvalidSubject
	}
	budget := len(t.Key) + len(t.Kind)
	attributes, err := cloneTargetValue(t.Attributes, 0, &budget)
	if err != nil {
		return Target{}, err
	}
	return Target{Key: t.Key, Kind: t.Kind, Attributes: attributes.(map[string]any)}, nil
}

// cloneTargetValue bounds nesting and aggregate storage before copying each container.
func cloneTargetValue(value any, depth int, budget *int) (any, error) {
	*budget += 16
	if depth > 8 || *budget > MaxTargetBytes {
		return nil, ErrInvalidSubject
	}
	switch value := value.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, time.Time:
		return value, nil
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, ErrInvalidSubject
		}
		return value, nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, ErrInvalidSubject
		}
		return value, nil
	case string:
		*budget += len(value)
		if !validString(value) || *budget > MaxTargetBytes {
			return nil, ErrInvalidSubject
		}
		return value, nil
	case map[string]any:
		if value == nil {
			return value, nil
		}
		if len(value) > MaxAttributes {
			return nil, ErrInvalidSubject
		}
		result := make(map[string]any, len(value))
		for key, item := range value {
			if key == "" || len(key) > MaxKeyBytes || !utf8.ValidString(key) {
				return nil, ErrInvalidSubject
			}
			*budget += len(key)
			next, err := cloneTargetValue(item, depth+1, budget)
			if err != nil {
				return nil, err
			}
			result[key] = next
		}
		return result, nil
	case map[string]string:
		if value == nil {
			return value, nil
		}
		if len(value) > MaxAttributes {
			return nil, ErrInvalidSubject
		}
		result := make(map[string]string, len(value))
		for key, item := range value {
			if key == "" || len(key) > MaxKeyBytes || !utf8.ValidString(key) {
				return nil, ErrInvalidSubject
			}
			*budget += len(key)
			if _, err := cloneTargetValue(item, depth+1, budget); err != nil {
				return nil, err
			}
			result[key] = item
		}
		return result, nil
	case []any:
		if value == nil {
			return value, nil
		}
		if len(value) > MaxSubjects {
			return nil, ErrInvalidSubject
		}
		result := make([]any, len(value))
		for index, item := range value {
			next, err := cloneTargetValue(item, depth+1, budget)
			if err != nil {
				return nil, err
			}
			result[index] = next
		}
		return result, nil
	case []string:
		if value == nil {
			return value, nil
		}
		if len(value) > MaxSubjects {
			return nil, ErrInvalidSubject
		}
		result := make([]string, len(value))
		for index, item := range value {
			if _, err := cloneTargetValue(item, depth+1, budget); err != nil {
				return nil, err
			}
			result[index] = item
		}
		return result, nil
	default:
		return nil, ErrInvalidSubject
	}
}

// Evaluator is the portable boolean resolution boundary implemented by drivers.
// Implementations preserve vendor values and use safe classified errors. They
// must not mutate Target data, must support concurrent use, and document any I/O.
// Features borrows the evaluator and never starts or closes its resources.
type Evaluator interface {
	Resolve(context.Context, string, bool, Target) Decision
}

// Features provides portable flag checks through a required borrowed evaluator.
// Native Client remains responsible for native refresh, status, and snapshots.
type Features struct{ evaluator Evaluator }

// New wraps an initialized evaluator without I/O, global registration, or workers.
func New(evaluator Evaluator) *Features { return &Features{evaluator: evaluator} }

// Enabled evaluates an untargeted flag with a false fallback.
func (f *Features) Enabled(ctx context.Context, key string) bool {
	return f.Evaluate(ctx, key, false).Value
}

// Evaluate explains an untargeted flag with an explicit fallback.
func (f *Features) Evaluate(ctx context.Context, key string, fallback bool) Decision {
	return f.resolve(ctx, key, fallback, Target{})
}

// For owns a target for repeated checks without mutating a shared client.
// Invalid targets are retained as a classified error for subsequent evaluations.
// A Scope captures targeting data, not a provider configuration snapshot.
func (f *Features) For(target Target) *Scope {
	owned, err := target.Clone()
	return &Scope{features: f, target: owned, err: err}
}

// Scope owns targeting data for one request, job, or other evaluation scope.
type Scope struct {
	features *Features
	target   Target
	err      error
}

// Enabled evaluates a targeted flag with a false fallback.
func (s *Scope) Enabled(ctx context.Context, key string) bool {
	return s.Evaluate(ctx, key, false).Value
}

// Evaluate explains a targeted flag with an explicit fallback.
func (s *Scope) Evaluate(ctx context.Context, key string, fallback bool) Decision {
	if err := ctx.Err(); err != nil {
		return Decision{Value: fallback, Reason: ReasonError, Err: err}
	}
	if s.err != nil {
		return Decision{Value: fallback, Reason: ReasonError, Err: s.err}
	}
	return s.features.resolve(ctx, key, fallback, s.target)
}

// resolve enforces portable fallback policy without reinterpreting successful vendor values.
func (f *Features) resolve(ctx context.Context, key string, fallback bool, target Target) Decision {
	if err := ctx.Err(); err != nil {
		return Decision{Value: fallback, Reason: ReasonError, Err: err}
	}
	if key == "" || len(key) > MaxStringBytes || !utf8.ValidString(key) {
		return Decision{Value: fallback, Reason: ReasonError, Err: ErrInvalidKey}
	}
	decision := f.evaluator.Resolve(ctx, key, fallback, target)
	if decision.Err != nil {
		decision.Value, decision.Reason = fallback, ReasonError
		decision.Err = safeError(decision.Err)
		if errors.Is(decision.Err, ErrNotFound) {
			decision.Reason = ReasonMissing
		}
	} else if decision.Reason == "" {
		decision.Reason = ReasonUnknown
	}
	return decision
}

// safeError prevents custom provider errors from leaking untrusted service text.
func safeError(err error) error {
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, ErrNotFound, ErrTypeMismatch, ErrClosed, ErrInvalidSubject, ErrInvalidKey, ErrInvalidValue, ErrUnsupported, ErrUnavailable} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrUnavailable
}

// Resolve adapts the native engine without changing its document or snapshot contracts.
// Native rules match only string attributes; typed non-string attributes do not match.
// Kind does not alter native identity hashing or its explicit key convention.
func (c *Client) Resolve(ctx context.Context, key string, fallback bool, target Target) Decision {
	if ctx.Err() != nil || !validKey(key) {
		return c.Evaluate(ctx, key, fallback, Subject{})
	}
	owned, err := target.Clone()
	if err != nil {
		return Decision{Value: fallback, Reason: ReasonError, Err: err}
	}
	attributes := make(map[string]string)
	for key, value := range owned.Attributes {
		if text, ok := value.(string); ok {
			attributes[key] = text
		}
	}
	return c.Evaluate(ctx, key, fallback, Subject{Key: owned.Key, Attributes: attributes})
}
