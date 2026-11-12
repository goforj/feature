package feature

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"time"
)

// Reason explains the selected value without recording targeting data.
type Reason string

const (
	// ReasonDefault indicates that no rule matched the configured flag default.
	ReasonDefault Reason = "default"
	// ReasonRule indicates that an ordered targeting rule matched.
	ReasonRule Reason = "rule"
	// ReasonDisabled indicates a disabled flag; the native engine forces false.
	// External drivers preserve the vendor's configured off-variation.
	ReasonDisabled Reason = "disabled"
	// ReasonMissing indicates that the flag is absent from the captured snapshot.
	ReasonMissing Reason = "missing"
	// ReasonError indicates cancellation or invalid evaluation input.
	ReasonError Reason = "error"
	// ReasonUnknown indicates that a provider supplied no portable reason.
	ReasonUnknown Reason = "unknown"
	// ReasonStatic indicates a provider's static value.
	ReasonStatic Reason = "static"
	// ReasonSplit indicates a provider's percentage or experimental allocation.
	ReasonSplit Reason = "split"
)

// Decision includes a value, its explanation, the captured revision, and any fallback error.
type Decision struct {
	Value    bool
	Reason   Reason
	Revision string
	// Variant identifies a vendor variation when the driver can supply it.
	Variant string
	Err     error
}

// View holds a complete immutable revision for request or job consistency.
type View struct {
	state *compiled
	at    time.Time
}

// Enabled evaluates against this view and returns the caller fallback on failure.
func (v *View) Enabled(ctx context.Context, key string, fallback bool, subject Subject) bool {
	return v.Evaluate(ctx, key, fallback, subject).Value
}

// Evaluate explains one local decision without logging identities or attributes.
func (v *View) Evaluate(ctx context.Context, key string, fallback bool, subject Subject) Decision {
	result := evaluateNative(v, ctx, key, fallback, TypeBoolean, subject)
	return Decision{Value: result.Value, Reason: result.Reason, Revision: result.Revision, Variant: result.Variant, Err: result.Err}
}

// matches requires every condition, with missing identities excluded from rollouts.
func matches(rule Rule, key string, subject Subject) bool {
	return matchesAt(rule, key, subject, time.Now())
}

// matchesAt keeps calendar eligibility and stable targeting on one captured evaluation time.
func matchesAt(rule Rule, key string, subject Subject, at time.Time) bool {
	if rule.Start != nil && at.Before(*rule.Start) || rule.End != nil && !at.Before(*rule.End) {
		return false
	}
	if len(rule.Subjects) > 0 {
		found := false
		for _, candidate := range rule.Subjects {
			if candidate == subject.Key {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for key, value := range rule.Attributes {
		if actual, present := subject.Attributes[key]; !present || actual != value {
			return false
		}
	}
	for _, condition := range rule.Conditions {
		if !conditionMatches(condition, subject) {
			return false
		}
	}
	if rule.Split != nil && subject.Key == "" {
		return false
	}
	if rule.Rollout != nil {
		percentage := rule.Rollout.BasisPoints
		if rule.Rollout.Progressive != nil {
			percentage = progressiveBasis(*rule.Rollout.Progressive, at)
		}
		return subject.Key != "" && bucket(key, rule.Rollout.Salt, subject.Key) < uint64(percentage)
	}
	return true
}

// bucket freezes stable v1 assignments independently of Go's randomized map hashes.
func bucket(key, salt, subject string) uint64 {
	h := sha256.New()
	h.Write([]byte("goforj-feature-rollout-v1"))
	writeHashField(h, key)
	writeHashField(h, salt)
	writeHashField(h, subject)
	return binary.BigEndian.Uint64(h.Sum(nil)[:8]) % 10000
}

// writeHashField makes concatenated UTF-8 fields unambiguous across implementations.
func writeHashField(h hash.Hash, value string) {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(value)))
	h.Write(size[:])
	h.Write([]byte(value))
}
