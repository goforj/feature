package feature

import (
	"context"
	"encoding/json"
	"regexp/syntax"
	"strings"
	"testing"
)

// patternDocument supplies independent pattern instances within normal document bounds.
func patternDocument(pattern string, count int) Document {
	rules := []Rule{}
	for count > 0 {
		rule := Rule{Value: true}
		for len(rule.Conditions) < MaxAttributes && count > 0 {
			rule.Conditions = append(rule.Conditions, Condition{Attribute: "name", Operator: OpMatches, Value: pattern})
			count--
		}
		rules = append(rules, rule)
	}
	return Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"test": {Default: false, Rules: rules}}}
}

// TestPatternEstimatesBoundCompiler compares inexpensive programs against the pre-expansion estimate.
func TestPatternEstimatesBoundCompiler(t *testing.T) {
	patterns := []string{"", "a", "abc", "[a-z]", ".", "(?s:.)", "^a$", "\\Aa\\z", "\\ba\\B", "(a)", "a|bc|def", "ab*c+d?", "(?:a?)*", "(?:a?)+", "a{0}", "a{0,3}", "a{2,4}", "a{2,}", "a{0,}", "(?:ab?){3}", "(?i:abc)", "[^\\s\\S]"}
	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			cost, err := patternCost(pattern)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := syntax.Parse(pattern, syntax.Perl)
			if err != nil {
				t.Fatal(err)
			}
			program, err := syntax.Compile(tree.Simplify())
			if err != nil {
				t.Fatal(err)
			}
			if len(program.Inst) > cost {
				t.Fatalf("actual %d exceeds estimate %d", len(program.Inst), cost)
			}
		})
	}
	// Parser simplification removes these nodes, so exercise their defensive estimates directly.
	for _, tree := range []*syntax.Regexp{
		{Op: syntax.OpNoMatch},
		{Op: syntax.OpRepeat, Min: 0, Max: 0, Sub: []*syntax.Regexp{{Op: syntax.OpEmptyMatch}}},
		{Op: syntax.OpRepeat, Min: 0, Max: -1, Sub: []*syntax.Regexp{{Op: syntax.OpEmptyMatch}}},
		{Op: syntax.OpRepeat, Min: 2, Max: -1, Sub: []*syntax.Regexp{{Op: syntax.OpLiteral, Rune: []rune{'a'}}}},
	} {
		if patternTreeCost(tree) < 1 {
			t.Fatal("missing instruction estimate")
		}
	}
	if patternTreeCost(&syntax.Regexp{Op: syntax.Op(255)}) != MaxPatternInstructions+1 {
		t.Fatal("unknown operator accepted")
	}
	if boundedPatternSum(MaxPatternInstructions+1, 0) != MaxPatternInstructions+1 || boundedPatternSum(MaxPatternInstructions, 1) != MaxPatternInstructions+1 {
		t.Fatal("sum did not saturate")
	}
	if boundedPatternProduct(MaxPatternInstructions, 2) != MaxPatternInstructions+1 || boundedPatternProduct(7, 0) != 0 {
		t.Fatal("product did not saturate")
	}
}

// TestPatternBudgetBoundaries rejects malformed sources and excessive expanded programs.
func TestPatternBudgetBoundaries(t *testing.T) {
	for _, pattern := range []string{strings.Repeat("a", MaxStringBytes+1), string([]byte{255}), "[", "(?:" + strings.Repeat("a?", 500) + "){1000}", "(?:a{1000}){1000}"} {
		if _, err := patternCost(pattern); err != ErrInvalidDocument {
			t.Fatalf("pattern accepted: %v", err)
		}
	}
	for _, value := range []any{false, "[", "(?:" + strings.Repeat("a?", 500) + "){1000}"} {
		doc := patternDocument("a", 1)
		doc.Flags["test"].Rules[0].Conditions[0].Value = value
		if err := validatePatternBudget(doc); err != ErrInvalidDocument {
			t.Fatal(err)
		}
	}
	doc := patternDocument("a", 1)
	flag := doc.Flags["test"]
	flag.Rules = make([]Rule, MaxRules+1)
	doc.Flags["test"] = flag
	if err := validatePatternBudget(doc); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if _, _, err := normalizeFlag(flag, TypedSchemaVersion); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	doc = patternDocument("a", 1)
	doc.Flags["test"].Rules[0].Conditions = make([]Condition, MaxAttributes+1)
	if err := validatePatternBudget(doc); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	// This estimate is exactly 4096: 4094 literal instructions plus fail and match.
	boundary := "a{1000}b{1000}c{1000}d{1000}e{94}"
	if cost, err := patternCost(boundary); err != nil || cost != MaxPatternInstructions {
		t.Fatalf("%d %v", cost, err)
	}
	if _, err := patternCost(boundary + "e"); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if err := validatePatternBudget(patternDocument(boundary, 16)); err != nil {
		t.Fatal(err)
	}
	if err := validatePatternBudget(patternDocument(boundary, 17)); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	condition := Condition{Attribute: "name", Operator: OpMatches, Value: boundary + "e"}
	if _, err := normalizeCondition(condition); err != ErrInvalidDocument {
		t.Fatal(err)
	}
}

