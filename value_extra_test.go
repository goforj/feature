package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestNumericConditionRevision publishes numerically different conditions even when ordinary JSON collides.
func TestNumericConditionRevision(t *testing.T) {
	ctx := context.Background()
	integer := int64(9223372036854775000)
	doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {
		Default: false, Rules: []Rule{{Value: true, Conditions: []Condition{{Attribute: "id", Operator: OpEqual, Value: integer}}}},
	}}}
	source := mustMemorySource(t, doc)
	engine := sourceClient(t, source)
	subscription, err := engine.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	<-subscription.Changes
	scope := New(engine).For(Target{Attributes: map[string]any{"id": integer}})
	before := scope.Evaluate(ctx, "flag", false)
	if before.Err != nil || !before.Value {
		t.Fatal(before)
	}
	doc.Flags["flag"].Rules[0].Conditions[0].Value = float64(integer)
	if err := source.Replace(doc); err != nil {
		t.Fatal(err)
	}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	after := scope.Evaluate(ctx, "flag", true)
	if after.Err != nil || after.Value || after.Revision == before.Revision {
		t.Fatalf("numeric condition change lost: before=%+v after=%+v", before, after)
	}
	select {
	case change := <-subscription.Changes:
		if change.Revision != after.Revision || !reflect.DeepEqual(change.Keys, []string{"flag"}) {
			t.Fatal(change)
		}
	default:
		t.Fatal("numeric condition change did not notify")
	}
}

// TestVariantNumericConflicts rejects distinct numeric values that ordinary JSON would conflate.
func TestVariantNumericConflicts(t *testing.T) {
	integer := int64(9223372036854775000)
	doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"config": {
		Default: map[string]any{"id": integer}, DefaultVariant: "selected",
		Variants: map[string]any{"selected": map[string]any{"id": float64(integer)}},
	}}}
	if _, err := Encode(doc); err != ErrInvalidDocument {
		t.Fatal("contradictory numeric variant accepted", err)
	}
}

// TestFloatEncodingBudget includes explicit float syntax in complete-document size accounting.
func TestFloatEncodingBudget(t *testing.T) {
	doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{}}
	for index := 0; index < 200; index++ {
		values := make([]any, MaxSubjects)
		for item := range values {
			values[item] = float64(0)
		}
		doc.Flags[fmt.Sprint(index)] = Flag{Default: values}
	}
	if encoded, err := Encode(doc); err != ErrInvalidDocument {
		t.Fatalf("oversized float document accepted: bytes=%d error=%v", len(encoded), err)
	}
	if _, err := NewMemorySource(doc); err != ErrInvalidDocument {
		t.Fatalf("oversized float snapshot accepted: %v", err)
	}
}

// TestNestedFloatRoundTrip preserves precision and revisions in every native value position.
func TestNestedFloatRoundTrip(t *testing.T) {
	value := map[string]any{"numbers": []any{float64(1 << 63), 1e20, int64(9223372036854775807)}}
	doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"config": {
		Default: value, Off: value, Variants: map[string]any{"alternate": value},
		Rules: []Rule{{Value: value, Conditions: []Condition{{Attribute: "number", Operator: OpEqual, Value: float64(1 << 63)}}}},
	}}}
	before, err := compile(doc)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	after, err := compile(decoded)
	if err != nil || before.revision != after.revision || !reflect.DeepEqual(before.document, after.document) {
		t.Fatalf("float document changed: %v", err)
	}
}

// TestIntegralFloatRoundTrip retains explicitly typed floats whose JSON encoding uses integer tokens.
func TestIntegralFloatRoundTrip(t *testing.T) {
	for _, value := range []float64{1 << 53, 1<<53 + 2, 1 << 63, 1e20, -1e20} {
		doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"value": {Type: TypeFloat, Default: value}}}
		encoded, err := Encode(doc)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("cannot decode %s: %v", encoded, err)
		}
		engine := sourceClient(t, mustMemorySource(t, decoded))
		if result := New(engine).Float(context.Background(), "value", 0); result.Err != nil || result.Value != value {
			t.Fatal(result)
		}
	}
}

// TestJSONIntegerOverflow rejects integer tokens rather than rounding them through float64.
func TestJSONIntegerOverflow(t *testing.T) {
	for _, number := range []string{"9223372036854775808", "9223372036854775809", "-9223372036854775809", "18446744073709551615"} {
		t.Run(number, func(t *testing.T) {
			for _, value := range []any{json.Number(number), map[string]any{"id": json.Number(number)}, []any{json.Number(number)}} {
				if got, err := CloneValue(value); err != ErrInvalidValue {
					t.Fatalf("overflow accepted: value=%v error=%v", got, err)
				}
			}
			data := []byte(`{"schema":2,"flags":{"config":{"default":{"id":` + number + `}}}}`)
			if _, err := Decode(data); err != ErrInvalidDocument {
				t.Fatalf("overflow document accepted: %v", err)
			}
		})
	}
	for _, number := range []string{"9223372036854775807", "-9223372036854775808"} {
		value, err := CloneValue(json.Number(number))
		if _, ok := value.(int64); err != nil || !ok || fmt.Sprint(value) != number {
			t.Fatalf("integer boundary changed: %v, %v", value, err)
		}
	}
}

// TestJSONFloatIntegerPrecision applies the same lossless coercion policy to Go and JSON integers.
func TestJSONFloatIntegerPrecision(t *testing.T) {
	for _, number := range []string{"9007199254740993", "-9007199254740993", "9223372036854775807"} {
		if got, err := CopyValue(json.Number(number), TypeFloat); err != ErrInvalidValue {
			t.Fatalf("lossy float accepted: %v, %v", got, err)
		}
		data := []byte(`{"schema":2,"flags":{"number":{"type":"float","default":` + number + `}}}`)
		if _, err := Decode(data); err != ErrInvalidDocument {
			t.Fatalf("lossy float document accepted: %v", err)
		}
	}
	for _, number := range []string{"9007199254740992", "-9007199254740992", "12", "1.25", "1e2"} {
		if _, err := CopyValue(json.Number(number), TypeFloat); err != nil {
			t.Fatalf("valid float rejected: %s, %v", number, err)
		}
	}
}

// TestJSONValueContainerCompatibility covers ordinary Go string maps and arrays without coercing keys.
func TestJSONValueContainerCompatibility(t *testing.T) {
	for _, value := range []any{map[string]string{"plan": "pro"}, []string{"admin"}} {
		if _, err := CloneValue(value); err != nil {
			t.Fatal(err)
		}
	}
	many := map[string]string{}
	for i := 0; i <= MaxSubjects; i++ {
		many[fmt.Sprint(i)] = "x"
	}
	for _, value := range []any{many, map[string]string{string([]byte{255}): "x"}, map[string]string{"x": strings.Repeat("x", MaxValueBytes)}, make([]string, MaxSubjects+1), []string{string([]byte{255})}} {
		if _, err := CloneValue(value); err != ErrInvalidValue {
			t.Fatal(err)
		}
	}
	if _, err := CopyValue(json.Number("1e999"), TypeFloat); err != ErrInvalidValue {
		t.Fatal(err)
	}
}
