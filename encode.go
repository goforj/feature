package feature

import (
	"encoding/json"
	"strconv"
)

// encodedFloat retains floating-point syntax when JSON would otherwise emit an integer token.
type encodedFloat float64

// MarshalJSON preserves the numeric type of already validated finite values.
func (value encodedFloat) MarshalJSON() ([]byte, error) {
	return strconv.AppendFloat(nil, float64(value), 'e', -1, 64), nil
}

// encodedValue copies normalized containers while retaining their floating-point number syntax.
func encodedValue(value any) any {
	switch value := value.(type) {
	case float64:
		return encodedFloat(value)
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = encodedValue(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, item := range value {
			result[index] = encodedValue(item)
		}
		return result
	default:
		return value
	}
}

// encodeDocument preserves schema 1 bytes while making typed floating-point values round-trip.
func encodeDocument(doc Document) []byte {
	result := Document{Schema: doc.Schema, Flags: make(map[string]Flag, len(doc.Flags))}
	for key, flag := range doc.Flags {
		result.Flags[key] = encodedFlag(flag)
	}
	data, _ := json.Marshal(result)
	return data
}

// encodedFlag preserves numeric distinctions consistently in revisions, notifications, and persistence.
func encodedFlag(flag Flag) Flag {
	flag.Default = encodedValue(flag.Default)
	flag.Off = encodedValue(flag.Off)
	if flag.Variants != nil {
		variants := make(map[string]any, len(flag.Variants))
		for name, value := range flag.Variants {
			variants[name] = encodedValue(value)
		}
		flag.Variants = variants
	}
	flag.Rules = append([]Rule(nil), flag.Rules...)
	for index, rule := range flag.Rules {
		rule.Value = encodedValue(rule.Value)
		rule.Conditions = append([]Condition(nil), rule.Conditions...)
		for index := range rule.Conditions {
			rule.Conditions[index].Value = encodedValue(rule.Conditions[index].Value)
		}
		flag.Rules[index] = rule
	}
	return flag
}