// TestPatternBudgetSourceRejection keeps oversized programs out of every publication path.
func TestPatternBudgetSourceRejection(t *testing.T) {
	ctx := context.Background()
	bad := patternDocument("(?:"+strings.Repeat("a?", 500)+"){1000}", 128)
	raw, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(raw); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if _, err := NewMemorySource(bad); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if _, err := Open(ctx, sourceFunc(func(context.Context) (Document, error) { return bad, nil })); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	good := patternDocument("^a+$", 1)
	memory, err := NewMemorySource(good)
	if err != nil {
		t.Fatal(err)
	}
	if err := memory.Replace(bad); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	loaded, err := memory.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cached := memory.document.Flags["test"].Rules[0].Conditions[0].pattern
	if loaded.Flags["test"].Rules[0].Conditions[0].pattern != cached {
		t.Fatal("immutable regexp was recompiled")
	}
	// Changing the text of an owned condition must invalidate its cached program.
	condition := loaded.Flags["test"].Rules[0].Conditions[0]
	condition.Value = "b"
	normalized, err := normalizeCondition(condition)
	if err != nil || normalized.pattern == cached || !normalized.pattern.MatchString("b") {
		t.Fatal("stale program retained", err)
	}
	condition.Operator = OpEqual
	normalized, err = normalizeCondition(condition)
	if err != nil || normalized.pattern != nil {
		t.Fatal("non-pattern condition retained a compiled program", err)
	}
	current := good
	client, err := Open(ctx, sourceFunc(func(context.Context) (Document, error) { return current, nil }))
	if err != nil {
		t.Fatal(err)
	}
	before := client.Status()
	current = bad
	if err := client.Refresh(ctx); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if after := client.Status(); after.Revision != before.Revision || !after.LastSuccess.Equal(before.LastSuccess) {
		t.Fatal("failed refresh replaced snapshot")
	}
}

// TestPatternRuneEstimatesBoundCompiler checks shared tables without charging repeated references.
func TestPatternRuneEstimatesBoundCompiler(t *testing.T) {
	for _, pattern := range []string{`\pL`, `\pL{1000}`, `(?:\pL\pN){3,6}`, `(?i:abc)`, `abc{10}`, `.`, `(?s:.)`, `[a-z]`, `(?:\pL?)*`} {
		_, estimate, err := patternResources(pattern)
		if err != nil {
			t.Fatal(err)
		}
		tree, err := syntax.Parse(pattern, syntax.Perl)
		if err != nil {
			t.Fatal(err)
		}
		program, err := syntax.Compile(tree.Simplify())
		if err != nil {
			t.Fatal(err)
		}
		seen := map[*rune]bool{}
		actual := 0
		for _, inst := range program.Inst {
			if len(inst.Rune) > 0 && !seen[&inst.Rune[0]] {
				seen[&inst.Rune[0]] = true
				actual += len(inst.Rune)
			}
		}
		if actual > estimate {
			t.Fatalf("%s: retained %d exceeds estimate %d", pattern, actual, estimate)
		}
	}
	tree := &syntax.Regexp{Op: syntax.OpCharClass, Rune: make([]rune, MaxPatternRuneEntries)}
	if patternRuneCost(tree) != MaxPatternRuneEntries {
		t.Fatal("exact rune boundary rejected")
	}
	if boundedPatternRuneSum(MaxPatternRuneEntries, 1) != MaxPatternRuneEntries+1 || boundedPatternRuneSum(MaxPatternRuneEntries+1, 0) != MaxPatternRuneEntries+1 {
		t.Fatal("rune estimate did not saturate")
	}
}

