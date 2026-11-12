package feature

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestJSONValueContainerCompatibility covers ordinary Go string maps and arrays without coercing keys.
func TestJSONValueContainerCompatibility(t *testing.T) {
	for _, value := range []any{map[string]string{"plan": "pro"}, []string{"admin"}} {
		if _, err := CloneValue(value); err != nil {
			t.Fatal(err)
		}
	}
	many := map[string]string{}
	for i := 0; i <= MaxSubjects; i++ {
		many[fmt.Sprint(i)] = "x"
	}
	for _, value := range []any{many, map[string]string{string([]byte{255}): "x"}, map[string]string{"x": strings.Repeat("x", MaxValueBytes)}, make([]string, MaxSubjects+1), []string{string([]byte{255})}} {
		if _, err := CloneValue(value); err != ErrInvalidValue {
			t.Fatal(err)
		}
	}
	if _, err := CopyValue(json.Number("1e999"), TypeFloat); err != ErrInvalidValue {
		t.Fatal(err)
	}
}
