package feature

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"unicode/utf8"
)

// ValueType identifies the requested flag value independently of a provider.
type ValueType string

const (
	// TypeBoolean identifies boolean flag values.
	TypeBoolean ValueType = "boolean"
	// TypeString identifies UTF-8 string flag values.
	TypeString ValueType = "string"
	// TypeInteger identifies signed 64-bit integer flag values.
	TypeInteger ValueType = "integer"
	// TypeFloat identifies finite floating-point flag values.
	TypeFloat ValueType = "float"
	// TypeObject identifies JSON objects and arrays.
	TypeObject ValueType = "object"
	// MaxValueBytes bounds an owned flag value independently of targeting data.
	MaxValueBytes = 64 << 10
)

var (
	// ErrInvalidValue identifies unsupported, non-finite, or oversized values.
	ErrInvalidValue = errors.New("feature: invalid value")
	// ErrUnsupported identifies a capability absent from the selected driver.
	ErrUnsupported = errors.New("feature: capability unsupported")
)

// Result explains a typed decision with provider-neutral metadata and safe errors.
type Result[T any] struct {
	Value    T
	Reason   Reason
	Revision string
	Variant  string
	Err      error
}

// CloneValue owns bounded JSON-compatible values and normalizes integers to int64.
// Mutable results never share containers with their input. Object numbers retain
// integers where possible; no integer is silently narrowed or converted to float.
func CloneValue(value any) (any, error) {
	budget := 0
	return cloneValue(value, 0, &budget)
}

// cloneValue accounts for allocation before recursively owning a JSON value.
func cloneValue(value any, depth int, budget *int) (any, error) {
	*budget += 16
	if depth > 8 || *budget > MaxValueBytes {
		return nil, ErrInvalidValue
	}
	switch value := value.(type) {
	case nil, bool, int64:
		return value, nil
	case string:
		*budget += len(value)
		if !utf8.ValidString(value) || *budget > MaxValueBytes {
			return nil, ErrInvalidValue
		}
		return value, nil
	case int:
		return int64(value), nil
	case int8:
		return int64(value), nil
	case int16:
		return int64(value), nil
	case int32:
		return int64(value), nil
	case uint:
		return cloneUnsigned(uint64(value))
	case uint8:
		return int64(value), nil
	case uint16:
		return int64(value), nil
	case uint32:
		return int64(value), nil
	case uint64:
		return cloneUnsigned(value)
	case float32:
		return cloneFloat(float64(value))
	case float64:
		return cloneFloat(value)
	case json.Number:
		if integer, err := strconv.ParseInt(string(value), 10, 64); err == nil {
			return integer, nil
		}
		if number, err := strconv.ParseFloat(string(value), 64); err == nil {
			return cloneFloat(number)
		}
		return nil, ErrInvalidValue
	case map[string]any:
		if len(value) > MaxSubjects {
			return nil, ErrInvalidValue
		}
		result := make(map[string]any, len(value))
		for key, item := range value {
			*budget += len(key)
			if !utf8.ValidString(key) {
				return nil, ErrInvalidValue
			}
			next, err := cloneValue(item, depth+1, budget)
			if err != nil {
				return nil, err
			}
			result[key] = next
		}
		return result, nil
	case []any:
		if len(value) > MaxSubjects {
			return nil, ErrInvalidValue
		}
		result := make([]any, len(value))
		for i, item := range value {
			next, err := cloneValue(item, depth+1, budget)
			if err != nil {
				return nil, err
			}
			result[i] = next
		}
		return result, nil
	default:
		return nil, ErrInvalidValue
	}
}

// cloneUnsigned refuses precision loss when normalizing unsigned integers.
func cloneUnsigned(value uint64) (any, error) {
	if value > math.MaxInt64 {
		return nil, ErrInvalidValue
	}
	return int64(value), nil
}

// cloneFloat rejects values that cannot be represented in JSON.
func cloneFloat(value float64) (any, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, ErrInvalidValue
	}
	return value, nil
}

// valueType separates scalar flags from structured JSON values after normalization.
func valueType(value any) ValueType {
	switch value.(type) {
	case bool:
		return TypeBoolean
	case string:
		return TypeString
	case int64:
		return TypeInteger
	case float64:
		return TypeFloat
	case map[string]any, []any:
		return TypeObject
	default:
		return ""
	}
}

// CopyValue owns a value of the requested type and rejects lossy coercion.
func CopyValue(value any, kind ValueType) (any, error) {
	if number, ok := value.(json.Number); ok && kind == TypeFloat {
		parsed, err := number.Float64()
		if err != nil {
			return nil, ErrInvalidValue
		}
		value = parsed
	}
	owned, err := CloneValue(value)
	if err != nil {
		return nil, err
	}
	if kind == TypeFloat {
		if integer, ok := owned.(int64); ok {
			if integer > 1<<53 || integer < -(1<<53) {
				return nil, ErrInvalidValue
			}
			owned = float64(integer)
		}
	}
	if valueType(owned) != kind {
		return nil, ErrTypeMismatch
	}
	return owned, nil
}