// TestPatternRuneBudgetRejection bounds Unicode classes independently of instruction counts.
func TestPatternRuneBudgetRejection(t *testing.T) {
	ctx := context.Background()
	large := strings.Repeat(`\pL`, 250)
	small := strings.Repeat(`\pL`, 8)
	if _, _, err := patternResources(large); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if _, _, err := patternResources(small); err != nil {
		t.Fatal(err)
	}
	if err := validatePatternBudget(patternDocument(small, 1)); err != nil {
		t.Fatal(err)
	}
	_, runes, err := patternResources(small)
	if err != nil {
		t.Fatal(err)
	}
	fits := MaxDocumentPatternRuneEntries / runes
	if err := validatePatternBudget(patternDocument(small, fits)); err != nil {
		t.Fatal(err)
	}
	aggregate := patternDocument(small, fits+1)
	if err := validatePatternBudget(aggregate); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	for _, bad := range []Document{patternDocument(large, 128), aggregate} {
		raw, err := json.Marshal(bad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(raw); err != ErrInvalidDocument {
			t.Fatal(err)
		}
		if _, err := NewMemorySource(bad); err != ErrInvalidDocument {
			t.Fatal(err)
		}
		if _, err := Open(ctx, sourceFunc(func(context.Context) (Document, error) { return bad, nil })); err != ErrInvalidDocument {
			t.Fatal(err)
		}
	}
	current := patternDocument(`\pL+`, 1)
	memory, err := NewMemorySource(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := memory.Replace(aggregate); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if loaded, err := memory.Load(ctx); err != nil || loaded.Flags["test"].Rules[0].Conditions[0].Value != `\pL+` {
		t.Fatal("failed replacement changed source", err)
	}
	client, err := Open(ctx, sourceFunc(func(context.Context) (Document, error) { return current, nil }))
	if err != nil {
		t.Fatal(err)
	}
	before := client.Status()
	current = aggregate
	if err := client.Refresh(ctx); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	if after := client.Status(); after.Revision != before.Revision || !after.LastSuccess.Equal(before.LastSuccess) {
		t.Fatal("failed refresh changed snapshot")
	}
}

// TestPatternOptimizerStorage rejects capacity and optimizer amplification before regexp compilation.
func TestPatternOptimizerStorage(t *testing.T) {
	for _, pattern := range []string{`^\pL{900}$`, strings.Repeat(`[\pL\PL]`, 100)} {
		if _, _, err := patternResources(pattern); err != ErrInvalidDocument {
			t.Fatalf("optimizer storage accepted: %v", err)
		}
		doc := patternDocument(pattern, 4)
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(raw); err != ErrInvalidDocument {
			t.Fatal(err)
		}
		if _, err := NewMemorySource(doc); err != ErrInvalidDocument {
			t.Fatal(err)
		}
	}
	_, entries, err := patternResources(`^\pL{16}$`)
	if err != nil {
		t.Fatal(err)
	}
	fits := MaxDocumentPatternRuneEntries / entries
	if err := validatePatternBudget(patternDocument(`^\pL{16}$`, fits)); err != nil {
		t.Fatal(err)
	}
	if err := validatePatternBudget(patternDocument(`^\pL{16}$`, fits+1)); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	for _, pattern := range []string{`\pL{64}`, `^\pL{64}$`, `^[\pL\PL]{64}$`, `^(?:\pL|\pN)$`, `^(a?)*$`, `^(?i:k)$`, `^.$`, `^(?s:.)$`, `^a$`, `^(?:ab|cd)?$`} {
		if _, _, err := patternResources(pattern); err != nil {
			t.Fatalf("ordinary pattern %s rejected: %v", pattern, err)
		}
	}
	// Optional copies charge overlapping frontiers conservatively at each epsilon instruction.
	if _, _, err := patternResources(`^\pL{1,64}$`); err != ErrInvalidDocument {
		t.Fatal("conservative optional-frontier bound changed", err)
	}
	// Shared epsilon successors are visited once, including cycles introduced by stars.
	program := &syntax.Prog{Inst: []syntax.Inst{
		{Op: syntax.InstAlt, Out: 1, Arg: 1},
		{Op: syntax.InstNop, Out: 2},
		{Op: syntax.InstRune1, Rune: []rune{'a'}},
	}}
	if got := patternRuneFrontier(program, 0, make([]int, 3), 1); got != 2 {
		t.Fatal(got)
	}
	wide := &syntax.Prog{Start: 0, Inst: []syntax.Inst{
		{Op: syntax.InstEmptyWidth, Arg: uint32(syntax.EmptyBeginText), Out: 1},
		{Op: syntax.InstRune, Rune: make([]rune, MaxPatternRuneEntries+1)},
	}}
	if patternOnePassCost(wide) != MaxPatternRuneEntries+1 {
		t.Fatal("frontier overflow accepted")
	}
}
