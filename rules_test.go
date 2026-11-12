package feature

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// TestSchemaOneEncodingCompatibility freezes the pre-typed canonical bytes and revision.
func TestSchemaOneEncodingCompatibility(t *testing.T) {
	doc := Document{Schema: 1, Flags: map[string]Flag{"checkout": {Rules: []Rule{{Subjects: []string{"tenant:b", "tenant:a"}, Value: true}}}}}
	const previous = `{"schema":1,"flags":{"checkout":{"default":false,"rules":[{"subjects":["tenant:a","tenant:b"],"value":true}]}}}`
	data, err := Encode(doc)
	if err != nil || string(data) != previous {
		t.Fatal(string(data), err)
	}
	state, err := compile(doc)
	if err != nil || state.revision != fmt.Sprintf("%x", sha256.Sum256([]byte(previous))) {
		t.Fatal("schema 1 revision changed", err)
	}
}

// TestTypedConditions evaluates bounded comparisons through the public portable API.
func TestTypedConditions(t *testing.T) {
	cases := []struct {
		name      string
		condition Condition
		target    Target
		want      bool
	}{
		{"equal", Condition{"paid", OpEqual, true, nil}, Target{Attributes: map[string]any{"paid": true}}, true},
		{"not-equal", Condition{"plan", OpNotEqual, "free", nil}, Target{Attributes: map[string]any{"plan": "pro"}}, true},
		{"not-equal-missing", Condition{"plan", OpNotEqual, "free", nil}, Target{}, false},
		{"in", Condition{"plan", OpIn, []string{"pro", "enterprise"}, nil}, Target{Attributes: map[string]any{"plan": "pro"}}, true},
		{"not-in", Condition{"plan", OpNotIn, []any{"free"}, nil}, Target{Attributes: map[string]any{"plan": "pro"}}, true},
		{"in-miss", Condition{"plan", OpIn, []any{"free"}, nil}, Target{Attributes: map[string]any{"plan": "pro"}}, false},
		{"greater", Condition{"seats", OpGreater, 10, nil}, Target{Attributes: map[string]any{"seats": 12}}, true},
		{"greater-or-equal", Condition{"seats", OpGreaterOrEqual, 12.0, nil}, Target{Attributes: map[string]any{"seats": int64(12)}}, true},
		{"less", Condition{"seats", OpLess, 20, nil}, Target{Attributes: map[string]any{"seats": 12.5}}, true},
		{"less-or-equal", Condition{"seats", OpLessOrEqual, 12, nil}, Target{Attributes: map[string]any{"seats": 12}}, true},
		{"numeric-type-mismatch", Condition{"seats", OpGreater, 10, nil}, Target{Attributes: map[string]any{"seats": "12"}}, false},
		{"numeric-equality-type-mismatch", Condition{"seats", OpEqual, "12", nil}, Target{Attributes: map[string]any{"seats": 12}}, false},
		{"integer-precision", Condition{"seats", OpGreater, float64(9007199254740992), nil}, Target{Attributes: map[string]any{"seats": int64(9007199254740993)}}, true},
		{"integer-inequality", Condition{"seats", OpEqual, float64(9007199254740992), nil}, Target{Attributes: map[string]any{"seats": int64(9007199254740993)}}, false},
		{"contains-string", Condition{"plan", OpContains, "pro", nil}, Target{Attributes: map[string]any{"plan": "pro-annual"}}, true},
		{"contains-array", Condition{"roles", OpContains, "admin", nil}, Target{Attributes: map[string]any{"roles": []string{"admin", "viewer"}}}, true},
		{"contains-array-miss", Condition{"roles", OpContains, "owner", nil}, Target{Attributes: map[string]any{"roles": []any{"viewer"}}}, false},
		{"prefix", Condition{"plan", OpPrefix, "pro-", nil}, Target{Attributes: map[string]any{"plan": "pro-annual"}}, true},
		{"suffix", Condition{"plan", OpSuffix, "-annual", nil}, Target{Attributes: map[string]any{"plan": "pro-annual"}}, true},
		{"regex", Condition{"plan", OpMatches, "^pro-[a-z]+$", nil}, Target{Attributes: map[string]any{"plan": "pro-annual"}}, true},
		{"regex-type", Condition{"plan", OpMatches, "^pro", nil}, Target{Attributes: map[string]any{"plan": 12}}, false},
		{"exists-null", Condition{"value", OpExists, nil, nil}, Target{Attributes: map[string]any{"value": nil}}, true},
		{"missing", Condition{"value", OpMissing, nil, nil}, Target{}, true},
		{"equal-null", Condition{"value", OpEqual, nil, nil}, Target{Attributes: map[string]any{"value": nil}}, true},
		{"nested", Condition{"/organization/plan", OpEqual, "pro", nil}, Target{Attributes: map[string]any{"organization": map[string]string{"plan": "pro"}}}, true},
		{"array-index", Condition{"/roles/0", OpEqual, "admin", nil}, Target{Attributes: map[string]any{"roles": []any{"admin"}}}, true},
		{"string-array-index", Condition{"/roles/0", OpEqual, "admin", nil}, Target{Attributes: map[string]any{"roles": []string{"admin"}}}, true},
		{"string-array-missing", Condition{"/roles/2", OpEqual, "admin", nil}, Target{Attributes: map[string]any{"roles": []string{"admin"}}}, false},
		{"escaped-path", Condition{"/organization/a~1b~0c", OpEqual, "pro", nil}, Target{Attributes: map[string]any{"organization": map[string]any{"a/b~c": "pro"}}}, true},
		{"invalid-array-index", Condition{"/roles/00", OpEqual, "admin", nil}, Target{Attributes: map[string]any{"roles": []any{"admin"}}}, false},
		{"outside-array", Condition{"/roles/5", OpEqual, "admin", nil}, Target{Attributes: map[string]any{"roles": []any{"admin"}}}, false},
		{"scalar-path", Condition{"/organization/plan", OpEqual, "pro", nil}, Target{Attributes: map[string]any{"organization": "pro"}}, false},
		{"missing-path", Condition{"/organization/plan", OpEqual, "pro", nil}, Target{Attributes: map[string]any{"organization": map[string]any{}}}, false},
		{"key", Condition{"$key", OpEqual, "tenant:acme", nil}, Target{Key: "tenant:acme"}, true},
		{"kind", Condition{"$kind", OpEqual, "organization", nil}, Target{Kind: "organization"}, true},
		{"unsigned-full-range", Condition{"seats", OpGreater, 10, nil}, Target{Attributes: map[string]any{"seats": uint64(math.MaxUint64)}}, true},
		{"nonnumeric-time", Condition{"seats", OpGreater, 10, nil}, Target{Attributes: map[string]any{"seats": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}, false},
		{"structured-equality", Condition{"organization", OpEqual, map[string]any{"plan": "pro"}, nil}, Target{Attributes: map[string]any{"organization": map[string]any{"plan": "pro"}}}, true},
		{"structured-numeric-equality", Condition{"organization", OpEqual, map[string]any{"seats": []any{12}}, nil}, Target{Attributes: map[string]any{"organization": map[string]any{"seats": []any{12.0}}}}, true},
		{"structured-length-mismatch", Condition{"organization", OpEqual, map[string]any{"plan": "pro"}, nil}, Target{Attributes: map[string]any{"organization": map[string]any{}}}, false},
		{"structured-key-mismatch", Condition{"organization", OpEqual, map[string]any{"plan": nil}, nil}, Target{Attributes: map[string]any{"organization": map[string]any{"other": nil}}}, false},
		{"array-length-mismatch", Condition{"roles", OpEqual, []any{"admin"}, nil}, Target{Attributes: map[string]any{"roles": []any{}}}, false},
		{"array-element-mismatch", Condition{"roles", OpEqual, []any{"admin"}, nil}, Target{Attributes: map[string]any{"roles": []any{"viewer"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Document{Schema: 2, Flags: map[string]Flag{"flag": {Default: false, Rules: []Rule{{Conditions: []Condition{tc.condition}, Value: true}}}}}
			source, err := NewMemorySource(doc)
			if err != nil {
				t.Fatal(err)
			}
			engine, err := Open(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			decision := New(engine).For(tc.target).Evaluate(context.Background(), "flag", false)
			if decision.Err != nil || decision.Value != tc.want {
				t.Fatal(decision)
			}
			encoded, err := Encode(doc)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatal(string(encoded), err)
			}
			before, _ := compile(doc)
			after, _ := compile(decoded)
			if before.revision != after.revision {
				t.Fatal("condition round trip changed revision")
			}
		})
	}
	if value, present := subjectValue(Subject{Attributes: map[string]string{"plan": "pro"}}, "plan"); !present || value != "pro" {
		t.Fatal(value, present)
	}
	if value, present := subjectValue(Subject{Attributes: map[string]string{"plan": "pro"}}, "/plan"); !present || value != "pro" {
		t.Fatal(value, present)
	}
	if _, present := subjectValue(Subject{Attributes: map[string]string{}}, "/missing"); present {
		t.Fatal("missing legacy path present")
	}
}

// TestScheduledAndProgressiveRules fixes calendar eligibility for a captured request view.
func TestScheduledAndProgressiveRules(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	now := start.Add(-time.Second)
	doc := Document{Schema: 2, Flags: map[string]Flag{
		"window":   {Default: false, Rules: []Rule{{Start: &start, End: &end, Value: true}}},
		"progress": {Default: false, Rules: []Rule{{Rollout: &Rollout{Salt: "stable", Progressive: &Progressive{Start: start, End: end, From: 0, To: 10000}}, Value: true}}},
	}}
	source, err := NewMemorySource(doc)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := Open(ctx, source, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	flags := New(engine).For(Target{Key: "tenant:acme"})
	if flags.Enabled(ctx, "window") || flags.Enabled(ctx, "progress") {
		t.Fatal("scheduled rule started early")
	}
	now = start
	if !flags.Enabled(ctx, "window") || flags.Enabled(ctx, "progress") {
		t.Fatal("start boundary changed")
	}
	snapshot := New(engine.Snapshot()).For(Target{Key: "tenant:acme"})
	now = start.Add(30 * time.Minute)
	if d := flags.Evaluate(ctx, "progress", true); d.Value != (bucket("progress", "stable", "tenant:acme") < 5000) || d.Err != nil {
		t.Fatal(d)
	}
	now = end
	if flags.Enabled(ctx, "window") || !flags.Enabled(ctx, "progress") || !snapshot.Enabled(ctx, "window") || snapshot.Enabled(ctx, "progress") {
		t.Fatal("calendar snapshot was not fixed")
	}
	if progressiveBasis(Progressive{Start: start, End: end, From: 10000, To: 0}, start.Add(15*time.Minute)) != 7500 {
		t.Fatal("reverse rollout interpolation failed")
	}
	encoded, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatal(string(encoded), err)
	}
	opening := start
	*doc.Flags["window"].Rules[0].Start = end.Add(time.Hour)
	now = opening.Add(time.Minute)
	if err := engine.Refresh(ctx); err != nil || !flags.Enabled(ctx, "window") {
		t.Fatal("source borrowed schedule pointers", err)
	}
}

// TestProgressiveExactBoundary preserves eligibility at an exact basis-point transition.
func TestProgressiveExactBoundary(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	doc := Document{Schema: 2, Flags: map[string]Flag{
		"flag": {Default: false, Rules: []Rule{{Value: true, Rollout: &Rollout{Salt: "salt", Progressive: &Progressive{Start: start, End: start.Add(time.Hour), From: 0, To: 10000}}}}},
	}}
	engine, err := Open(ctx, mustMemorySource(t, doc), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	flags := New(engine).For(Target{Key: "probe-3197"})
	for _, tc := range []struct {
		elapsed time.Duration
		want    bool
	}{
		{1080*time.Millisecond - time.Nanosecond, false},
		{1080 * time.Millisecond, true},
		{1080*time.Millisecond + time.Nanosecond, true},
	} {
		now = start.Add(tc.elapsed)
		decision := flags.Evaluate(ctx, "flag", false)
		if decision.Err != nil || decision.Value != tc.want {
			t.Fatalf("at %s: got %+v, want %t", tc.elapsed, decision, tc.want)
		}
	}
}

// TestProgressiveBasisExactValues freezes integer interpolation and signed truncation.
func TestProgressiveBasisExactValues(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const maxDuration = time.Duration(1<<63 - 1)
	for _, tc := range []struct {
		name     string
		duration time.Duration
		elapsed  time.Duration
		from     int
		to       int
		want     int
	}{
		{"before-start", time.Hour, -time.Nanosecond, 2000, 8000, 2000},
		{"at-start", time.Hour, 0, 2000, 8000, 2000},
		{"at-end", time.Hour, time.Hour, 2000, 8000, 8000},
		{"after-end", time.Hour, time.Hour + time.Nanosecond, 2000, 8000, 8000},
		{"increasing-before-boundary", time.Hour, 1080*time.Millisecond - time.Nanosecond, 0, 10000, 2},
		{"increasing-at-boundary", time.Hour, 1080 * time.Millisecond, 0, 10000, 3},
		{"increasing-after-boundary", time.Hour, 1080*time.Millisecond + time.Nanosecond, 0, 10000, 3},
		{"decreasing-before-boundary", time.Hour, 1080*time.Millisecond - time.Nanosecond, 10000, 0, 9998},
		{"decreasing-at-boundary", time.Hour, 1080 * time.Millisecond, 10000, 0, 9997},
		{"decreasing-after-boundary", time.Hour, 1080*time.Millisecond + time.Nanosecond, 10000, 0, 9997},
		{"unchanged", time.Hour, time.Minute, 4200, 4200, 4200},
		{"long-increasing-midpoint", maxDuration, maxDuration / 2, 0, 10000, 4999},
		{"long-decreasing-midpoint", maxDuration, maxDuration / 2, 10000, 0, 5001},
		{"long-last-nanosecond", maxDuration, maxDuration - time.Nanosecond, 0, 10000, 9999},
		{"nanosecond-increasing-first", 3 * time.Nanosecond, time.Nanosecond, 0, 10000, 3333},
		{"nanosecond-increasing-last", 3 * time.Nanosecond, 2 * time.Nanosecond, 0, 10000, 6666},
		{"nanosecond-decreasing-first", 3 * time.Nanosecond, time.Nanosecond, 10000, 0, 6667},
		{"nanosecond-decreasing-last", 3 * time.Nanosecond, 2 * time.Nanosecond, 10000, 0, 3334},
		{"nonzero-increasing", 3 * time.Nanosecond, time.Nanosecond, 2000, 8000, 4000},
		{"nonzero-decreasing", 3 * time.Nanosecond, time.Nanosecond, 8000, 2000, 6000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			progress := Progressive{Start: start, End: start.Add(tc.duration), From: tc.from, To: tc.to}
			if got := progressiveBasis(progress, start.Add(tc.elapsed)); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// TestAdvancedDefinitionBudgets bounds aggregate payloads before whole-document ownership.
func TestAdvancedDefinitionBudgets(t *testing.T) {
	large := strings.Repeat("x", MaxValueBytes/2)
	rules := make([]Rule, MaxRules)
	for i := range rules {
		rules[i] = Rule{Subjects: []string{"tenant"}, Value: large}
	}
	variants := map[string]any{}
	for i := 0; i < MaxAttributes; i++ {
		variants[fmt.Sprint(i)] = large
	}
	conditions := make([]Condition, 8)
	for i := range conditions {
		conditions[i] = Condition{Attribute: "organization", Operator: OpEqual, Value: large}
	}
	subjects := make([]string, 170)
	for i := range subjects {
		subjects[i] = strings.Repeat("x", MaxStringBytes)
	}
	for _, flag := range []Flag{
		{Default: "base", Rules: rules},
		{Default: large, Variants: variants},
		{Default: false, Rules: []Rule{{Conditions: conditions, Value: true}}},
		{Default: false, Rules: []Rule{{Subjects: subjects, Rollout: &Rollout{BasisPoints: 5000, Salt: strings.Repeat("x", MaxStringBytes)}, Value: true}}},
	} {
		if _, err := NewMemorySource(Document{Schema: 2, Flags: map[string]Flag{"x": flag}}); err != ErrInvalidDocument {
			t.Fatal("aggregate budget accepted", err)
		}
	}
}

// sourceClient constructs a native client for ownership checks.
func sourceClient(t *testing.T, source Source) *Client {
	t.Helper()
	client, err := Open(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// TestNamedVariantsAndSplits preserves explicit allocation order, off-values, and result labels.
func TestNamedVariantsAndSplits(t *testing.T) {
	ctx := context.Background()
	doc := Document{Schema: 2, Flags: map[string]Flag{
		"theme": {Variants: map[string]any{"control": "blue", "treatment": "green"}, DefaultVariant: "control", Rules: []Rule{{Subjects: []string{"early"}, Variant: "treatment"}}},
		"split": {Variants: map[string]any{"control": "blue", "treatment": "green"}, DefaultVariant: "control", Rules: []Rule{{Split: &Split{Salt: "v1", Allocations: []Allocation{{"control", 5000}, {"treatment", 5000}}}}}},
		"off":   {Variants: map[string]any{"on": "green", "off": "blue"}, DefaultVariant: "on", OffVariant: "off", Disabled: true},
	}}
	engine := sourceClient(t, mustMemorySource(t, doc))
	flags := New(engine)
	if d := flags.String(ctx, "theme", "f"); d.Value != "blue" || d.Variant != "control" {
		t.Fatal(d)
	}
	if d := flags.For(Target{Key: "early"}).String(ctx, "theme", "f"); d.Value != "green" || d.Variant != "treatment" {
		t.Fatal(d)
	}
	if d := flags.String(ctx, "off", "f"); d.Value != "blue" || d.Variant != "off" || d.Reason != ReasonDisabled {
		t.Fatal(d)
	}
	if d := flags.String(ctx, "split", "f"); d.Variant != "control" || d.Reason != ReasonDefault {
		t.Fatal("anonymous split was eligible", d)
	}
	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		key := fmt.Sprint(i)
		scope := flags.For(Target{Key: key})
		before := scope.String(ctx, "split", "f")
		after := scope.String(ctx, "split", "f")
		if before != after || before.Err != nil || before.Reason != ReasonSplit {
			t.Fatal(before, after)
		}
		if before.Value != doc.Flags["split"].Variants[before.Variant] {
			t.Fatal(before)
		}
		counts[before.Variant]++
	}
	if counts["control"] == 0 || counts["treatment"] == 0 {
		t.Fatal(counts)
	}
	encoded, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(string(encoded), err)
	}
	before, _ := compile(doc)
	after, _ := compile(decoded)
	if before.revision != after.revision {
		t.Fatal("variant round trip changed revision")
	}
	doc.Flags["theme"].Variants["control"] = "mutated"
	if flags.String(ctx, "theme", "f").Value != "blue" {
		t.Fatal("variant map was borrowed")
	}
}

// mustMemorySource fails the owning test instead of masking invalid fixture wiring.
func mustMemorySource(t *testing.T, doc Document) *MemorySource {
	t.Helper()
	source, err := NewMemorySource(doc)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// TestAdvancedDefinitionValidation rejects invalid conditions, references, schedules, and allocations.
func TestAdvancedDefinitionValidation(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	zero := time.Time{}
	conditions := []Condition{{Attribute: "", Operator: OpEqual}, {Attribute: "/bad~", Operator: OpEqual}, {Attribute: "/bad~2", Operator: OpEqual}, {Attribute: "x", Operator: "unknown"}, {Attribute: "x", Operator: OpEqual, Value: func() {}}, {Attribute: "x", Operator: OpExists, Value: true}, {Attribute: "x", Operator: OpIn, Value: "wrong"}, {Attribute: "x", Operator: OpIn, Value: []any{}}, {Attribute: "x", Operator: OpGreater, Value: "wrong"}, {Attribute: "x", Operator: OpPrefix, Value: true}, {Attribute: "x", Operator: OpMatches, Value: "["}}
	for _, condition := range conditions {
		if _, err := NewMemorySource(Document{Schema: 2, Flags: map[string]Flag{"x": {Rules: []Rule{{Conditions: []Condition{condition}, Value: true}}}}}); err != ErrInvalidDocument {
			t.Fatal(condition, err)
		}
	}
	flags := []Flag{
		{Default: true, Variants: map[string]any{}},
		{Variants: map[string]any{"a": "x"}, DefaultVariant: "missing"},
		{Default: "contradiction", Variants: map[string]any{"a": "x"}, DefaultVariant: "a"},
		{Default: func() {}, Variants: map[string]any{"a": "x"}, DefaultVariant: "a"},
		{Variants: map[string]any{"a": "x", "bad/key": "y"}, DefaultVariant: "a"},
		{Variants: map[string]any{"a": "x", "b": 12}, DefaultVariant: "a"},
		{Default: "x", OffVariant: "missing"},
		{Default: true, Rules: []Rule{{Start: &zero, Value: true}}},
		{Default: true, Rules: []Rule{{Start: &end, End: &start, Value: true}}},
		{Default: true, Rules: []Rule{{End: &zero, Value: true}}},
		{Default: true, Rules: []Rule{{Split: &Split{Salt: "x"}}}},
		{Default: true, Rules: []Rule{{Split: &Split{Salt: "x", Allocations: []Allocation{{"missing", 10000}}}}}},
		{Default: true, Variants: map[string]any{"a": true}, Rules: []Rule{{Split: &Split{Salt: "x", Allocations: []Allocation{{"a", 9000}}}}}},
		{Default: true, Variants: map[string]any{"a": true}, Rules: []Rule{{Split: &Split{Salt: "x", Allocations: []Allocation{{"a", 5000}, {"a", 5000}}}}}},
		{Default: true, Variants: map[string]any{"a": true}, Rules: []Rule{{Split: &Split{Salt: "x", Allocations: []Allocation{{"a", 10000}}}, Value: true}}},
		{Default: true, Rules: []Rule{{Rollout: &Rollout{Salt: "x", Progressive: &Progressive{Start: end, End: start, From: 0, To: 10000}}, Value: true}}},
	}
	for i, flag := range flags {
		if _, err := NewMemorySource(Document{Schema: 2, Flags: map[string]Flag{"x": flag}}); err != ErrInvalidDocument {
			t.Fatal(i, err)
		}
	}
	for _, rule := range []Rule{{Conditions: []Condition{{Attribute: "x", Operator: OpEqual, Value: true}}, Value: true}, {Start: &start, Value: true}, {Split: &Split{Salt: "x"}}, {Rollout: &Rollout{Salt: "x", Progressive: &Progressive{Start: start, End: end}}, Value: true}} {
		if _, err := NewMemorySource(Document{Schema: 1, Flags: map[string]Flag{"x": {Rules: []Rule{rule}}}}); err != ErrInvalidDocument {
			t.Fatal(rule, err)
		}
	}
	invalidJSON := []string{
		`{"schema":2,"flags":{"x":{"variants":{"a":true},"default_variant":"a","rules":[{"split":{"salt":"x","allocations":[{"variant":"a"}]}}]}}}`,
		`{"schema":2,"flags":{"x":{"default":false,"rules":[{"rollout":{"salt":"x","progressive":{"start":"2026-01-01T00:00:00Z","end":"2026-01-02T00:00:00Z","to":10000}},"value":true}]}}}`,
		`{"schema":2,"flags":{"x":{"default":false,"rules":[{"conditions":[{"attribute":"x","op":"eq","typo":true}],"value":true}]}}}`,
		`{"schema":2,"flags":{"x":{"default":false,"rules":[{"split":{"salt":"x","typo":true}}]}}}`,
		`{"schema":2,"flags":{"x":{"default":false,"rules":[{"rollout":{"salt":"x","progressive":{"typo":true}},"value":true}]}}}`,
	}
	for _, input := range invalidJSON {
		if _, err := Decode([]byte(input)); err != ErrInvalidDocument {
			t.Fatal(input, err)
		}
	}
	if _, err := CloneValue(json.Number(strings.Repeat("1", MaxValueBytes))); err != ErrInvalidValue {
		t.Fatal(err)
	}
	if _, err := CloneValue(json.Number("1e999")); err != ErrInvalidValue {
		t.Fatal(err)
	}
}

// TestSplitGoldenAssignments freezes ordered allocation boundaries through the native application API.
func TestSplitGoldenAssignments(t *testing.T) {
	cases := []struct {
		name        string
		allocations []Allocation
	}{
		{"unequal-three-way", []Allocation{{"first", 1000}, {"second", 2500}, {"third", 6500}}},
		{"zero-width-allocations", []Allocation{{"leading", 0}, {"first", 1000}, {"middle", 0}, {"second", 2500}, {"third", 6500}, {"trailing", 0}}},
	}
	// These identity assignments are fixed vectors for flag "split" and salt "v1".
	identities := []struct {
		key, variant string
		bucket       int
	}{
		{"125", "first", 0},
		{"10122", "first", 999},
		{"1089", "second", 1000},
		{"8582", "second", 3499},
		{"2485", "third", 3500},
		{"14727", "third", 5000},
		{"3225", "third", 9999},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			doc := Document{Schema: 2, Flags: map[string]Flag{"split": {
				Variants:       map[string]any{"leading": "leading-value", "first": "first-value", "middle": "middle-value", "second": "second-value", "third": "third-value", "trailing": "trailing-value"},
				DefaultVariant: "leading",
				Rules:          []Rule{{Split: &Split{Salt: "v1", Allocations: tc.allocations}}},
			}}}
			engine := sourceClient(t, mustMemorySource(t, doc))
			for _, identity := range identities {
				t.Run(fmt.Sprintf("bucket-%d", identity.bucket), func(t *testing.T) {
					scope := New(engine).For(Target{Key: identity.key})
					before := scope.String(ctx, "split", "fallback")
					if before.Err != nil || before.Reason != ReasonSplit || before.Variant != identity.variant || before.Value != identity.variant+"-value" {
						t.Fatalf("identity %s: got %+v, want variant %s", identity.key, before, identity.variant)
					}
					if err := engine.Refresh(ctx); err != nil {
						t.Fatal(err)
					}
					if after := scope.String(ctx, "split", "fallback"); after != before {
						t.Fatalf("stable identity changed after refresh: before %+v, after %+v", before, after)
					}
				})
			}
		})
	}
}
