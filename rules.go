package feature

import (
	"math/big"
	"math/bits"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Operator identifies one bounded native targeting comparison.
type Operator string

const (
	// OpEqual matches an existing attribute with an equal typed value.
	OpEqual Operator = "eq"
	// OpNotEqual matches an existing attribute with a different typed value.
	OpNotEqual Operator = "ne"
	// OpIn matches an existing attribute in a configured list.
	OpIn Operator = "in"
	// OpNotIn excludes a configured list while requiring the attribute to exist.
	OpNotIn Operator = "not_in"
	// OpGreater compares numeric attributes without string conversion.
	OpGreater Operator = "gt"
	// OpGreaterOrEqual compares numeric attributes inclusively.
	OpGreaterOrEqual Operator = "ge"
	// OpLess compares numeric attributes without string conversion.
	OpLess Operator = "lt"
	// OpLessOrEqual compares numeric attributes inclusively.
	OpLessOrEqual Operator = "le"
	// OpContains tests string substrings or array membership.
	OpContains Operator = "contains"
	// OpPrefix tests a string prefix.
	OpPrefix Operator = "prefix"
	// OpSuffix tests a string suffix.
	OpSuffix Operator = "suffix"
	// OpMatches applies a precompiled RE2 regular expression.
	OpMatches Operator = "matches"
	// OpExists requires an attribute, including an explicitly null value.
	OpExists Operator = "exists"
	// OpMissing requires that an attribute is absent.
	OpMissing Operator = "missing"
)

// Condition compares a typed attribute; / paths use JSON Pointer escapes.
// $key and $kind address explicit identity fields rather than attributes.
type Condition struct {
	Attribute string   `json:"attribute"`
	Operator  Operator `json:"op"`
	Value     any      `json:"value,omitempty"`
	pattern   *regexp.Regexp
}

// Allocation assigns stable identity buckets to a named variant.
type Allocation struct {
	Variant     string `json:"variant"`
	BasisPoints int    `json:"basis_points"`
}

// Split partitions all 10000 identity buckets in explicit allocation order.
type Split struct {
	Salt        string       `json:"salt"`
	Allocations []Allocation `json:"allocations"`
}

// Progressive interpolates a rollout percentage between two absolute instants.
type Progressive struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	From  int       `json:"from"`
	To    int       `json:"to"`
}

// normalizeCondition validates comparison policy once before publication.
func normalizeCondition(condition Condition) (Condition, error) {
	if !validConditionName(condition.Attribute) {
		return Condition{}, ErrInvalidDocument
	}
	if strings.HasPrefix(condition.Attribute, "/") {
		for _, part := range strings.Split(condition.Attribute[1:], "/") {
			for i := 0; i < len(part); i++ {
				if part[i] == '~' {
					if i+1 == len(part) || (part[i+1] != '0' && part[i+1] != '1') {
						return Condition{}, ErrInvalidDocument
					}
					i++
				}
			}
		}
	}
	value, err := CloneValue(condition.Value)
	if err != nil {
		return Condition{}, ErrInvalidDocument
	}
	condition.Value = value
	switch condition.Operator {
	case OpEqual, OpNotEqual:
	case OpExists, OpMissing:
		if value != nil {
			return Condition{}, ErrInvalidDocument
		}
	case OpIn, OpNotIn:
		items, ok := value.([]any)
		if !ok || len(items) == 0 {
			return Condition{}, ErrInvalidDocument
		}
	case OpGreater, OpGreaterOrEqual, OpLess, OpLessOrEqual:
		if valueType(value) != TypeInteger && valueType(value) != TypeFloat {
			return Condition{}, ErrInvalidDocument
		}
	case OpContains:
	case OpPrefix, OpSuffix, OpMatches:
		text, ok := value.(string)
		if !ok || len(text) > MaxStringBytes {
			return Condition{}, ErrInvalidDocument
		}
		if condition.Operator == OpMatches {
			condition.pattern, err = regexp.Compile(text)
			if err != nil {
				return Condition{}, ErrInvalidDocument
			}
		}
	default:
		return Condition{}, ErrInvalidDocument
	}
	return condition, nil
}

// subjectValue resolves literal attributes or explicit JSON Pointer traversal.
func subjectValue(subject Subject, name string) (any, bool) {
	if name == "$key" {
		return subject.Key, subject.Key != ""
	}
	if name == "$kind" {
		return subject.kind, subject.kind != ""
	}
	if !strings.HasPrefix(name, "/") {
		if subject.values != nil {
			value, present := subject.values[name]
			return value, present
		}
		value, present := subject.Attributes[name]
		return value, present
	}
	var value any = subject.values
	if subject.values == nil {
		value = subject.Attributes
	}
	for _, part := range strings.Split(name[1:], "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch object := value.(type) {
		case map[string]any:
			next, present := object[key]
			if !present {
				return nil, false
			}
			value = next
		case map[string]string:
			next, present := object[key]
			if !present {
				return nil, false
			}
			value = next
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(object) || key != strconv.Itoa(index) {
				return nil, false
			}
			value = object[index]
		case []string:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(object) || key != strconv.Itoa(index) {
				return nil, false
			}
			value = object[index]
		default:
			return nil, false
		}
	}
	return value, true
}

