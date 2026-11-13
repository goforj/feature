package feature

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

// objectReaderFunc controls result ownership and metadata at the structured boundary.
type objectReaderFunc func(context.Context, string, any) Result[any]

// Object delegates the consumer read without imposing a native engine.
func (f objectReaderFunc) Object(ctx context.Context, key string, fallback any) Result[any] {
	return f(ctx, key, fallback)
}

// rejectObjectJSON models an application-defined decoder that rejects otherwise valid JSON.
type rejectObjectJSON struct{}

// UnmarshalJSON confirms application decoder diagnostics remain private.
func (*rejectObjectJSON) UnmarshalJSON([]byte) error { return errors.New("private decoder field") }

// TestObjectAsTypes preserves native precision, metadata, application fields, and owned containers.
func TestObjectAsTypes(t *testing.T) {
	ctx := context.Background()
	type layout struct {
		Columns int     `json:"columns"`
		IDs     []int64 `json:"ids"`
		Extra   any     `json:"extra"`
	}
	source, err := NewMemorySource(Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{
		"layout": {Variants: map[string]any{"wide": map[string]any{"columns": 3, "ids": []any{int64(9007199254740993)}, "extra": int64(9007199254740993), "future": true}}, DefaultVariant: "wide"},
		"menu":   {Default: []any{"home", "billing"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	flags := New(engine)
	fallback := layout{Columns: 1, IDs: []int64{7}}
	result := ObjectAs(ctx, flags, "layout", fallback)
	if result.Err != nil || result.Value.Columns != 3 || result.Value.IDs[0] != 9007199254740993 || result.Value.Extra.(json.Number).String() != "9007199254740993" || result.Variant != "wide" || result.Revision != engine.Status().Revision || result.Reason != ReasonDefault {
		t.Fatal(result)
	}
	result.Value.IDs[0] = 0
	if fallback.IDs[0] != 7 || ObjectAs(ctx, flags, "layout", fallback).Value.IDs[0] != 9007199254740993 {
		t.Fatal("result aliases source or fallback")
	}
	menu := ObjectAs(ctx, flags.For(Target{Key: "tenant"}), "menu", []string{"fallback"})
	if menu.Err != nil || strings.Join(menu.Value, ",") != "home,billing" {
		t.Fatal(menu)
	}
	missing := ObjectAs(ctx, flags, "missing", fallback)
	missing.Value.IDs[0] = 8
	if missing.Err != ErrNotFound || missing.Reason != ReasonMissing || fallback.IDs[0] != 7 {
		t.Fatal(missing)
	}
}

// TestObjectAsFailures verifies bounded fallback validation and safe conversion failures.
func TestObjectAsFailures(t *testing.T) {
	ctx := context.Background()
	calls := 0
	reader := objectReaderFunc(func(context.Context, string, any) Result[any] {
		calls++
		return Result[any]{Value: map[string]any{"columns": 3}}
	})
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if got := ObjectAs(canceled, reader, "x", map[string]any{}); got.Err != context.Canceled {
		t.Fatal(got)
	}
	for _, key := range []string{"", strings.Repeat("x", MaxStringBytes+1), string([]byte{0xff})} {
		if got := ObjectAs(ctx, reader, key, map[string]any{}); got.Err != ErrInvalidKey {
			t.Fatal(got)
		}
	}
	cycle := map[string]any{}
	cycle["cycle"] = cycle
	for _, fallback := range []any{nil, 1, "text", func() {}, cycle, map[string]any{"x": math.NaN()}, map[string]any{"x": strings.Repeat("x", MaxValueBytes)}, map[string]any{"x": uint64(math.MaxUint64)}} {
		if got := ObjectAs(ctx, reader, "x", fallback); got.Err != ErrInvalidValue {
			t.Fatal("invalid fallback", got)
		}
	}
	if calls != 0 {
		t.Fatal("invalid input reached reader", calls)
	}
	if got := ObjectAs(ctx, reader, "x", rejectObjectJSON{}); got.Err != ErrInvalidValue {
		t.Fatal("decoder failure", got)
	}
	type layout struct {
		Columns int `json:"columns"`
	}
	for _, value := range []any{map[string]any{"columns": "private"}, map[string]any{"columns": 1.5}, map[string]any{"columns": float64(math.MaxInt64)}, map[string]any{"columns": func() {}}, true} {
		broken := objectReaderFunc(func(context.Context, string, any) Result[any] {
			return Result[any]{Value: value, Reason: ReasonRule, Revision: "rev", Variant: "variant"}
		})
		got := ObjectAs(ctx, broken, "x", layout{Columns: 1})
		if got.Err != ErrInvalidValue || got.Value.Columns != 1 || got.Reason != ReasonError || got.Revision != "rev" || got.Variant != "variant" {
			t.Fatal(got)
		}
	}
}

// TestObjectAsFallbackIsolation keeps separate copies when a custom reader mutates its argument.
func TestObjectAsFallbackIsolation(t *testing.T) {
	fallback := map[string]any{"nested": map[string]any{"text": "original"}}
	for _, err := range []error{ErrTypeMismatch, errors.New("private provider text")} {
		reader := objectReaderFunc(func(_ context.Context, _ string, argument any) Result[any] {
			argument.(map[string]any)["nested"].(map[string]any)["text"] = "mutated"
			return Result[any]{Value: argument, Err: err, Revision: "revision"}
		})
		got := ObjectAs(context.Background(), reader, "object", fallback)
		if got.Value["nested"].(map[string]any)["text"] != "original" || fallback["nested"].(map[string]any)["text"] != "original" || got.Revision != "revision" {
			t.Fatal(got)
		}
		if got.Err != ErrTypeMismatch && got.Err != ErrUnavailable {
			t.Fatal(got)
		}
	}
}

// TestObjectAsSDKNumbers decodes integral SDK floats without losing ownership of pointer fallbacks.
func TestObjectAsSDKNumbers(t *testing.T) {
	type layout struct {
		Columns int `json:"columns"`
	}
	fallback := &layout{Columns: 1}
	reader := objectReaderFunc(func(context.Context, string, any) Result[any] {
		return Result[any]{Value: map[string]any{"columns": float64(3)}}
	})
	result := ObjectAs(context.Background(), reader, "layout", fallback)
	if result.Err != nil || result.Reason != ReasonUnknown || result.Value.Columns != 3 || fallback.Columns != 1 || result.Value == fallback {
		t.Fatal(result)
	}
}
