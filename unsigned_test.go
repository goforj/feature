package feature

import (
	"context"
	"math"
	"testing"
)

// TestUnsignedTargetConditions keeps the full accepted unsigned target range eligible for numeric rules.
func TestUnsignedTargetConditions(t *testing.T) {
	large := uint64(math.MaxInt64) + 1
	type conditionCase struct {
		name      string
		condition Condition
		target    any
		want      bool
	}
	cases := []conditionCase{
		{"greater", Condition{Attribute: "n", Operator: OpGreater, Value: int64(0)}, large, true},
		{"greater-inclusive", Condition{Attribute: "n", Operator: OpGreaterOrEqual, Value: float64(large)}, large, true},
		{"not-equal", Condition{Attribute: "n", Operator: OpNotEqual, Value: int64(0)}, large, true},
		{"equal-float", Condition{Attribute: "n", Operator: OpEqual, Value: float64(large)}, large, true},
		{"rounded-float-differs", Condition{Attribute: "n", Operator: OpEqual, Value: float64(large)}, large + 1, false},
		{"in", Condition{Attribute: "n", Operator: OpIn, Value: []any{float64(large)}}, large, true},
		{"not-in", Condition{Attribute: "n", Operator: OpNotIn, Value: []any{int64(0)}}, large, true},
		{"less-rounded-limit", Condition{Attribute: "n", Operator: OpLess, Value: float64(math.MaxUint64)}, uint64(math.MaxUint64), true},
		{"less-inclusive", Condition{Attribute: "n", Operator: OpLessOrEqual, Value: float64(math.MaxUint64)}, uint64(math.MaxUint64), true},
		{"nested-equality", Condition{Attribute: "n", Operator: OpEqual, Value: map[string]any{"id": float64(large)}}, map[string]any{"id": large}, true},
		{"nested-inequality", Condition{Attribute: "n", Operator: OpNotEqual, Value: []any{int64(0)}}, []any{large}, true},
		{"contains", Condition{Attribute: "n", Operator: OpContains, Value: float64(large)}, []any{large}, true},
		{"pointer", Condition{Attribute: "/n/id", Operator: OpGreater, Value: int64(0)}, map[string]any{"id": large}, true},
	}
	if uint64(uint(large)) == large {
		cases = append(cases, conditionCase{"uint", Condition{Attribute: "n", Operator: OpGreater, Value: int64(0)}, uint(large), true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: false, Rules: []Rule{{Conditions: []Condition{tc.condition}, Value: true}}}}}
			engine := sourceClient(t, mustMemorySource(t, doc))
			target := Target{Attributes: map[string]any{"n": tc.target}}
			if _, err := target.Clone(); err != nil {
				t.Fatal(err)
			}
			for _, evaluator := range []Evaluator{engine, engine.Snapshot()} {
				result := New(evaluator).For(target).Evaluate(context.Background(), "flag", !tc.want)
				if result.Err != nil || result.Value != tc.want {
					t.Fatalf("unsigned condition: got %+v, want %v", result, tc.want)
				}
			}
		})
	}
	if _, err := CloneValue(large); err != ErrInvalidValue {
		t.Fatal("flag value signed range changed", err)
	}
}

// TestUnsignedComparisonSymmetry preserves exact ordering without narrowing either operand.
func TestUnsignedComparisonSymmetry(t *testing.T) {
	for _, tc := range []struct {
		left  uint64
		right any
		want  int
	}{
		{math.MaxUint64, int64(math.MaxInt64), 1},
		{math.MaxUint64, float64(math.MaxUint64), -1},
		{1 << 63, float64(1 << 63), 0},
		{1<<63 + 1, float64(1 << 63), 1},
	} {
		if got, numeric := numericComparison(tc.left, tc.right); !numeric || got != tc.want {
			t.Fatalf("unsigned comparison: got %d, %v, want %d", got, numeric, tc.want)
		}
		if got, numeric := numericComparison(tc.right, tc.left); !numeric || got != -tc.want {
			t.Fatalf("reversed comparison: got %d, %v, want %d", got, numeric, -tc.want)
		}
	}
}
