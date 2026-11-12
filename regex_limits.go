package feature

import (
	"regexp/syntax"
	"unicode"
)

const (
	// MaxPatternInstructions bounds a conservative estimate of one expanded regular expression program.
	MaxPatternInstructions = 4096
	// MaxDocumentPatternInstructions bounds the sum of estimates for all matches conditions.
	// Repeated patterns are charged separately, even when compiled state can be reused.
	MaxDocumentPatternInstructions = 65536
	// MaxPatternRuneEntries bounds a conservative estimate of rune and one-pass dispatch storage.
	// Estimates include backing capacity and optimizer copies and can reject smaller actual programs.
	// Optional Unicode repetitions can exceed the estimate even when fixed repetitions fit.
	MaxPatternRuneEntries = 262144
	// MaxDocumentPatternRuneEntries bounds the sum of four-byte storage-entry estimates for matches conditions.
	MaxDocumentPatternRuneEntries = 1048576
)

// validatePatternBudget rejects excessive aggregate expansion before any pattern is compiled.
func validatePatternBudget(doc Document) error {
	total := 0
	totalRunes := 0
	for _, flag := range doc.Flags {
		if len(flag.Rules) > MaxRules {
			return ErrInvalidDocument
		}
		for _, rule := range flag.Rules {
			if len(rule.Conditions) > MaxAttributes {
				return ErrInvalidDocument
			}
			for _, condition := range rule.Conditions {
				if condition.Operator != OpMatches {
					continue
				}
				text, ok := condition.Value.(string)
				if !ok {
					return ErrInvalidDocument
				}
				cost, runes, err := patternResources(text)
				if err != nil {
					return err
				}
				total += cost
				totalRunes += runes
				if total > MaxDocumentPatternInstructions || totalRunes > MaxDocumentPatternRuneEntries {
					return ErrInvalidDocument
				}
			}
		}
	}
	return nil
}

// patternCost validates both expansion and retained rune tables for a single condition.
func patternCost(text string) (int, error) {
	cost, _, err := patternResources(text)
	return cost, err
}

// patternResources parses bounded source without Simplify, which expands counted repetitions.
func patternResources(text string) (int, int, error) {
	if !validString(text) {
		return 0, 0, ErrInvalidDocument
	}
	tree, err := syntax.Parse(text, syntax.Perl)
	if err != nil {
		return 0, 0, ErrInvalidDocument
	}
	// The compiler adds fail and match instructions around the expression.
	cost := boundedPatternSum(patternTreeCost(tree), 2)
	runes := patternRuneCost(tree)
	if cost > MaxPatternInstructions || runes > MaxPatternRuneEntries {
		return 0, 0, ErrInvalidDocument
	}
	// Expansion is safe only after the instruction bound has been established.
	// A parsed, simplified syntax tree contains only compiler-supported operators.
	program, _ := syntax.Compile(tree.Simplify())
	runes = boundedPatternRuneSum(runes, patternOnePassCost(program))
	if runes > MaxPatternRuneEntries {
		return 0, 0, ErrInvalidDocument
	}
	return cost, runes, nil
}

// patternRuneCost bounds retained tables before repetition expansion.
// Simplify and the compiler share rune slices across repeated copies of a syntax node.
// Distinct source nodes include their backing capacity, which can outlive class simplification.
func patternRuneCost(tree *syntax.Regexp) int {
	cost := cap(tree.Rune)
	if tree.Op == syntax.OpAnyCharNotNL {
		cost = 4
	}
	if tree.Op == syntax.OpAnyChar {
		cost = 2
	}
	for _, child := range tree.Sub {
		cost = boundedPatternRuneSum(cost, patternRuneCost(child))
	}
	return cost
}

// patternOnePassCost bounds copies made by Go's anchored one-pass optimizer.
// Each instruction's first-rune frontier bounds its Rune dispatch table, including
// alternation merges and epsilon propagation. Twice its length plus eight entries
// bounds append capacity rounding; half its length plus one bounds the Next table.
// Charging every instruction also covers temporary tables discarded by cleanup.
func patternOnePassCost(program *syntax.Prog) int {
	start := program.Inst[program.Start]
	if start.Op != syntax.InstEmptyWidth || syntax.EmptyOp(start.Arg)&syntax.EmptyBeginText == 0 {
		return 0
	}
	seen := make([]int, len(program.Inst))
	cost := 0
	for pc := range program.Inst {
		frontier := patternRuneFrontier(program, uint32(pc), seen, pc+1)
		if frontier > MaxPatternRuneEntries {
			return MaxPatternRuneEntries + 1
		}
		cost = boundedPatternRuneSum(cost, frontier*2+8+frontier/2+1)
		if cost > MaxPatternRuneEntries {
			return cost
		}
	}
	return cost
}

