package feature

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

// fallbackEvaluator retains the mutable argument to exercise ownership at the evaluator boundary.
type fallbackEvaluator struct {
	*Client
	argument any
	err      error
	invalid  bool
}

// ResolveObject mutates nested containers before returning a controlled decision.
func (e *fallbackEvaluator) ResolveObject(_ context.Context, _ string, fallback any, _ Target) Result[any] {
	e.argument = fallback
	mutateFallback(fallback, "provider")
	value := fallback
	if e.invalid {
		value = map[string]any{"invalid": func() {}}
	}
	return Result[any]{Value: value, Err: e.err}
}

// mutateFallback changes both map and array descendants to detect shallow copies.
func mutateFallback(value any, text string) {
	var object map[string]any
	switch value := value.(type) {
	case map[string]any:
		object = value
	case []any:
		object = value[0].(map[string]any)
	}
	object["text"] = text
	object["nested"].([]any)[0].(map[string]any)["text"] = text
}

// TestObjectFallbackIsolation preserves caller and fallback ownership despite evaluator mutation.
func TestObjectFallbackIsolation(t *testing.T) {
	for _, array := range []bool{false, true} {
		for _, scoped := range []bool{false, true} {
			for _, outcome := range []string{"error", "invalid", "success"} {
				t.Run(fmt.Sprintf("%s/array=%t/scoped=%t", outcome, array, scoped), func(t *testing.T) {
					var fallback any = map[string]any{"text": "original", "nested": []any{map[string]any{"text": "original"}}}
					if array {
						fallback = []any{fallback}
					}
					want, err := CloneValue(fallback)
					if err != nil {
						t.Fatal(err)
					}
					evaluator := &fallbackEvaluator{invalid: outcome == "invalid"}
					var wantErr error
					if outcome == "error" {
						evaluator.err = ErrUnavailable
						wantErr = ErrUnavailable
					}
					if outcome == "invalid" {
						wantErr = ErrInvalidValue
					}
					flags := New(evaluator)
					var result Result[any]
					if scoped {
						result = flags.For(Target{Key: "user"}).Object(context.Background(), "flag", fallback)
					} else {
						result = flags.Object(context.Background(), "flag", fallback)
					}
					if result.Err != wantErr {
						t.Fatalf("error = %v, want %v", result.Err, wantErr)
					}
					if !reflect.DeepEqual(fallback, want) {
						t.Fatal("evaluator mutated caller fallback")
					}
					if outcome == "success" {
						mutateFallback(want, "provider")
					} else if result.Reason != ReasonError {
						t.Fatalf("reason = %v, want %v", result.Reason, ReasonError)
					}
					if !reflect.DeepEqual(result.Value, want) {
						t.Fatalf("value = %#v, want %#v", result.Value, want)
					}
					mutateFallback(evaluator.argument, "retained")
					mutateFallback(fallback, "caller")
					if !reflect.DeepEqual(result.Value, want) {
						t.Fatal("returned value shares evaluator or caller containers")
					}
					mutateFallback(result.Value, "result")
					callerWant, err := CloneValue(want)
					if err != nil {
						t.Fatal(err)
					}
					mutateFallback(callerWant, "caller")
					if !reflect.DeepEqual(fallback, callerWant) {
						t.Fatal("returned value mutation changed caller fallback")
					}
					mutateFallback(callerWant, "retained")
					if !reflect.DeepEqual(evaluator.argument, callerWant) {
						t.Fatal("returned value mutation changed evaluator argument")
					}
				})
			}
		}
	}
}
