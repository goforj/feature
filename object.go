package feature

import (
	"bytes"
	"context"
	"encoding/json"
	"unicode/utf8"
)

// ObjectReader is the structured read boundary implemented by Features and Scope.
// Implementations must support concurrent calls and must not mutate their inputs.
type ObjectReader interface {
	Object(context.Context, string, any) Result[any]
}

// ObjectAs decodes an object or array flag into an application type using JSON tags.
// It preserves decision metadata and returns an owned fallback on resolution or decoding
// failure. Decoding errors are ErrInvalidValue, without field names or source contents.
// Fallbacks must encode as bounded JSON objects or arrays; unknown fields are ignored.
// Numbers in interface fields use json.Number to avoid silently rounding native integers.
// Concrete fields follow encoding/json conversions; use int64 or json.Number for exact
// integers. Results and owned fallbacks contain only their JSON-visible representation.
// Custom JSON marshalers and unmarshalers are application-owned code.
func ObjectAs[T any](ctx context.Context, reader ObjectReader, key string, fallback T) Result[T] {
	result := Result[T]{Value: fallback, Reason: ReasonError}
	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if key == "" || len(key) > MaxStringBytes || !utf8.ValidString(key) {
		result.Err = ErrInvalidKey
		return result
	}
	data, err := json.Marshal(fallback)
	if err != nil || len(data) > MaxValueBytes {
		result.Err = ErrInvalidValue
		return result
	}
	argument, err := CopyValue(decodeValue(data), TypeObject)
	if err != nil {
		result.Err = ErrInvalidValue
		return result
	}
	owned, err := decodeObject[T](data)
	if err != nil {
		result.Err = ErrInvalidValue
		return result
	}
	result.Value = owned
	decision := reader.Object(ctx, key, argument)
	result.Reason, result.Revision, result.Variant = decision.Reason, decision.Revision, decision.Variant
	if result.Reason == "" {
		result.Reason = ReasonUnknown
	}
	if decision.Err != nil {
		result.Err = safeError(decision.Err)
		result.Reason = ReasonError
		if result.Err == ErrNotFound {
			result.Reason = ReasonMissing
		}
		return result
	}
	value, err := CopyValue(decision.Value, TypeObject)
	if err != nil {
		result.Err = ErrInvalidValue
		result.Reason = ReasonError
		return result
	}
	// CopyValue has already restricted the result to finite, JSON-compatible containers.
	data, _ = json.Marshal(value)
	decoded, err := decodeObject[T](data)
	if err != nil {
		result.Err = ErrInvalidValue
		result.Reason = ReasonError
		return result
	}
	result.Value = decoded
	return result
}

// decodeObject preserves exact number text in interface fields while honoring application JSON types.
func decodeObject[T any](data []byte) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err := decoder.Decode(&value)
	return value, err
}