// numericComparison compares normalized integers and floats without narrowing integers.
func numericComparison(left, right any) (int, bool) {
	var a, b big.Float
	switch value := left.(type) {
	case int64:
		a.SetInt64(value)
	case uint64:
		a.SetUint64(value)
	case float64:
		a.SetFloat64(value)
	default:
		return 0, false
	}
	switch value := right.(type) {
	case int64:
		b.SetInt64(value)
	case uint64:
		b.SetUint64(value)
	case float64:
		b.SetFloat64(value)
	default:
		return 0, false
	}
	return a.Cmp(&b), true
}

// equalValue treats numbers numerically while preserving all other JSON types.
func equalValue(left, right any) bool {
	if comparison, numeric := numericComparison(left, right); numeric {
		return comparison == 0
	}
	switch value := left.(type) {
	case map[string]any:
		other, ok := right.(map[string]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for key, item := range value {
			candidate, present := other[key]
			if !present || !equalValue(item, candidate) {
				return false
			}
		}
		return true
	case []any:
		other, ok := right.([]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for index, item := range value {
			if !equalValue(item, other[index]) {
				return false
			}
		}
		return true
	default:
		return left == right
	}
}

// conditionMatches requires presence for every comparison except explicit missing.
func conditionMatches(condition Condition, subject Subject) bool {
	actual, present := subjectValue(subject, condition.Attribute)
	if condition.Operator == OpExists {
		return present
	}
	if condition.Operator == OpMissing {
		return !present
	}
	if !present {
		return false
	}
	budget := 0
	actual, err := cloneValue(actual, 0, &budget, true)
	if err != nil {
		return false
	}
	switch condition.Operator {
	case OpEqual:
		return equalValue(actual, condition.Value)
	case OpNotEqual:
		return !equalValue(actual, condition.Value)
	case OpIn, OpNotIn:
		found := false
		for _, item := range condition.Value.([]any) {
			if equalValue(actual, item) {
				found = true
				break
			}
		}
		return found == (condition.Operator == OpIn)
	case OpGreater, OpGreaterOrEqual, OpLess, OpLessOrEqual:
		comparison, numeric := numericComparison(actual, condition.Value)
		if !numeric {
			return false
		}
		switch condition.Operator {
		case OpGreater:
			return comparison > 0
		case OpGreaterOrEqual:
			return comparison >= 0
		case OpLess:
			return comparison < 0
		default:
			return comparison <= 0
		}
	case OpContains:
		if items, ok := actual.([]any); ok {
			for _, item := range items {
				if equalValue(item, condition.Value) {
					return true
				}
			}
			return false
		}
		text, ok := actual.(string)
		needle, stringValue := condition.Value.(string)
		return ok && stringValue && strings.Contains(text, needle)
	case OpPrefix:
		text, ok := actual.(string)
		return ok && strings.HasPrefix(text, condition.Value.(string))
	case OpSuffix:
		text, ok := actual.(string)
		return ok && strings.HasSuffix(text, condition.Value.(string))
	default:
		text, ok := actual.(string)
		return ok && condition.pattern.MatchString(text)
	}
}

// progressiveBasis clamps endpoints and interpolates within a validated time interval.
func progressiveBasis(progress Progressive, at time.Time) int {
	if !at.After(progress.Start) {
		return progress.From
	}
	if !at.Before(progress.End) {
		return progress.To
	}
	delta := progress.To - progress.From
	if delta < 0 {
		delta = -delta
	}
	// The product can exceed 64 bits for long intervals, and exact division keeps
	// basis-point transitions independent of floating-point rounding.
	high, low := bits.Mul64(uint64(at.Sub(progress.Start)), uint64(delta))
	change, _ := bits.Div64(high, low, uint64(progress.End.Sub(progress.Start)))
	if progress.To < progress.From {
		return progress.From - int(change)
	}
	return progress.From + int(change)
}

// splitVariant preserves explicit allocation order and the stable native bucket encoding.
func splitVariant(split Split, key, identity string) string {
	position := int(bucket(key, split.Salt, identity))
	index := 0
	for index < len(split.Allocations)-1 && position >= split.Allocations[index].BasisPoints {
		position -= split.Allocations[index].BasisPoints
		index++
	}
	return split.Allocations[index].Variant
}

// validInstant rejects time values that cannot round-trip through native JSON.
func validInstant(at time.Time) bool {
	return !at.IsZero() && at.Year() >= 0 && at.Year() <= 9999 && at.UTC().Year() >= 0 && at.UTC().Year() <= 9999
}

// validConditionName preserves bounded Unicode attribute names without implicit normalization.
func validConditionName(name string) bool {
	return name != "" && len(name) <= MaxStringBytes && utf8.ValidString(name)
}
