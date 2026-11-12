package feature

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// sampleDocument supplies all targeting conditions for validation and ownership tests.
func sampleDocument() Document {
	return Document{Schema: SchemaVersion, Flags: map[string]Flag{
		"checkout-v2": {Rules: []Rule{
			{Subjects: []string{"tenant:internal", "tenant:staff"}, Value: true},
			{Attributes: map[string]string{"plan": "pro"}, Rollout: &Rollout{BasisPoints: 1000, Salt: "checkout-2026"}, Value: true},
		}},
	}}
}

// TestCompileValidatesEveryBoundary checks typed sources before they can publish data.
func TestCompileValidatesEveryBoundary(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Document)
	}{
		{"schema", func(d *Document) { d.Schema = 2 }},
		{"missing flags", func(d *Document) { d.Flags = nil }},
		{"too many flags", func(d *Document) {
			for i := 0; i <= MaxFlags; i++ {
				d.Flags[fmt.Sprint(i)] = Flag{}
			}
		}},
		{"empty key", func(d *Document) { d.Flags[""] = Flag{} }},
		{"invalid key", func(d *Document) { d.Flags["private/key"] = Flag{} }},
		{"long key", func(d *Document) { d.Flags[strings.Repeat("a", MaxKeyBytes+1)] = Flag{} }},
		{"many rules", func(d *Document) { d.Flags["test"] = Flag{Rules: make([]Rule, MaxRules+1)} }},
		{"empty rule", func(d *Document) { d.Flags["test"] = Flag{Rules: []Rule{{}}} }},
		{"many subjects", func(d *Document) { d.Flags["test"] = Flag{Rules: []Rule{{Subjects: make([]string, MaxSubjects+1)}}} }},
		{"empty subject", func(d *Document) { d.Flags["test"] = Flag{Rules: []Rule{{Subjects: []string{""}}}} }},
		{"long subject", func(d *Document) {
			d.Flags["test"] = Flag{Rules: []Rule{{Subjects: []string{strings.Repeat("s", MaxStringBytes+1)}}}}
		}},
		{"invalid utf8", func(d *Document) { d.Flags["test"] = Flag{Rules: []Rule{{Subjects: []string{string([]byte{255})}}}} }},
		{"many attrs", func(d *Document) {
			a := map[string]string{}
			for i := 0; i <= MaxAttributes; i++ {
				a[fmt.Sprint(i)] = "x"
			}
			d.Flags["test"] = Flag{Rules: []Rule{{Attributes: a}}}
		}},
		{"invalid attr key", func(d *Document) {
			d.Flags["test"] = Flag{Rules: []Rule{{Attributes: map[string]string{"bad key": "value"}}}}
		}},
		{"invalid attr value", func(d *Document) {
			d.Flags["test"] = Flag{Rules: []Rule{{Attributes: map[string]string{"key": strings.Repeat("x", MaxStringBytes+1)}}}}
		}},
		{"negative rollout", func(d *Document) { f := d.Flags["checkout-v2"]; f.Rules[1].Rollout.BasisPoints = -1 }},
		{"large rollout", func(d *Document) { f := d.Flags["checkout-v2"]; f.Rules[1].Rollout.BasisPoints = 10001 }},
		{"empty salt", func(d *Document) { f := d.Flags["checkout-v2"]; f.Rules[1].Rollout.Salt = "" }},
		{"long salt", func(d *Document) {
			f := d.Flags["checkout-v2"]
			f.Rules[1].Rollout.Salt = strings.Repeat("x", MaxStringBytes+1)
		}},
		{"total budget", func(d *Document) {
			r := Rule{Value: true}
			for i := 0; i < MaxSubjects; i++ {
				r.Subjects = append(r.Subjects, strings.Repeat("x", MaxStringBytes))
			}
			d.Flags["test"] = Flag{Rules: []Rule{r}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := sampleDocument()
			tc.change(&d)
			if _, err := compile(d); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("got %v", err)
			}
		})
	}
	if _, err := compile(Document{Schema: 1, Flags: map[string]Flag{strings.Repeat("x", MaxKeyBytes): {}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := compile(Document{Schema: 1, Flags: map[string]Flag{}}); err != nil {
		t.Fatal(err)
	}
}

// TestMemoryOwnership proves no mutable data crosses the snapshot or source boundary.
func TestMemoryOwnership(t *testing.T) {
	ctx := context.Background()
	doc := sampleDocument()
	source, err := NewMemorySource(doc)
	if err != nil {
		t.Fatal(err)
	}
	f := doc.Flags["checkout-v2"]
	f.Rules[0].Subjects[0] = "changed"
	f.Rules[1].Attributes["plan"] = "changed"
	f.Rules[1].Rollout.Salt = "changed"
	loaded, err := source.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Flags["checkout-v2"].Rules[1].Attributes["plan"]; got != "pro" {
		t.Fatal(got)
	}
	loaded.Flags["checkout-v2"] = Flag{Disabled: true}
	loaded, err = source.Load(ctx)
	if err != nil || loaded.Flags["checkout-v2"].Disabled {
		t.Fatal("source was mutated", err)
	}
	state, err := compile(loaded)
	if err != nil {
		t.Fatal(err)
	}
	f = loaded.Flags["checkout-v2"]
	f.Rules[0].Subjects[0] = "external"
	f.Rules[1].Attributes["plan"] = "free"
	f.Rules[1].Rollout.BasisPoints = 0
	view := &View{state: state}
	if !view.Enabled(ctx, "checkout-v2", false, Subject{Key: "tenant:internal"}) {
		t.Fatal("snapshot borrowed inputs")
	}
	if _, err := NewMemorySource(Document{}); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	before, _ := source.Load(ctx)
	if err := source.Replace(Document{}); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	a, _ := compile(before)
	after, _ := source.Load(ctx)
	b, _ := compile(after)
	if a.revision != b.revision {
		t.Fatal("failed replace changed source")
	}
	if err := source.Replace(Document{Schema: 1, Flags: map[string]Flag{}}); err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := source.Load(cancelCtx); err != context.Canceled {
		t.Fatal(err)
	}
}

// TestRevisionNormalization proves equivalent unordered inputs preserve revision identity.
func TestRevisionNormalization(t *testing.T) {
	a := sampleDocument()
	b := sampleDocument()
	b.Flags["checkout-v2"].Rules[0].Subjects[0], b.Flags["checkout-v2"].Rules[0].Subjects[1] = "tenant:staff", "tenant:internal"
	x, _ := compile(a)
	y, _ := compile(b)
	if x.revision != y.revision {
		t.Fatal("subject ordering changed revision")
	}
	f := b.Flags["checkout-v2"]
	f.Default = true
	b.Flags["checkout-v2"] = f
	y, _ = compile(b)
	if x.revision == y.revision {
		t.Fatal("different decisions share revision")
	}
}

// TestCompileBudgetAcrossFlags rejects aggregate growth independently of map iteration order.
func TestCompileBudgetAcrossFlags(t *testing.T) {
	doc := Document{Schema: 1, Flags: map[string]Flag{}}
	for i := 0; i < MaxFlags; i++ {
		key := fmt.Sprintf("%0128d", i)
		doc.Flags[key] = Flag{}
	}
	subjects := make([]string, 700)
	for i := range subjects {
		subjects[i] = strings.Repeat("x", 64)
	}
	key := fmt.Sprintf("%0128d", 0)
	doc.Flags[key] = Flag{Rules: []Rule{{Subjects: subjects}}}
	for i := 0; i < 20; i++ {
		if _, err := compile(doc); err != ErrInvalidDocument {
			t.Fatal(err)
		}
	}
}
