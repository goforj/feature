package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestCalendarUTCYearBounds rejects schedules whose normalized form cannot be persisted.
func TestCalendarUTCYearBounds(t *testing.T) {
	for _, at := range []time.Time{
		time.Date(0, 1, 1, 0, 0, 0, 0, time.FixedZone("east", 14*3600)),
		time.Date(9999, 12, 31, 23, 0, 0, 0, time.FixedZone("west", -14*3600)),
	} {
		for _, field := range []string{"start", "end", "progressive"} {
			t.Run(fmt.Sprintf("%d/%s", at.Year(), field), func(t *testing.T) {
				rule := Rule{Value: true}
				switch field {
				case "start":
					rule.Start = &at
				case "end":
					rule.End = &at
				default:
					start, end := at, at.Add(24*time.Hour)
					if at.Year() == 9999 {
						start, end = at.Add(-24*time.Hour), at
					}
					rule.Rollout = &Rollout{Salt: "v1", Progressive: &Progressive{Start: start, End: end, From: 0, To: 10000}}
				}
				doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: false, Rules: []Rule{rule}}}}
				if _, err := Encode(doc); err != ErrInvalidDocument {
					t.Fatalf("invalid UTC schedule encoded: %v", err)
				}
				if _, err := NewMemorySource(doc); err != ErrInvalidDocument {
					t.Fatalf("invalid UTC schedule published: %v", err)
				}
				data, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := Decode(data); err != ErrInvalidDocument {
					t.Fatalf("invalid UTC schedule decoded: %v", err)
				}
			})
		}
	}
	for _, at := range []time.Time{
		time.Date(0, 1, 1, 0, 0, 0, 0, time.FixedZone("west", -14*3600)),
		time.Date(9999, 12, 31, 23, 0, 0, 0, time.FixedZone("east", 14*3600)),
	} {
		doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: false, Rules: []Rule{{Start: &at, Value: true}}}}}
		encoded, err := Encode(doc)
		if err != nil {
			t.Fatal("valid calendar boundary rejected", err)
		}
		if _, err := Decode(encoded); err != nil {
			t.Fatal("valid calendar boundary did not round-trip", err)
		}
	}
}

// TestSerializedDocumentLimit checks composed metadata against the actual publication size.
func TestSerializedDocumentLimit(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	name := strings.Repeat("v", MaxKeyBytes)
	doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{}}
	for index := 0; index < 100; index++ {
		flag := Flag{Variants: map[string]any{name: true}, DefaultVariant: name}
		for rule := 0; rule < MaxRules; rule++ {
			flag.Rules = append(flag.Rules, Rule{Start: &at, Variant: name})
		}
		doc.Flags[fmt.Sprint(index)] = flag
	}
	if _, err := Encode(doc); err != ErrInvalidDocument {
		t.Fatal("oversized encoded composition accepted", err)
	}
	if _, err := NewMemorySource(doc); err != ErrInvalidDocument {
		t.Fatal("oversized publication accepted", err)
	}
	delete(doc.Flags, "99")
	for index := 50; index < 99; index++ {
		delete(doc.Flags, fmt.Sprint(index))
	}
	source := mustMemorySource(t, doc)
	loaded, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(loaded)
	if err != nil || len(encoded) > MaxDocumentBytes {
		t.Fatalf("bounded composition failed: %v, %d", err, len(encoded))
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatal(err)
	}
}

// TestCalendarFailureRetention keeps invalid normalized schedules out of startup and refresh publication.
func TestCalendarFailureRetention(t *testing.T) {
	ctx := context.Background()
	valid := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: true}}}
	at := time.Date(9999, 12, 31, 23, 0, 0, 0, time.FixedZone("west", -14*3600))
	invalid := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: true, Rules: []Rule{{Start: &at, Value: false}}}}}
	current := invalid
	source := SourceFunc(func(context.Context) (Document, error) { return current, nil })
	if _, err := Open(ctx, source); err != ErrInvalidDocument {
		t.Fatal("invalid initial schedule accepted", err)
	}
	current = valid
	engine := sourceClient(t, source)
	before := engine.Status().Revision
	current = invalid
	if err := engine.Refresh(ctx); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if result := New(engine).Evaluate(ctx, "flag", false); result.Err != nil || !result.Value || result.Revision != before {
		t.Fatal("invalid schedule replaced snapshot", result)
	}
	current = valid
	if err := engine.Refresh(ctx); err != nil || engine.Status().LastError != nil {
		t.Fatal("schedule recovery failed", err)
	}
}

// TestEncodingFailureSurfaces keeps serialization errors visible at the private encoding boundary.
func TestEncodingFailureSurfaces(t *testing.T) {
	at := time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC)
	doc := Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"flag": {Default: false, Rules: []Rule{{Start: &at, Value: true}}}}}
	if _, err := encodeDocument(doc); err == nil {
		t.Fatal("serialization failure was discarded")
	}
}
