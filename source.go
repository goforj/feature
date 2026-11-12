package feature

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"unicode/utf8"
)

// MemorySource owns a validated document for tests and embedded applications.
type MemorySource struct {
	mu       sync.RWMutex
	document Document
}

// NewMemorySource validates and copies a document without starting background work.
func NewMemorySource(doc Document) (*MemorySource, error) {
	state, err := compile(doc)
	if err != nil {
		return nil, err
	}
	return &MemorySource{document: state.document}, nil
}

// Replace validates a complete replacement without changing any client's snapshot.
// Clients adopt the new document on their next successful refresh.
func (s *MemorySource) Replace(doc Document) error {
	state, err := compile(doc)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.document = state.document
	s.mu.Unlock()
	return nil
}

// Load returns an owned copy so consumers cannot mutate the source's document.
func (s *MemorySource) Load(ctx context.Context) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneDocument(s.document), nil
}

// FileSource reads a bounded JSON document from a regular file or regular-file symlink.
// Local filesystem calls cannot be interrupted; cancellation is checked at their boundaries.
type FileSource struct {
	path string
}

// NewFileSource captures a path without accessing the filesystem.
// Use a stable absolute path when application code may change its working directory.
func NewFileSource(path string) *FileSource {
	return &FileSource{path: path}
}

// Load follows atomic file replacements and redacts filesystem errors and paths.
func (s *FileSource) Load(ctx context.Context) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	// Reject devices and pipes before opening; the second check validates the opened file.
	info, err := os.Stat(s.path)
	if err != nil || !info.Mode().IsRegular() {
		return Document{}, ErrSource
	}
	file, err := os.Open(s.path)
	if err != nil {
		return Document{}, ErrSource
	}
	defer file.Close()
	return readFile(ctx, file)
}

// readFile bounds bytes even when the source grows between stat and read.
func readFile(ctx context.Context, file *os.File) (Document, error) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Document{}, ErrSource
	}
	if info.Size() > MaxDocumentBytes {
		return Document{}, ErrInvalidDocument
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxDocumentBytes+1))
	if err != nil {
		return Document{}, ErrSource
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	return Decode(data)
}

// wireDocument preserves required-field presence until JSON validation completes.
type wireDocument struct {
	Schema int                 `json:"schema"`
	Flags  map[string]wireFlag `json:"flags"`
}

// wireFlag distinguishes an explicit false default from an omitted default.
type wireFlag struct {
	Default  json.RawMessage `json:"default"`
	Disabled bool            `json:"disabled"`
	Rules    []wireRule      `json:"rules"`
	Type     ValueType       `json:"type"`
	Off      json.RawMessage `json:"off"`
}

// wireRule preserves required values and rollout percentages while decoding.
type wireRule struct {
	Subjects   []string          `json:"subjects"`
	Attributes map[string]string `json:"attributes"`
	Rollout    *wireRollout      `json:"rollout"`
	Value      json.RawMessage   `json:"value"`
}

// wireRollout prevents an omitted percentage from silently becoming zero.
type wireRollout struct {
	BasisPoints *int   `json:"basis_points"`
	Salt        string `json:"salt"`
}

// Encode validates and serializes normalized definitions with stable type annotations.
// Use Encode when persisting Go documents; it preserves boolean zero values and
// distinguishes integral floating-point defaults from integer defaults.
func Encode(doc Document) ([]byte, error) {
	state, err := compile(doc)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(state.document)
	return data, nil
}

