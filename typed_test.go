package feature

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// resultEvaluator controls portable typed results while retaining the complete native interface.
type resultEvaluator struct {
	*Client
	result Result[string]
}

// ResolveString exposes untrusted custom evaluator results to the facade's normalization policy.
func (e resultEvaluator) ResolveString(context.Context, string, string, Target) Result[string] {
	return e.result
}

// TestTypedPortableResultPolicy rejects malformed provider values and redacts custom errors.
func TestTypedPortableResultPolicy(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		result Result[string]
		want   error
		reason Reason
	}{
		{Result[string]{Value: "ok"}, nil, ReasonUnknown},
		{Result[string]{Value: "ok", Reason: ReasonDisabled}, nil, ReasonDisabled},
		{Result[string]{Value: string([]byte{255})}, ErrInvalidValue, ReasonError},
		{Result[string]{Err: ErrNotFound}, ErrNotFound, ReasonMissing},
		{Result[string]{Err: errors.New("private provider text")}, ErrUnavailable, ReasonError},
		{Result[string]{Err: ErrUnsupported}, ErrUnsupported, ReasonError},
		{Result[string]{Err: ErrInvalidValue}, ErrInvalidValue, ReasonError},
	} {
		d := New(resultEvaluator{result: tc.result}).String(ctx, "x", "fallback")
		if d.Err != tc.want || d.Reason != tc.reason || tc.want != nil && d.Value != "fallback" {
			t.Fatal(d)
		}
	}
}

// typedDocument exercises the same rules and snapshots for every supported value type.
func typedDocument() Document {
	return Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{
		"bool":     {Default: true},
		"string":   {Default: "classic", Rules: []Rule{{Subjects: []string{"early"}, Value: "modern"}}},
		"int":      {Default: int64(9007199254740993)},
		"float":    {Type: TypeFloat, Default: 1.5},
		"object":   {Default: map[string]any{"nested": []any{"blue", int64(3), nil}}},
		"disabled": {Default: "on", Disabled: true, Off: "off"},
	}}
}

