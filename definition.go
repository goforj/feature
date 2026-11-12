package feature

import (
	"bytes"
	"encoding/json"
	"sort"
)

// valueBytes accounts for bounded encoded values before copying a complete document.
func valueBytes(value any) int { data, _ := json.Marshal(value); return len(data) }

// flagType preserves schema 1's boolean contract while typed definitions declare their type.
func flagType(flag Flag) ValueType {
	if flag.Type == "" {
		return TypeBoolean
	}
	return flag.Type
}

// selectedValue resolves an explicit variant and rejects contradictory inline values.
func selectedValue(value any, name string, flag Flag) (any, error) {
	if name != "" {
		selected, present := flag.Variants[name]
		if !present {
			return nil, ErrInvalidDocument
		}
		if value != nil {
			normalized, err := CopyValue(value, flagType(flag))
			if err != nil {
				return nil, err
			}
			left, _ := json.Marshal(normalized)
			right, _ := json.Marshal(selected)
			if !bytes.Equal(left, right) {
				return nil, ErrInvalidDocument
			}
		}
		return CloneValue(selected)
	}
	if value == nil && flagType(flag) == TypeBoolean {
		value = false
	}
	return CopyValue(value, flagType(flag))
}

// normalizeFlag owns one complete bounded definition before publication.
func normalizeFlag(flag Flag, schema int) (Flag, int, error) {
	if len(flag.Rules) > MaxRules || len(flag.Variants) > MaxAttributes {
		return Flag{}, 0, ErrInvalidDocument
	}
	if schema == SchemaVersion && (flag.Type != "" || flag.Off != nil || flag.Variants != nil || flag.DefaultVariant != "" || flag.OffVariant != "") {
		return Flag{}, 0, ErrInvalidDocument
	}
	value := flag.Default
	if flag.DefaultVariant != "" {
		var present bool
		value, present = flag.Variants[flag.DefaultVariant]
		if !present {
			return Flag{}, 0, ErrInvalidDocument
		}
	}
	if value == nil {
		value = false
	}
	owned, err := CloneValue(value)
	if err != nil {
		return Flag{}, 0, err
	}
	kind := flag.Type
	if schema == SchemaVersion {
		kind = TypeBoolean
	} else if kind == "" {
		kind = valueType(owned)
	}
	result := flag
	result.Rules = nil
	result.Variants = nil
	if schema == TypedSchemaVersion {
		result.Type = kind
	}
	if flag.Variants != nil {
		if len(flag.Variants) == 0 {
			return Flag{}, 0, ErrInvalidDocument
		}
		result.Variants = make(map[string]any, len(flag.Variants))
		for name, value := range flag.Variants {
			if !validKey(name) {
				return Flag{}, 0, ErrInvalidDocument
			}
			next, err := CopyValue(value, kind)
			if err != nil {
				return Flag{}, 0, err
			}
			result.Variants[name] = next
		}
	}
	result.Default, err = selectedValue(flag.Default, flag.DefaultVariant, result)
	if err != nil {
		return Flag{}, 0, err
	}
	if flag.DefaultVariant != "" {
		result.Default = nil
	}
	if flag.Off != nil || flag.OffVariant != "" {
		result.Off, err = selectedValue(flag.Off, flag.OffVariant, result)
		if err != nil {
			return Flag{}, 0, err
		}
		if flag.OffVariant != "" {
			result.Off = nil
		}
	} else if flag.Disabled && kind != TypeBoolean {
		return Flag{}, 0, ErrInvalidDocument
	}
	budget := 64
	if schema == TypedSchemaVersion {
		budget += valueBytes(result.Default) + valueBytes(result.Off) + valueBytes(result.Variants)*6
	}
	for _, rule := range flag.Rules {
		next, cost, err := normalizeRule(rule, result, schema)
		if err != nil {
			return Flag{}, 0, err
		}
		budget += cost
		if budget > MaxDocumentBytes {
			return Flag{}, 0, ErrInvalidDocument
		}
		result.Rules = append(result.Rules, next)
	}
	if budget > MaxDocumentBytes {
		return Flag{}, 0, ErrInvalidDocument
	}
	return result, budget, nil
}

