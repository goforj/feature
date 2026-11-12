package feature

import (
	"context"
	"testing"
	"time"
)

// TestTimeTargetConditions preserves supported time leaves without suppressing unrelated comparisons.
func TestTimeTargetConditions(t *testing.T) {
	when := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		condition Condition
		actual    any
		want      bool
	}{
		{"contains-string", Condition{Attribute: "value", Operator: OpContains, Value: "admin"}, []any{when, "admin"}, true},
		{"contains-object", Condition{Attribute: "value", Operator: OpContains, Value: map[string]any{"enabled": true}}, []any{when, map[string]any{"enabled": true}}, true},
		{"not-equal", Condition{Attribute: "value", Operator: OpNotEqual, Value: "free"}, when, true},
		{"not-in", Condition{Attribute: "value", Operator: OpNotIn, Value: []any{"free"}}, when, true},
		{"not-in-nested", Condition{Attribute: "value", Operator: OpNotIn, Value: []any{map[string]any{"at": nil}}}, map[string]any{"at": when}, true},
		{"unequal-object", Condition{Attribute: "value", Operator: OpNotEqual, Value: map[string]any{"at": nil}}, map[string]any{"at": when}, true},
		{"in-miss", Condition{Attribute: "value", Operator: OpIn, Value: []any{"free"}}, when, false},
		{"no-string-conversion", Condition{Attribute: "value", Operator: OpEqual, Value: when.Format(time.RFC3339)}, when, false},
		{"not-numeric", Condition{Attribute: "value", Operator: OpGreater, Value: 0}, when, false},
		{"not-prefix", Condition{Attribute: "value", Operator: OpPrefix, Value: "2026"}, when, false},
		{"exists", Condition{Attribute: "value", Operator: OpExists}, when, true},
		{"not-missing", Condition{Attribute: "value", Operator: OpMissing}, when, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: false, Rules: []Rule{{Conditions: []Condition{tc.condition}, Value: true}}}}}
			engine := sourceClient(t, mustMemorySource(t, doc))
			target := Target{Attributes: map[string]any{"value": tc.actual}}
			if _, err := target.Clone(); err != nil {
				t.Fatal(err)
			}
			for _, evaluator := range []Evaluator{engine, engine.Snapshot()} {
				result := New(evaluator).For(target).Evaluate(context.Background(), "flag", !tc.want)
				if result.Err != nil || result.Value != tc.want {
					t.Fatalf("time target condition: got %+v, want %v", result, tc.want)
				}
			}
		})
	}
	if _, err := CloneValue(when); err != ErrInvalidValue {
		t.Fatal("configured value domain changed", err)
	}
	if _, err := NewMemorySource(Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: false, Rules: []Rule{{Conditions: []Condition{{Attribute: "value", Operator: OpEqual, Value: when}}, Value: true}}}}}); err != ErrInvalidDocument {
		t.Fatal("time comparison literal accepted", err)
	}
}

// TestConditionRejectsUnsupportedActual keeps invalid internal inputs from matching negative comparisons.
func TestConditionRejectsUnsupportedActual(t *testing.T) {
	condition := Condition{Attribute: "value", Operator: OpNotEqual, Value: int64(0)}
	if conditionMatches(condition, Subject{values: map[string]any{"value": func() {}}}) {
		t.Fatal("unsupported comparison input matched a rule")
	}
}