// TestTypedNativeReads preserves exact values, owned objects, snapshots, and classified failures.
func TestTypedNativeReads(t *testing.T) {
	ctx := context.Background()
	source, err := NewMemorySource(typedDocument())
	if err != nil {
		t.Fatal(err)
	}
	client, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	flags := New(client)
	scope := flags.For(Target{Key: "early"})
	if !flags.Enabled(ctx, "bool") || flags.String(ctx, "string", "fallback").Value != "classic" || scope.String(ctx, "string", "fallback").Value != "modern" {
		t.Fatal("typed rule changed")
	}
	if flags.Int(ctx, "int", 0).Value != 9007199254740993 || scope.Int(ctx, "int", 0).Value != 9007199254740993 {
		t.Fatal("integer precision lost")
	}
	if flags.Float(ctx, "float", 0).Value != 1.5 || scope.Float(ctx, "float", 0).Value != 1.5 {
		t.Fatal("float lost")
	}
	object := flags.Object(ctx, "object", map[string]any{})
	if object.Err != nil {
		t.Fatal(object)
	}
	object.Value.(map[string]any)["nested"].([]any)[0] = "mutated"
	if scope.Object(ctx, "object", map[string]any{}).Value.(map[string]any)["nested"].([]any)[0] != "blue" {
		t.Fatal("snapshot exposed mutable value")
	}
	if d := scope.String(ctx, "disabled", "fallback"); d.Value != "off" || d.Reason != ReasonDisabled {
		t.Fatal(d)
	}
	if d := flags.String(ctx, "int", "fallback"); d.Value != "fallback" || d.Err != ErrTypeMismatch {
		t.Fatal(d)
	}
	if d := flags.Int(ctx, "absent", 12); d.Value != 12 || d.Err != ErrNotFound || d.Reason != ReasonMissing {
		t.Fatal(d)
	}
	if d := flags.Evaluate(ctx, "string", true); !d.Value || d.Err != ErrTypeMismatch {
		t.Fatal(d)
	}
	view := client.Snapshot()
	doc := typedDocument()
	doc.Flags["string"] = Flag{Default: "changed"}
	if err := source.Replace(doc); err != nil {
		t.Fatal(err)
	}
	if err := client.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if view.String(ctx, "string", "fallback", Subject{}).Value != "classic" || flags.String(ctx, "string", "fallback").Value != "changed" {
		t.Fatal("typed view was not immutable")
	}
	if view.Int(ctx, "int", 0, Subject{}).Value != 9007199254740993 || view.Float(ctx, "float", 0, Subject{}).Value != 1.5 || view.Object(ctx, "object", map[string]any{}, Subject{}).Err != nil {
		t.Fatal("typed view failed")
	}
	for _, input := range []Document{typedDocument(), doc} {
		state, err := compile(input)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(state.document)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := Decode(data)
		if err != nil {
			t.Fatal(string(data), err)
		}
		next, err := compile(decoded)
		if err != nil || next.revision != state.revision {
			t.Fatal("typed round trip changed revision", err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, target := range []Target{{}, {Attributes: map[string]any{"bad": func() {}}}} {
		if client.ResolveString(canceled, "string", "fallback", target).Err != context.Canceled || client.ResolveInt(canceled, "int", 1, target).Err != context.Canceled || client.ResolveFloat(canceled, "float", 1, target).Err != context.Canceled || client.ResolveObject(canceled, "object", map[string]any{}, target).Err != context.Canceled {
			t.Fatal("cancellation precedence changed")
		}
	}
	invalid := Target{Key: strings.Repeat("x", MaxStringBytes+1)}
	if client.ResolveString(ctx, "string", "f", invalid).Err != ErrInvalidSubject || client.ResolveInt(ctx, "int", 1, invalid).Err != ErrInvalidSubject || client.ResolveFloat(ctx, "float", 1, invalid).Err != ErrInvalidSubject || client.ResolveObject(ctx, "object", map[string]any{}, invalid).Err != ErrInvalidSubject {
		t.Fatal("invalid targeting accepted")
	}
	if flags.String(ctx, "", "fallback").Err != ErrInvalidKey || flags.String(canceled, "string", "fallback").Err != context.Canceled || flags.For(invalid).String(ctx, "string", "fallback").Err != ErrInvalidSubject {
		t.Fatal("portable validation changed")
	}
	if flags.Float(ctx, "float", math.NaN()).Err != ErrInvalidValue || flags.Object(ctx, "object", nil).Err != ErrInvalidValue {
		t.Fatal("invalid fallback accepted")
	}
	if view.String(ctx, "string", "f", Subject{Attributes: map[string]string{"bad key": "x"}}).Err != ErrInvalidSubject {
		t.Fatal("native subject accepted")
	}
	if view.Float(ctx, "float", math.Inf(1), Subject{}).Err != ErrInvalidValue {
		t.Fatal("invalid native fallback accepted")
	}
	if New(evaluatorFunc(func(context.Context, string, bool, Target) Decision { return Decision{} })).String(ctx, "string", "f").Err != ErrUnsupported {
		t.Fatal("boolean-only evaluator gained typed capability")
	}
}

// TestValueBounds rejects unsafe data before publication and owns every supported container.
func TestValueBounds(t *testing.T) {
	values := []any{nil, true, int(1), int8(1), int16(1), int32(1), int64(1), uint(1), uint8(1), uint16(1), uint32(1), uint64(1), float32(1.5), float64(1.5), json.Number("12"), json.Number("1.5"), "text", map[string]any{"x": []any{nil, true}}, []any{"x"}}
	for _, value := range values {
		if _, err := CloneValue(value); err != nil {
			t.Fatal(value, err)
		}
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	bad := []any{func() {}, uint64(math.MaxUint64), uint(math.MaxUint64), math.NaN(), math.Inf(1), float32(math.Inf(1)), json.Number("bad"), strings.Repeat("x", MaxValueBytes), string([]byte{255}), map[string]any{string([]byte{255}): true}, []any{func() {}}, cycle, make([]any, MaxSubjects+1)}
	many := map[string]any{}
	for i := 0; i <= MaxSubjects; i++ {
		many[fmt.Sprint(i)] = true
	}
	bad = append(bad, many, map[string]any{"x": func() {}})
	for i, value := range bad {
		if _, err := CloneValue(value); err != ErrInvalidValue {
			t.Fatalf("%d: %v", i, err)
		}
	}
	if _, err := CopyValue(int64(1<<53+1), TypeFloat); err != ErrInvalidValue {
		t.Fatal("lossy float conversion accepted", err)
	}
	if value, err := CopyValue(int64(12), TypeFloat); err != nil || value != float64(12) {
		t.Fatal(value, err)
	}
	if _, err := CopyValue(json.Number("bad"), TypeFloat); err != ErrInvalidValue {
		t.Fatal(err)
	}
	if _, err := CopyValue("x", TypeInteger); err != ErrTypeMismatch {
		t.Fatal(err)
	}
	if value, err := CloneValue(map[string]any{}); err != nil || !reflect.DeepEqual(value, map[string]any{}) {
		t.Fatal(value, err)
	}
}

// TestTypedDefinitionsRejectInvalidValues preserves schema 1 and rejects inconsistent typed rules.
func TestTypedDefinitionsRejectInvalidValues(t *testing.T) {
	if _, err := Encode(Document{}); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if _, err := NewMemorySource(Document{Schema: 1, Flags: map[string]Flag{"zero": {Rules: []Rule{{Subjects: []string{"a"}}}}}}); err != nil {
		t.Fatal(err)
	}
	if subject, err := nativeSubject(Target{Attributes: map[string]any{"plan": "pro", "paid": true}}); err != nil || subject.Attributes["plan"] != "pro" {
		t.Fatal(subject, err)
	}
	oversized := Document{Schema: 2, Flags: map[string]Flag{}}
	for i := 0; i < 32; i++ {
		oversized.Flags[fmt.Sprint(i)] = Flag{Default: strings.Repeat("x", MaxValueBytes/2)}
	}
	if _, err := NewMemorySource(oversized); err != ErrInvalidDocument {
		t.Fatal("aggregate value budget bypassed", err)
	}
	cases := []Flag{{Default: func() {}}, {Default: "x", Type: TypeInteger}, {Default: "x", Off: 1}, {Default: "x", Disabled: true}, {Default: "x", Rules: []Rule{{Subjects: []string{"a"}, Value: 12}}}, {Default: true, Type: "unknown"}}
	for _, flag := range cases {
		if _, err := NewMemorySource(Document{Schema: 2, Flags: map[string]Flag{"x": flag}}); err != ErrInvalidDocument {
			t.Fatal(flag, err)
		}
	}
	for _, flag := range []Flag{{Default: "x"}, {Default: true, Type: TypeBoolean}, {Default: true, Off: false}} {
		if _, err := NewMemorySource(Document{Schema: 1, Flags: map[string]Flag{"x": flag}}); err != ErrInvalidDocument {
			t.Fatal(flag, err)
		}
	}
	for _, input := range []string{
		`{"schema":2,"flags":{"x":{"default":null}}}`,
		`{"schema":2,"flags":{"x":{"default":true,"rules":[{"subjects":["a"],"value":null}]}}}`,
		`{"schema":2,"flags":{"x":{"type":"object","default":{"x":null,"x":true}}}}`,
		`{"schema":2,"flags":{"x":{"type":"string","default":"x","rules":[{"subjects":["a"],"value":true}]}}}`,
		`{"schema":2,"flags":{"x":{"default":"x","off":null}}}`,
	} {
		if _, err := Decode([]byte(input)); err != ErrInvalidDocument {
			t.Fatal(input, err)
		}
	}
}

// TestTypedJSONStructuralBounds checks nested value limits before typed allocation.
func TestTypedJSONStructuralBounds(t *testing.T) {
	for _, value := range []any{make([]any, MaxSubjects+1), func() any {
		m := map[string]any{}
		for i := 0; i <= MaxSubjects; i++ {
			m[fmt.Sprint(i)] = true
		}
		return m
	}()} {
		data, err := json.Marshal(map[string]any{"schema": 2, "flags": map[string]any{"x": map[string]any{"default": value}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(data); err != ErrInvalidDocument {
			t.Fatal(err)
		}
	}
	deep := strings.Repeat("[", 20) + "true" + strings.Repeat("]", 20)
	if _, err := Decode([]byte(`{"schema":2,"flags":{"x":{"default":` + deep + `}}}`)); err != ErrInvalidDocument {
		t.Fatal(err)
	}
}
