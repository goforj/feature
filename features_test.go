package feature

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// evaluatorFunc supplies deterministic driver responses at the portable boundary.
type evaluatorFunc func(context.Context, string, bool, Target) Decision

// Resolve delegates to the test scenario without a global client.
func (f evaluatorFunc) Resolve(ctx context.Context, key string, fallback bool, target Target) Decision {
	return f(ctx, key, fallback, target)
}

// TestTargetClonePreservesTypesAndOwnership checks every supported scalar and mutable container.
func TestTargetClonePreservesTypesAndOwnership(t *testing.T) {
	values := []any{nil, true, int(1), int8(2), int16(3), int32(4), int64(5), uint(1), uint8(2), uint16(3), uint32(4), uint64(5), float32(1.5), float64(2.5), time.Unix(123, 0), "hello", map[string]any{"a": true}, map[string]string{"a": "b"}, []any{true}, []string{"a"}, map[string]any(nil), map[string]string(nil), []any(nil), []string(nil)}
	target := Target{Key: "tenant:acme", Kind: "organization", Attributes: map[string]any{"values": values}}
	owned, err := target.Clone()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(target, owned) {
		t.Fatal("types or nil containers changed")
	}
	values[16].(map[string]any)["a"] = false
	values[17].(map[string]string)["a"] = "changed"
	values[18].([]any)[0] = false
	values[19].([]string)[0] = "changed"
	if reflect.DeepEqual(target, owned) {
		t.Fatal("target retained mutable input")
	}
	nilTarget, err := (Target{}).Clone()
	if err != nil || nilTarget.Attributes != nil {
		t.Fatal(nilTarget, err)
	}
}

// TestTargetRejectsMalformedAndOversizedData covers bounds before SDK conversion or allocation.
func TestTargetRejectsMalformedAndOversizedData(t *testing.T) {
	badUTF8 := string([]byte{255})
	cases := []Target{
		{Key: strings.Repeat("a", MaxStringBytes+1)}, {Key: badUTF8}, {Kind: strings.Repeat("a", MaxKeyBytes+1)}, {Kind: badUTF8},
		{Attributes: map[string]any{"x": math.NaN()}}, {Attributes: map[string]any{"x": math.Inf(1)}}, {Attributes: map[string]any{"x": float32(math.Inf(1))}},
		{Attributes: map[string]any{"x": float32(math.NaN())}}, {Attributes: map[string]any{"x": badUTF8}}, {Attributes: map[string]any{"x": strings.Repeat("a", MaxStringBytes+1)}},
		{Attributes: map[string]any{"": true}}, {Attributes: map[string]any{strings.Repeat("a", MaxKeyBytes+1): true}}, {Attributes: map[string]any{badUTF8: true}},
		{Attributes: map[string]any{"x": func() {}}}, {Attributes: map[string]any{"x": make([]any, MaxSubjects+1)}}, {Attributes: map[string]any{"x": make([]string, MaxSubjects+1)}},
		{Attributes: map[string]any{"x": []any{func() {}}}}, {Attributes: map[string]any{"x": []string{badUTF8}}},
		{Attributes: map[string]any{"x": map[string]string{"": "x"}}}, {Attributes: map[string]any{"x": map[string]string{badUTF8: "x"}}},
		{Attributes: map[string]any{"x": map[string]string{strings.Repeat("a", MaxKeyBytes+1): "x"}}}, {Attributes: map[string]any{"x": map[string]string{"a": badUTF8}}},
	}
	many := map[string]any{}
	manyStrings := map[string]string{}
	for i := 0; i <= MaxAttributes; i++ {
		many[fmt.Sprint(i)] = true
		manyStrings[fmt.Sprint(i)] = "x"
	}
	cases = append(cases, Target{Attributes: many}, Target{Attributes: map[string]any{"nested": manyStrings}})
	var nested any = true
	for i := 0; i < 10; i++ {
		nested = map[string]any{"a": nested}
	}
	cases = append(cases, Target{Attributes: map[string]any{"nested": nested}})
	large := make([]any, 100)
	for i := range large {
		large[i] = strings.Repeat("x", MaxStringBytes)
	}
	cases = append(cases, Target{Attributes: map[string]any{"large": large}})
	for i, target := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := target.Clone(); err != ErrInvalidSubject {
				t.Fatal(err)
			}
		})
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	if _, err := (Target{Attributes: cycle}).Clone(); err != ErrInvalidSubject {
		t.Fatal(err)
	}
}