// normalizeRule validates conditions and allocations before owning their mutable data.
func normalizeRule(rule Rule, flag Flag, schema int) (Rule, int, error) {
	if len(rule.Subjects) > MaxSubjects || len(rule.Conditions) > MaxAttributes || !validAttributes(rule.Attributes) {
		return Rule{}, 0, ErrInvalidDocument
	}
	if schema == SchemaVersion && (rule.Variant != "" || rule.Split != nil || len(rule.Conditions) > 0 || rule.Start != nil || rule.End != nil || rule.Rollout != nil && rule.Rollout.Progressive != nil) {
		return Rule{}, 0, ErrInvalidDocument
	}
	if len(rule.Subjects) == 0 && len(rule.Attributes) == 0 && rule.Rollout == nil && len(rule.Conditions) == 0 && rule.Start == nil && rule.End == nil && rule.Split == nil {
		return Rule{}, 0, ErrInvalidDocument
	}
	result := rule
	result.Conditions = nil
	cost := 128
	for _, identity := range rule.Subjects {
		if identity == "" || !validString(identity) {
			return Rule{}, 0, ErrInvalidDocument
		}
		cost += len(identity)*6 + 4
	}
	for key, value := range rule.Attributes {
		cost += (len(key)+len(value))*6 + 8
	}
	if cost > MaxDocumentBytes {
		return Rule{}, 0, ErrInvalidDocument
	}
	if rule.Start != nil {
		if !validInstant(*rule.Start) {
			return Rule{}, 0, ErrInvalidDocument
		}
		at := rule.Start.Round(0).UTC()
		result.Start = &at
	}
	if rule.End != nil {
		if !validInstant(*rule.End) || rule.Start != nil && !rule.End.After(*rule.Start) {
			return Rule{}, 0, ErrInvalidDocument
		}
		at := rule.End.Round(0).UTC()
		result.End = &at
	}
	for _, condition := range rule.Conditions {
		normalized, err := normalizeCondition(condition)
		if err != nil {
			return Rule{}, 0, err
		}
		cost += valueBytes(normalized) * 6
		if cost > MaxDocumentBytes {
			return Rule{}, 0, ErrInvalidDocument
		}
		result.Conditions = append(result.Conditions, normalized)
	}
	if rule.Split != nil {
		if rule.Value != nil || rule.Variant != "" || !validString(rule.Split.Salt) || rule.Split.Salt == "" || len(rule.Split.Allocations) == 0 || len(rule.Split.Allocations) > MaxAttributes {
			return Rule{}, 0, ErrInvalidDocument
		}
		total := 0
		seen := map[string]bool{}
		for _, allocation := range rule.Split.Allocations {
			if allocation.BasisPoints < 0 || allocation.BasisPoints > 10000 || seen[allocation.Variant] {
				return Rule{}, 0, ErrInvalidDocument
			}
			if _, present := flag.Variants[allocation.Variant]; !present {
				return Rule{}, 0, ErrInvalidDocument
			}
			seen[allocation.Variant] = true
			total += allocation.BasisPoints
		}
		if total != 10000 {
			return Rule{}, 0, ErrInvalidDocument
		}
		result.Split = &Split{Salt: rule.Split.Salt, Allocations: append([]Allocation(nil), rule.Split.Allocations...)}
		cost += valueBytes(result.Split) * 6
	} else {
		value, err := selectedValue(rule.Value, rule.Variant, flag)
		if err != nil {
			return Rule{}, 0, err
		}
		result.Value = value
		if rule.Variant != "" {
			result.Value = nil
		}
		if schema == TypedSchemaVersion {
			cost += valueBytes(value)
		}
	}
	if rule.Rollout != nil {
		rollout := *rule.Rollout
		if rollout.BasisPoints < 0 || rollout.BasisPoints > 10000 || rollout.Salt == "" || !validString(rollout.Salt) {
			return Rule{}, 0, ErrInvalidDocument
		}
		if rollout.Progressive != nil {
			progress := *rollout.Progressive
			if !validInstant(progress.Start) || !validInstant(progress.End) || !progress.End.After(progress.Start) || progress.From < 0 || progress.From > 10000 || progress.To < 0 || progress.To > 10000 || !progress.Start.Add(progress.End.Sub(progress.Start)).Equal(progress.End) {
				return Rule{}, 0, ErrInvalidDocument
			}
			progress.Start = progress.Start.Round(0).UTC()
			progress.End = progress.End.Round(0).UTC()
			rollout.Progressive = &progress
		}
		result.Rollout = &rollout
		cost += len(rollout.Salt)*6 + 64
		if rollout.Progressive != nil {
			cost += valueBytes(rollout.Progressive)
		}
	}
	if cost > MaxDocumentBytes {
		return Rule{}, 0, ErrInvalidDocument
	}
	result.Subjects = append([]string(nil), rule.Subjects...)
	sort.Strings(result.Subjects)
	result.Attributes = nil
	if len(rule.Attributes) > 0 {
		result.Attributes = make(map[string]string, len(rule.Attributes))
		for key, value := range rule.Attributes {
			result.Attributes[key] = value
		}
	}
	return result, cost, nil
}
