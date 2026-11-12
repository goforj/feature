package feature

import "context"

// evaluateNative applies the same snapshot, ordering, and fallback policy to every value type.
func evaluateNative[T any](v *View, ctx context.Context, key string, fallback T, kind ValueType, subject Subject) Result[T] {
	result := Result[T]{Value: fallback, Reason: ReasonError, Revision: v.state.revision}
	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if !validKey(key) {
		result.Err = ErrInvalidKey
		return result
	}
	if !validString(subject.Key) || !validAttributes(subject.Attributes) {
		result.Err = ErrInvalidSubject
		return result
	}
	owned, err := CopyValue(fallback, kind)
	if err != nil {
		result.Err = ErrInvalidValue
		return result
	}
	result.Value = owned.(T)
	flag, ok := v.state.document.Flags[key]
	if !ok {
		result.Reason = ReasonMissing
		result.Err = ErrNotFound
		return result
	}
	if flagType(flag) != kind {
		result.Err = ErrTypeMismatch
		return result
	}
	value := flag.Default
	result.Reason = ReasonDefault
	if flag.Disabled {
		result.Reason = ReasonDisabled
		value = flag.Off
		if value == nil {
			value = false
		}
	} else {
		for _, rule := range flag.Rules {
			if matches(rule, key, subject) {
				value = rule.Value
				result.Reason = ReasonRule
				break
			}
		}
	}
	value, _ = CopyValue(value, kind)
	result.Value = value.(T)
	return result
}

// nativeSubject validates typed targets before matching native string conditions.
func nativeSubject(target Target) (Subject, error) {
	owned, err := target.Clone()
	if err != nil {
		return Subject{}, err
	}
	attributes := make(map[string]string)
	for key, value := range owned.Attributes {
		if text, ok := value.(string); ok {
			attributes[key] = text
		}
	}
	return Subject{Key: owned.Key, Attributes: attributes}, nil
}

// recordResult preserves aggregate accounting for all native value types.
func recordResult[T any](client *Client, result Result[T]) Result[T] {
	client.evaluations.Add(1)
	if result.Err != nil {
		client.fallbacks.Add(1)
	}
	return result
}

// String evaluates a typed value against this immutable snapshot.
func (v *View) String(ctx context.Context, key string, fallback string, subject Subject) Result[string] {
	return evaluateNative(v, ctx, key, fallback, TypeString, subject)
}

// ResolveString evaluates a typed value against the latest native snapshot.
func (c *Client) ResolveString(ctx context.Context, key string, fallback string, target Target) Result[string] {
	view := c.Snapshot()
	if ctx.Err() != nil || !validKey(key) {
		return recordResult(c, view.String(ctx, key, fallback, Subject{}))
	}
	subject, err := nativeSubject(target)
	if err != nil {
		return recordResult(c, Result[string]{Value: fallback, Reason: ReasonError, Revision: view.state.revision, Err: err})
	}
	return recordResult(c, view.String(ctx, key, fallback, subject))
}

// Int evaluates a typed value against this immutable snapshot.
func (v *View) Int(ctx context.Context, key string, fallback int64, subject Subject) Result[int64] {
	return evaluateNative(v, ctx, key, fallback, TypeInteger, subject)
}

// ResolveInt evaluates a typed value against the latest native snapshot.
func (c *Client) ResolveInt(ctx context.Context, key string, fallback int64, target Target) Result[int64] {
	view := c.Snapshot()
	if ctx.Err() != nil || !validKey(key) {
		return recordResult(c, view.Int(ctx, key, fallback, Subject{}))
	}
	subject, err := nativeSubject(target)
	if err != nil {
		return recordResult(c, Result[int64]{Value: fallback, Reason: ReasonError, Revision: view.state.revision, Err: err})
	}
	return recordResult(c, view.Int(ctx, key, fallback, subject))
}

// Float evaluates a typed value against this immutable snapshot.
func (v *View) Float(ctx context.Context, key string, fallback float64, subject Subject) Result[float64] {
	return evaluateNative(v, ctx, key, fallback, TypeFloat, subject)
}

// ResolveFloat evaluates a typed value against the latest native snapshot.
func (c *Client) ResolveFloat(ctx context.Context, key string, fallback float64, target Target) Result[float64] {
	view := c.Snapshot()
	if ctx.Err() != nil || !validKey(key) {
		return recordResult(c, view.Float(ctx, key, fallback, Subject{}))
	}
	subject, err := nativeSubject(target)
	if err != nil {
		return recordResult(c, Result[float64]{Value: fallback, Reason: ReasonError, Revision: view.state.revision, Err: err})
	}
	return recordResult(c, view.Float(ctx, key, fallback, subject))
}

// Object evaluates a typed value against this immutable snapshot.
func (v *View) Object(ctx context.Context, key string, fallback any, subject Subject) Result[any] {
	return evaluateNative(v, ctx, key, fallback, TypeObject, subject)
}

// ResolveObject evaluates a typed value against the latest native snapshot.
func (c *Client) ResolveObject(ctx context.Context, key string, fallback any, target Target) Result[any] {
	view := c.Snapshot()
	if ctx.Err() != nil || !validKey(key) {
		return recordResult(c, view.Object(ctx, key, fallback, Subject{}))
	}
	subject, err := nativeSubject(target)
	if err != nil {
		return recordResult(c, Result[any]{Value: fallback, Reason: ReasonError, Revision: view.state.revision, Err: err})
	}
	return recordResult(c, view.Object(ctx, key, fallback, subject))
}