// TestPortableFallbacksAndReasons preserves successful off-variations and classifies every failure.
func TestPortableFallbacksAndReasons(t *testing.T) {
	ctx := context.Background()
	var result Decision
	calls := 0
	features := New(evaluatorFunc(func(context.Context, string, bool, Target) Decision { calls++; return result }))
	result = Decision{Value: true, Reason: ReasonDisabled, Variant: "off-true"}
	if d := features.Evaluate(ctx, "checkout", false); !d.Value || d.Reason != ReasonDisabled || d.Variant != "off-true" {
		t.Fatal(d)
	}
	result = Decision{Value: true}
	if !features.Enabled(ctx, "checkout") || features.Evaluate(ctx, "checkout", false).Reason != ReasonUnknown {
		t.Fatal("missing reason normalization")
	}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, ErrNotFound, ErrTypeMismatch, ErrClosed, ErrInvalidSubject, ErrInvalidKey, ErrUnavailable, errors.New("private source credentials")} {
		result = Decision{Value: false, Err: err}
		d := features.Evaluate(ctx, "checkout", true)
		want := err
		if err.Error() == "private source credentials" {
			want = ErrUnavailable
		}
		if !d.Value || d.Err != want {
			t.Fatal(d)
		}
		if err == ErrNotFound && d.Reason != ReasonMissing {
			t.Fatal(d)
		}
	}
	for _, key := range []string{"", strings.Repeat("x", MaxStringBytes+1), string([]byte{255})} {
		if d := features.Evaluate(ctx, key, true); d.Err != ErrInvalidKey || !d.Value {
			t.Fatal(d)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	before := calls
	if d := features.Evaluate(canceled, "checkout", true); d.Err != context.Canceled || !d.Value || calls != before {
		t.Fatal(d)
	}
	result = Decision{Err: ErrUnavailable}
	if features.Enabled(ctx, "checkout") {
		t.Fatal("implicit fallback must be false")
	}
}

// TestScopeOwnsContextAndSupportsConcurrentUse avoids shared-client context mutation.
func TestScopeOwnsContextAndSupportsConcurrentUse(t *testing.T) {
	ctx := context.Background()
	features := New(evaluatorFunc(func(_ context.Context, _ string, _ bool, target Target) Decision {
		return Decision{Value: target.Attributes["paid"] == true}
	}))
	target := Target{Key: "tenant:acme", Attributes: map[string]any{"paid": true}}
	scope := features.For(target)
	target.Attributes["paid"] = false
	if !scope.Enabled(ctx, "checkout") || features.For(target).Enabled(ctx, "checkout") {
		t.Fatal("scope borrowed input")
	}
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if !scope.Enabled(ctx, "checkout") {
				t.Error("scope changed")
			}
		}()
	}
	group.Wait()
	invalid := features.For(Target{Key: strings.Repeat("x", MaxStringBytes+1)})
	if d := invalid.Evaluate(ctx, "checkout", true); d.Err != ErrInvalidSubject || !d.Value {
		t.Fatal(d)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if d := invalid.Evaluate(canceled, "checkout", false); d.Err != context.Canceled {
		t.Fatal(d)
	}
}

// TestNativeResolutionValidationOrder preserves cancellation and key precedence before typed target conversion.
func TestNativeResolutionValidationOrder(t *testing.T) {
	client, _ := openMemory(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx context.Context
		key string
	}{
		{canceled, "checkout-v2"}, {context.Background(), "invalid:key"}, {canceled, "invalid:key"},
	} {
		before := client.Status().Evaluations
		want := client.Evaluate(tc.ctx, tc.key, true, Subject{})
		got := client.Resolve(tc.ctx, tc.key, true, Target{Attributes: map[string]any{"unsupported": func() {}}})
		if got != want {
			t.Errorf("Resolve=%+v Evaluate=%+v", got, want)
		}
		if count := client.Status().Evaluations; count != before+2 {
			t.Errorf("native accounting changed: %d", count-before)
		}
	}
}

// TestNativeResolutionKeepsExistingContracts adapts portable targets without string coercion.
func TestNativeResolutionKeepsExistingContracts(t *testing.T) {
	ctx := context.Background()
	client, _ := openMemory(t)
	features := New(client)
	if !features.For(Target{Key: "tenant:internal"}).Enabled(ctx, "checkout-v2") {
		t.Fatal("native identity targeting lost")
	}
	if d := client.Resolve(ctx, "checkout-v2", true, Target{Attributes: map[string]any{"plan": "pro", "paid": true}}); d.Err != nil {
		t.Fatal(d)
	}
	if d := client.Resolve(ctx, "checkout-v2", true, Target{Key: strings.Repeat("x", MaxStringBytes+1)}); d.Err != ErrInvalidSubject || !d.Value {
		t.Fatal(d)
	}
}