// patternRuneFrontier bounds distinct first-rune ranges reachable through epsilon edges.
func patternRuneFrontier(program *syntax.Prog, pc uint32, seen []int, visit int) int {
	if seen[pc] == visit {
		return 0
	}
	seen[pc] = visit
	inst := program.Inst[pc]
	switch inst.Op {
	case syntax.InstAlt, syntax.InstAltMatch:
		return boundedPatternRuneSum(patternRuneFrontier(program, inst.Out, seen, visit), patternRuneFrontier(program, inst.Arg, seen, visit))
	case syntax.InstCapture, syntax.InstEmptyWidth, syntax.InstNop:
		return patternRuneFrontier(program, inst.Out, seen, visit)
	case syntax.InstRune, syntax.InstRune1:
		if len(inst.Rune) == 1 {
			count := 2
			if syntax.Flags(inst.Arg)&syntax.FoldCase != 0 {
				first := inst.Rune[0]
				for next := unicode.SimpleFold(first); next != first; next = unicode.SimpleFold(next) {
					count += 2
				}
			}
			return count
		}
		return len(inst.Rune)
	case syntax.InstRuneAny:
		return 2
	case syntax.InstRuneAnyNotNL:
		return 4
	default:
		return 0
	}
}

// boundedPatternRuneSum saturates source-table estimates before addition can overflow.
func boundedPatternRuneSum(left, right int) int {
	if left > MaxPatternRuneEntries || right > MaxPatternRuneEntries-left {
		return MaxPatternRuneEntries + 1
	}
	return left + right
}

// patternTreeCost conservatively bounds expanded instructions without constructing expansions.
// Nullable stars can need two control instructions; bounded repeats include each optional copy.
// Estimates intentionally retain instructions that the compiler may later simplify away.
func patternTreeCost(tree *syntax.Regexp) int {
	switch tree.Op {
	case syntax.OpNoMatch, syntax.OpEmptyMatch, syntax.OpCharClass, syntax.OpAnyCharNotNL, syntax.OpAnyChar,
		syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return 1
	case syntax.OpLiteral:
		return len(tree.Rune)
	case syntax.OpCapture:
		return boundedPatternSum(patternTreeCost(tree.Sub[0]), 2)
	case syntax.OpStar:
		return boundedPatternSum(patternTreeCost(tree.Sub[0]), 2)
	case syntax.OpPlus, syntax.OpQuest:
		return boundedPatternSum(patternTreeCost(tree.Sub[0]), 1)
	case syntax.OpConcat, syntax.OpAlternate:
		cost := 0
		if tree.Op == syntax.OpAlternate {
			cost = len(tree.Sub) - 1
		}
		for _, child := range tree.Sub {
			cost = boundedPatternSum(cost, patternTreeCost(child))
		}
		return cost
	case syntax.OpRepeat:
		child := patternTreeCost(tree.Sub[0])
		if tree.Max == -1 {
			if tree.Min == 0 {
				return boundedPatternSum(child, 2)
			}
			return boundedPatternSum(boundedPatternProduct(child, tree.Min), 2)
		}
		if tree.Max == 0 {
			return 1
		}
		return boundedPatternSum(boundedPatternProduct(child, tree.Max), tree.Max-tree.Min)
	default:
		return MaxPatternInstructions + 1
	}
}

// boundedPatternSum saturates estimates so nested repetitions cannot overflow integers.
func boundedPatternSum(left, right int) int {
	if left > MaxPatternInstructions || right > MaxPatternInstructions-left {
		return MaxPatternInstructions + 1
	}
	return left + right
}

// boundedPatternProduct bounds multiplication before performing it.
func boundedPatternProduct(cost, count int) int {
	if count > 0 && cost > MaxPatternInstructions/count {
		return MaxPatternInstructions + 1
	}
	return cost * count
}
