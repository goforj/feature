package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestComparisonCacheNormalization preserves comparison types and owns each structured copy once.
func TestComparisonCacheNormalization(t *testing.T) {
	when := time.Unix(123, 0).UTC()
	for _, actual := range []any{
		map[string]any{"values": []any{uint64(math.MaxUint64), when, int(3), json.Number("1.5")}},
		map[string]string{"plan": "pro"}, []any{uint64(math.MaxUint64), when, int(3)}, []string{"admin"},
	} {
		t.Run(fmt.Sprintf("%T", actual), func(t *testing.T) {
			var cache comparisonCache
			want, err := (*comparisonCache)(nil).normalize("value", actual)
			if err != nil {
				t.Fatal(err)
			}
			got, err := cache.normalize("value", actual)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("normalization: %v, %v", got, err)
			}
			switch value := got.(type) {
			case map[string]any:
				value["cache-marker"] = true
			case []any:
				value[0] = "cache-marker"
			}
			again, err := cache.normalize("value", actual)
			if err != nil || !reflect.DeepEqual(got, again) || len(cache.values) != 1 {
				t.Fatal("structured value was normalized again", again, err)
			}
			uncached, err := (*comparisonCache)(nil).normalize("value", actual)
			if err != nil || !reflect.DeepEqual(uncached, want) {
				t.Fatal("comparison copy changed its input", err)
			}
			if _, err := cache.normalize("other", actual); err != nil || len(cache.values) != 2 {
				t.Fatal("distinct attributes shared a cache entry", err)
			}
		})
	}
	var cache comparisonCache
	for _, actual := range []any{nil, true, "pro", int(3), uint64(math.MaxUint64), when, json.Number("1.5"), func() {}} {
		want, wantErr := (*comparisonCache)(nil).normalize("scalar", actual)
		got, err := cache.normalize("scalar", actual)
		if !reflect.DeepEqual(got, want) || err != wantErr || cache.values != nil {
			t.Fatal("scalar behavior changed or allocated a cache", got, err)
		}
	}
}

// TestComparisonCacheInvalidInput preserves validation failures across repeated negative comparisons.
func TestComparisonCacheInvalidInput(t *testing.T) {
	var deep any = true
	for range 10 {
		deep = []any{deep}
	}
	for _, actual := range []any{
		[]any{func() {}}, map[string]any{"bad": math.NaN()},
		map[string]string{"bad": string([]byte{255})}, []string{strings.Repeat("x", MaxValueBytes)},
		make([]any, MaxSubjects+1), deep,
	} {
		var cache comparisonCache
		subject := Subject{values: map[string]any{"value": actual}}
		for range 2 {
			for _, operator := range []Operator{OpNotEqual, OpNotIn} {
				value := any("unrelated")
				if operator == OpNotIn {
					value = []any{"unrelated"}
				}
				condition := Condition{Attribute: "value", Operator: operator, Value: value}
				if conditionMatches(condition, subject, &cache) || conditionMatches(condition, subject, nil) {
					t.Fatalf("invalid %T matched %s", actual, operator)
				}
			}
		}
		if len(cache.values) != 1 || cache.values["value"].err != ErrInvalidValue {
			t.Fatal("validation failure was not cached", cache.values)
		}
	}
}

// TestComparisonCacheEvaluationIsolation keeps cache lifetimes within a single flag decision.
func TestComparisonCacheEvaluationIsolation(t *testing.T) {
	ctx := context.Background()
	conditions := []Condition{
		{Attribute: "roles", Operator: OpContains, Value: "admin"},
		{Attribute: "roles", Operator: OpContains, Value: "viewer"},
	}
	doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{
		"flag": {Default: false, Rules: []Rule{
			{Conditions: append(append([]Condition{}, conditions...), Condition{Attribute: "roles", Operator: OpContains, Value: "owner"}), Value: true},
			{Conditions: conditions, Value: true},
		}},
		"other": {Default: false, Rules: []Rule{{Conditions: []Condition{{Attribute: "roles", Operator: OpContains, Value: "owner"}}, Value: true}}},
	}}
	source := mustMemorySource(t, doc)
	client := sourceClient(t, source)
	view := client.Snapshot()
	roles := []string{"admin", "viewer"}
	target := Target{Attributes: map[string]any{"roles": roles}}
	for range 2 {
		if got := view.Resolve(ctx, "flag", false, target); got.Err != nil || !got.Value || got.Reason != ReasonRule {
			t.Fatal("repeated conditions or rules changed", got)
		}
	}
	roles[0] = "owner"
	if got := view.Resolve(ctx, "flag", true, target); got.Err != nil || got.Value {
		t.Fatal("evaluation reused a previous target value", got)
	}
	if got := view.Resolve(ctx, "other", false, target); got.Err != nil || !got.Value {
		t.Fatal("flags shared comparison state", got)
	}
	if got := view.Resolve(ctx, "flag", false, Target{Attributes: map[string]any{"roles": []any{"admin", "viewer"}}}); got.Err != nil || !got.Value {
		t.Fatal("targets shared comparison state", got)
	}
	doc.Flags["flag"] = Flag{Default: false}
	if err := source.Replace(doc); err != nil {
		t.Fatal(err)
	}
	if err := client.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	roles[0] = "admin"
	if old, current := view.Resolve(ctx, "flag", false, target), client.Snapshot().Resolve(ctx, "flag", true, target); old.Err != nil || !old.Value || current.Err != nil || current.Value || old.Revision == current.Revision {
		t.Fatal("snapshots shared comparison state", old, current)
	}
}

// BenchmarkStructuredComparisons measures repeated scans with distinct operands and a late rule miss.
func BenchmarkStructuredComparisons(b *testing.B) {
	for _, size := range []struct{ rules, conditions, items int }{{1, 1, 100}, {8, 8, 100}, {64, 32, 1024}} {
		b.Run(fmt.Sprintf("rules%d_conditions%d_items%d", size.rules, size.conditions, size.items), func(b *testing.B) {
			values := make([]any, size.items)
			for i := range values {
				values[i] = fmt.Sprintf("product:%04d", i)
			}
			flag := Flag{Default: false}
			for r := range size.rules {
				rule := Rule{Value: true}
				for n := range size.conditions {
					operand := values[(r*(size.conditions-1)+n)%size.items]
					if n == size.conditions-1 {
						operand = fmt.Sprintf("missing:%04d", r)
					}
					rule.Conditions = append(rule.Conditions, Condition{Attribute: "products", Operator: OpContains, Value: operand})
				}
				flag.Rules = append(flag.Rules, rule)
			}
			source, err := NewMemorySource(Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": flag}})
			if err != nil {
				b.Fatal(err)
			}
			client, err := Open(context.Background(), source)
			if err != nil {
				b.Fatal(err)
			}
			scope := New(client.Snapshot()).For(Target{Attributes: map[string]any{"products": values}})
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if got := scope.Evaluate(context.Background(), "flag", true); got.Err != nil || got.Value {
					b.Fatal(got)
				}
			}
		})
	}
}
