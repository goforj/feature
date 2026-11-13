package feature

import (
	"fmt"
	"sort"
)

// ValidationError identifies an authoring problem without changing runtime error redaction.
// Path is a JSON Pointer into the document and can include a valid flag key. Problem
// is a fixed diagnostic without values or source locations. Inspect these fields
// explicitly in authoring tools; Error itself remains the safe ErrInvalidDocument text.
type ValidationError struct {
	Path    string
	Problem string
}

// Error keeps ordinary error logging free of definition names and contents.
func (*ValidationError) Error() string { return ErrInvalidDocument.Error() }

// Unwrap retains errors.Is compatibility with the runtime validation classification.
func (*ValidationError) Unwrap() error { return ErrInvalidDocument }

// Validate checks the same complete definition accepted by Open, Encode, and Replace.
// Invalid definitions return a ValidationError describing one identified problem.
// Diagnostics are deterministic and bounded, and never publish or mutate the document.
// Runtime source operations continue to return only ErrInvalidDocument.
func Validate(doc Document) error {
	if _, err := compile(doc); err == nil {
		return nil
	}
	if doc.Schema != SchemaVersion && doc.Schema != TypedSchemaVersion {
		return &ValidationError{Path: "/schema", Problem: "unsupported schema version"}
	}
	if doc.Flags == nil || len(doc.Flags) > MaxFlags {
		return &ValidationError{Path: "/flags", Problem: "flags must be present and within the inventory limit"}
	}
	keys := make([]string, 0, len(doc.Flags))
	for key := range doc.Flags {
		if !validKey(key) {
			return &ValidationError{Path: "/flags", Problem: "flag names must use the supported key grammar"}
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if issue := patternValidationIssue(doc, keys); issue != nil {
		return issue
	}
	for _, key := range keys {
		flag := doc.Flags[key]
		if _, _, err := normalizeFlag(flag, doc.Schema); err != nil {
			return flagValidationIssue(flag, doc.Schema, "/flags/"+key)
		}
	}
	return &ValidationError{Path: "/flags", Problem: "complete document exceeds its normalized encoding or aggregate budget"}
}

// patternValidationIssue locates bounded pattern preflight failures before diagnostic normalization.
func patternValidationIssue(doc Document, keys []string) *ValidationError {
	instructions, runes := 0, 0
	for _, key := range keys {
		flag := doc.Flags[key]
		path := "/flags/" + key + "/rules"
		if len(flag.Rules) > MaxRules {
			return &ValidationError{Path: path, Problem: "too many rules"}
		}
		for index, rule := range flag.Rules {
			path := fmt.Sprintf("%s/%d/conditions", path, index)
			if len(rule.Conditions) > MaxAttributes {
				return &ValidationError{Path: path, Problem: "too many conditions"}
			}
			for index, condition := range rule.Conditions {
				if condition.Operator != OpMatches {
					continue
				}
				path := fmt.Sprintf("%s/%d/value", path, index)
				text, ok := condition.Value.(string)
				if !ok {
					return &ValidationError{Path: path, Problem: "pattern must be a string"}
				}
				cost, entries, err := patternResources(text)
				if err != nil {
					return &ValidationError{Path: path, Problem: "pattern is invalid or exceeds compilation limits"}
				}
				instructions += cost
				runes += entries
				if instructions > MaxDocumentPatternInstructions || runes > MaxDocumentPatternRuneEntries {
					return &ValidationError{Path: path, Problem: "complete document exceeds the pattern compilation budget"}
				}
			}
		}
	}
	return nil
}

// flagValidationIssue narrows a compiler failure using the same flag, rule, and condition validators.
func flagValidationIssue(flag Flag, schema int, path string) *ValidationError {
	base := flag
	base.Rules = nil
	normalized, _, err := normalizeFlag(base, schema)
	if err != nil {
		if flag.DefaultVariant != "" {
			if _, present := flag.Variants[flag.DefaultVariant]; !present {
				return &ValidationError{Path: path + "/default_variant", Problem: "default variant must reference the flag's variant catalog"}
			}
		}
		if flag.OffVariant != "" {
			if _, present := flag.Variants[flag.OffVariant]; !present {
				return &ValidationError{Path: path + "/off_variant", Problem: "off variant must reference the flag's variant catalog"}
			}
		}
		return &ValidationError{Path: path, Problem: "default, off value, declared type, or variant catalog is invalid"}
	}
	for index, rule := range flag.Rules {
		if _, _, err := normalizeRule(rule, normalized, schema); err != nil {
			path := fmt.Sprintf("%s/rules/%d", path, index)
			for index, condition := range rule.Conditions {
				if _, err := normalizeCondition(condition); err != nil {
					return &ValidationError{Path: fmt.Sprintf("%s/conditions/%d", path, index), Problem: "condition attribute, operator, or operand is invalid"}
				}
			}
			if rule.Variant != "" {
				if _, present := normalized.Variants[rule.Variant]; !present {
					return &ValidationError{Path: path + "/variant", Problem: "rule variant must reference the flag's variant catalog"}
				}
			}
			return &ValidationError{Path: path, Problem: "rule action, targeting, schedule, or rollout is invalid"}
		}
	}
	return &ValidationError{Path: path, Problem: "flag exceeds its aggregate definition budget"}
}
