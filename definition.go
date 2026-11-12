package feature

import "encoding/json"

// valueBytes accounts for encoded typed values before a complete document is copied.
func valueBytes(value any) int { data, _ := json.Marshal(value); return len(data) }

// flagType preserves schema 1's boolean contract while typed definitions declare their type.
func flagType(flag Flag) ValueType {
	if flag.Type == "" {
		return TypeBoolean
	}
	return flag.Type
}

// normalizeFlag owns default and off-values before any snapshot can publish them.
func normalizeFlag(flag Flag, schema int) (Flag, error) {
	value := flag.Default
	if value == nil {
		value = false
	}
	owned, err := CloneValue(value)
	if err != nil {
		return Flag{}, err
	}
	kind := flag.Type
	if schema == SchemaVersion {
		if kind != "" || flag.Off != nil {
			return Flag{}, ErrInvalidDocument
		}
		kind = TypeBoolean
	} else if kind == "" {
		kind = valueType(owned)
	}
	owned, err = CopyValue(value, kind)
	if err != nil {
		return Flag{}, err
	}
	result := flag
	result.Default = owned
	if schema == TypedSchemaVersion {
		result.Type = kind
	}
	if flag.Off != nil {
		result.Off, err = CopyValue(flag.Off, kind)
		if err != nil {
			return Flag{}, err
		}
	} else if flag.Disabled && kind != TypeBoolean {
		return Flag{}, ErrInvalidDocument
	}
	for _, rule := range flag.Rules {
		value := rule.Value
		if value == nil && kind == TypeBoolean {
			value = false
		}
		if _, err := CopyValue(value, kind); err != nil {
			return Flag{}, err
		}
	}
	return result, nil
}
