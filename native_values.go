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
	if flag.DefaultVariant != "" {
		value = flag.Variants[flag.DefaultVariant]
	}
	result.Reason = ReasonDefault
	result.Variant = flag.DefaultVariant
	if flag.Disabled {
		result.Reason = ReasonDisabled
		result.Variant = flag.OffVariant
		value = flag.Off
		if flag.OffVariant != "" {
			value = flag.Variants[flag.OffVariant]
		}
		if value == nil {
			value = false
		}
	} else {
		for _, rule := range flag.Rules {
			if matchesAt(rule, key, subject, v.at) {
				value = rule.Value
				if rule.Variant != "" {
					value = flag.Variants[rule.Variant]
				}
				result.Variant = rule.Variant
				result.Reason = ReasonRule
				if rule.Split != nil {
					result.Variant = splitVariant(*rule.Split, key, subject.Key)
					value = flag.Variants[result.Variant]
					result.Reason = ReasonSplit
				}
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
		if text, ok := value.(string); ok && validKey(key) {
			attributes[key] = text
		}
	}
	return Subject{Key: owned.Key, Attributes: attributes, values: owned.Attributes, kind: owned.Kind}, nil
}

// recordResult preserves aggregate accounting for all native value types.
func recordResult[T any](client *Client, result Result[T]) Result[T] {
	client.evaluations.Add(1)
	if result.Err != nil {
		client.fallbacks.Add(1)
	}
	return result
}

// resolveView validates portable targeting without changing snapshot accounting.
func resolveView[T any](v *View, ctx context.Context, key string, fallback T, kind ValueType, target Target) Result[T] {
	if ctx.Err() != nil || !validKey(key) {
		return evaluateNative(v, ctx, key, fallback, kind, Subject{})
	}
	subject, err := nativeSubject(target)
	if err != nil {
		return Result[T]{Value: fallback, Reason: ReasonError, Revision: v.state.revision, Err: err}
	}
	return evaluateNative(v, ctx, key, fallback, kind, subject)
}

// Resolve lets portable targeting scopes capture both revision and evaluation time.
func (v *View) Resolve(ctx context.Context, key string, fallback bool, target Target) Decision {
	result := resolveView(v, ctx, key, fallback, TypeBoolean, target)
	return Decision{Value: result.Value, Reason: result.Reason, Revision: result.Revision, Variant: result.Variant, Err: result.Err}
}

// String evaluates a typed value against this immutable snapshot.
func (v *View) String(ctx context.Context, key string, fallback string, subject Subject) Result[string] {
	return evaluateNative(v, ctx, key, fallback, TypeString, subject)
}

// ResolveString implements portable typed evaluation against this captured revision and time.
func (v *View) ResolveString(ctx context.Context, key string, fallback string, target Target) Result[string] {
	return resolveView(v, ctx, key, fallback, TypeString, target)
}

// ResolveString evaluates against the latest snapshot and records aggregate counters.
func (c *Client) ResolveString(ctx context.Context, key string, fallback string, target Target) Result[string] {
	return recordResult(c, c.Snapshot().ResolveString(ctx, key, fallback, target))
}

// Int evaluates a typed value against this immutable snapshot.
func (v *View) Int(ctx context.Context, key string, fallback int64, subject Subject) Result[int64] {
	return evaluateNative(v, ctx, key, fallback, TypeInteger, subject)
}

// ResolveInt implements portable typed evaluation against this captured revision and time.
func (v *View) ResolveInt(ctx context.Context, key string, fallback int64, target Target) Result[int64] {
	return resolveView(v, ctx, key, fallback, TypeInteger, target)
}

// ResolveInt evaluates against the latest snapshot and records aggregate counters.
func (c *Client) ResolveInt(ctx context.Context, key string, fallback int64, target Target) Result[int64] {
	return recordResult(c, c.Snapshot().ResolveInt(ctx, key, fallback, target))
}

// Float evaluates a typed value against this immutable snapshot.
func (v *View) Float(ctx context.Context, key string, fallback float64, subject Subject) Result[float64] {
	return evaluateNative(v, ctx, key, fallback, TypeFloat, subject)
}

// ResolveFloat implements portable typed evaluation against this captured revision and time.
func (v *View) ResolveFloat(ctx context.Context, key string, fallback float64, target Target) Result[float64] {
	return resolveView(v, ctx, key, fallback, TypeFloat, target)
}

// ResolveFloat evaluates against the latest snapshot and records aggregate counters.
func (c *Client) ResolveFloat(ctx context.Context, key string, fallback float64, target Target) Result[float64] {
	return recordResult(c, c.Snapshot().ResolveFloat(ctx, key, fallback, target))
}

// Object evaluates a typed value against this immutable snapshot.
func (v *View) Object(ctx context.Context, key string, fallback any, subject Subject) Result[any] {
	return evaluateNative(v, ctx, key, fallback, TypeObject, subject)
}

// ResolveObject implements portable typed evaluation against this captured revision and time.
func (v *View) ResolveObject(ctx context.Context, key string, fallback any, target Target) Result[any] {
	return resolveView(v, ctx, key, fallback, TypeObject, target)
}

// ResolveObject evaluates against the latest snapshot and records aggregate counters.
func (c *Client) ResolveObject(ctx context.Context, key string, fallback any, target Target) Result[any] {
	return recordResult(c, c.Snapshot().ResolveObject(ctx, key, fallback, target))
}