// Decode validates bounded strict JSON, including duplicate keys and required fields.
// Errors never include document text. The returned document is owned by the caller.
func Decode(data []byte) (Document, error) {
	if len(data) > MaxDocumentBytes || !utf8.Valid(data) {
		return Document{}, ErrInvalidDocument
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var header struct {
		Schema int `json:"schema"`
	}
	// This selects a scanner grammar only; the scanner and wire decoder validate the document.
	_ = json.Unmarshal(data, &header)
	kind := "document"
	if header.Schema == TypedSchemaVersion {
		kind = "document2"
	}
	if err := scanJSON(decoder, kind, 0); err != nil {
		return Document{}, ErrInvalidDocument
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Document{}, ErrInvalidDocument
	}
	var wire wireDocument
	if err := json.Unmarshal(data, &wire); err != nil {
		return Document{}, ErrInvalidDocument
	}
	doc := Document{Schema: wire.Schema}
	if wire.Flags != nil {
		doc.Flags = make(map[string]Flag, len(wire.Flags))
	}
	for key, flag := range wire.Flags {
		if flag.Default == nil {
			return Document{}, ErrInvalidDocument
		}
		defaultValue := decodeValue(flag.Default)
		if defaultValue == nil {
			return Document{}, ErrInvalidDocument
		}
		value := Flag{Default: defaultValue, Disabled: flag.Disabled, Type: flag.Type}
		if flag.Off != nil {
			value.Off = decodeValue(flag.Off)
			if value.Off == nil {
				return Document{}, ErrInvalidDocument
			}
		}
		for _, rule := range flag.Rules {
			if rule.Value == nil {
				return Document{}, ErrInvalidDocument
			}
			ruleValue := decodeValue(rule.Value)
			if ruleValue == nil {
				return Document{}, ErrInvalidDocument
			}
			next := Rule{Subjects: rule.Subjects, Attributes: rule.Attributes, Value: ruleValue}
			if rule.Rollout != nil {
				if rule.Rollout.BasisPoints == nil {
					return Document{}, ErrInvalidDocument
				}
				next.Rollout = &Rollout{BasisPoints: *rule.Rollout.BasisPoints, Salt: rule.Rollout.Salt}
			}
			value.Rules = append(value.Rules, next)
		}
		doc.Flags[key] = value
	}
	state, err := compile(doc)
	if err != nil {
		return Document{}, err
	}
	return state.document, nil
}

// scanJSON rejects ambiguous keys, unexpected fields, nulls, and excessive nesting.
func scanJSON(decoder *json.Decoder, kind string, depth int) error {
	if depth > 16 {
		return ErrInvalidDocument
	}
	token, err := decoder.Token()
	if err != nil || (token == nil && kind != "value") {
		return ErrInvalidDocument
	}
	switch token {
	case json.Delim('{'):
		seen := make(map[string]bool)
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return ErrInvalidDocument
			}
			key := token.(string)
			if (kind == "flags" || kind == "flags2") && len(seen) >= MaxFlags || kind == "attributes" && len(seen) >= MaxAttributes || kind == "value" && len(seen) >= MaxSubjects {
				return ErrInvalidDocument
			}
			if seen[key] {
				return ErrInvalidDocument
			}
			seen[key] = true
			next, ok := fieldKind(kind, key)
			if !ok {
				return ErrInvalidDocument
			}
			if err := scanJSON(decoder, next, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case json.Delim('['):
		count := 0
		for decoder.More() {
			if (kind == "rules" || kind == "rules2") && count >= MaxRules || kind == "subjects" && count >= MaxSubjects || kind == "value" && count >= MaxSubjects {
				return ErrInvalidDocument
			}
			count++
			next := "scalar"
			if kind == "rules" {
				next = "rule"
			}
			if kind == "rules2" {
				next = "rule2"
			}
			if kind == "value" {
				next = "value"
			}
			if err := scanJSON(decoder, next, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return nil
	}
}

// fieldKind keeps structural field names case-sensitive while allowing dynamic inventories.
func fieldKind(kind, key string) (string, bool) {
	switch kind {
	case "document2":
		if key == "flags" {
			return "flags2", true
		}
		return "scalar", key == "schema"
	case "flags2":
		return "flag2", true
	case "flag2":
		if key == "default" || key == "off" {
			return "value", true
		}
		if key == "rules" {
			return "rules2", true
		}
		return "scalar", key == "type" || key == "disabled"
	case "rule2":
		if key == "value" {
			return "value", true
		}
		return key, key == "subjects" || key == "attributes" || key == "rollout"
	case "value":
		return "value", true
	case "document":
		return key, key == "schema" || key == "flags"
	case "flags":
		return "flag", true
	case "flag":
		return key, key == "default" || key == "disabled" || key == "rules"
	case "rule":
		return key, key == "subjects" || key == "attributes" || key == "rollout" || key == "value"
	case "rollout":
		return "scalar", key == "basis_points" || key == "salt"
	case "attributes":
		return "scalar", true
	default:
		return "", false
	}
}

// decodeValue retains numeric text until the declared flag type can validate it.
func decodeValue(data []byte) any {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	// The complete document was already checked by both the scanner and typed decoder.
	_ = decoder.Decode(&value)
	return value
}
