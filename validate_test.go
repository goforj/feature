package feature

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestValidationLocations gives authoring tools deterministic paths without leaking details through errors.
func TestValidationLocations(t *testing.T) {
	validRule := Rule{Subjects: []string{"tenant"}, Value: false}
	cases := []struct {
		name string
		doc  Document
		path string
	}{
		{"schema", Document{Schema: 99}, "/schema"},
		{"inventory", Document{Schema: 1}, "/flags"},
		{"key", Document{Schema: 1, Flags: map[string]Flag{"private/key": {}}}, "/flags"},
		{"type", Document{Schema: 2, Flags: map[string]Flag{"private": {Type: TypeInteger, Default: "private value"}}}, "/flags/private"},
		{"default variant", Document{Schema: 2, Flags: map[string]Flag{"private": {DefaultVariant: "missing"}}}, "/flags/private/default_variant"},
		{"off variant", Document{Schema: 2, Flags: map[string]Flag{"private": {Variants: map[string]any{"yes": true}, DefaultVariant: "yes", OffVariant: "missing"}}}, "/flags/private/off_variant"},
		{"invalid catalog", Document{Schema: 2, Flags: map[string]Flag{"private": {Variants: map[string]any{"yes": true, "no": "wrong"}, DefaultVariant: "yes", OffVariant: "no"}}}, "/flags/private"},
		{"rule", Document{Schema: 2, Flags: map[string]Flag{"private": {Default: false, Rules: []Rule{validRule, {Value: true}}}}}, "/flags/private/rules/1"},
		{"condition", Document{Schema: 2, Flags: map[string]Flag{"private": {Default: false, Rules: []Rule{{Conditions: []Condition{{Attribute: "plan", Operator: "unknown"}}, Value: true}}}}}, "/flags/private/rules/0/conditions/0"},
		{"rule variant", Document{Schema: 2, Flags: map[string]Flag{"private": {Default: false, Rules: []Rule{{Subjects: []string{"tenant"}, Variant: "missing"}}}}}, "/flags/private/rules/0/variant"},
		{"contradictory rule", Document{Schema: 2, Flags: map[string]Flag{"private": {Variants: map[string]any{"yes": true}, DefaultVariant: "yes", Rules: []Rule{{Subjects: []string{"tenant"}, Variant: "yes", Value: false}}}}}, "/flags/private/rules/0"},
		{"pattern type", patternDocument("a", 1), "/flags/test/rules/0/conditions/0/value"},
		{"pattern syntax", patternDocument("[", 1), "/flags/test/rules/0/conditions/0/value"},
	}
	cases[len(cases)-2].doc.Flags["test"].Rules[0].Conditions[0].Value = true
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for repeat := 0; repeat < 3; repeat++ {
				err := Validate(tc.doc)
				var issue *ValidationError
				if !errors.As(err, &issue) || !errors.Is(err, ErrInvalidDocument) || issue.Path != tc.path || issue.Problem == "" {
					t.Fatal(err, issue)
				}
				if err.Error() != ErrInvalidDocument.Error() {
					t.Fatal("ordinary error text leaked authoring details", err)
				}
				if _, err := NewMemorySource(tc.doc); err != ErrInvalidDocument {
					t.Fatal("runtime error contract changed", err)
				}
			}
		})
	}
	valid := Document{Schema: 2, Flags: map[string]Flag{"value": {Default: map[string]any{"nested": []any{int64(3)}}}}}
	before, err := Encode(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	after, err := Encode(valid)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("validation changed definition", err)
	}
}

// TestValidationBudgets bounds diagnostic work using the same compilation and container limits.
func TestValidationBudgets(t *testing.T) {
	flags := map[string]Flag{}
	for index := 0; index <= MaxFlags; index++ {
		flags[fmt.Sprint(index)] = Flag{}
	}
	rules := make([]Rule, MaxRules+1)
	conditions := make([]Condition, MaxAttributes+1)
	large := strings.Repeat("x", MaxValueBytes/2)
	largeRules := make([]Rule, MaxRules)
	for index := range largeRules {
		largeRules[index] = Rule{Subjects: []string{"tenant"}, Value: large}
	}
	global := Document{Schema: 1, Flags: map[string]Flag{}}
	for index := 0; index < MaxFlags; index++ {
		global.Flags[fmt.Sprintf("%0128d", index)] = Flag{}
	}
	subjects := make([]string, 700)
	for index := range subjects {
		subjects[index] = strings.Repeat("x", 64)
	}
	global.Flags[fmt.Sprintf("%0128d", 0)] = Flag{Rules: []Rule{{Subjects: subjects}}}
	small := strings.Repeat(`\pL`, 8)
	_, entries, err := patternResources(small)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		doc  Document
		path string
	}{
		{Document{Schema: 1, Flags: flags}, "/flags"},
		{Document{Schema: 2, Flags: map[string]Flag{"a": {Rules: rules}}}, "/flags/a/rules"},
		{Document{Schema: 2, Flags: map[string]Flag{"a": {Rules: []Rule{{Conditions: conditions}}}}}, "/flags/a/rules/0/conditions"},
		{Document{Schema: 2, Flags: map[string]Flag{"a": {Default: large, Rules: largeRules}}}, "/flags/a"},
		{global, "/flags"},
		{patternDocument("a{1000}b{1000}c{1000}d{1000}e{94}", 17), ""},
		{patternDocument(small, MaxDocumentPatternRuneEntries/entries+1), ""},
	} {
		err := Validate(tc.doc)
		var issue *ValidationError
		if !errors.As(err, &issue) || tc.path != "" && issue.Path != tc.path {
			t.Fatal(err, issue, tc.path)
		}
		if tc.path == "" && !strings.HasSuffix(issue.Path, "/value") {
			t.Fatal(issue)
		}
	}
}

// TestValidationOrdering keeps the first identified flag stable across map iteration orders.
func TestValidationOrdering(t *testing.T) {
	doc := Document{Schema: 2, Flags: map[string]Flag{"z": {Default: func() {}}, "a": {Default: "good"}, "b": {Default: func() {}}}}
	for index := 0; index < 20; index++ {
		err := Validate(doc)
		var issue *ValidationError
		if !errors.As(err, &issue) || issue.Path != "/flags/b" {
			t.Fatal(err, issue)
		}
	}
}
