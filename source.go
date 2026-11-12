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
	Default  *bool      `json:"default"`
	Disabled bool       `json:"disabled"`
	Rules    []wireRule `json:"rules"`
}

// wireRule preserves required values and rollout percentages while decoding.
type wireRule struct {
	Subjects   []string          `json:"subjects"`
	Attributes map[string]string `json:"attributes"`
	Rollout    *wireRollout      `json:"rollout"`
	Value      *bool             `json:"value"`
}

// wireRollout prevents an omitted percentage from silently becoming zero.
type wireRollout struct {
	BasisPoints *int   `json:"basis_points"`
	Salt        string `json:"salt"`
}

// Decode validates bounded strict JSON, including duplicate keys and required fields.
// Errors never include document text. The returned document is owned by the caller.
func Decode(data []byte) (Document, error) {
	if len(data) > MaxDocumentBytes || !utf8.Valid(data) {
		return Document{}, ErrInvalidDocument
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSON(decoder, "document", 0); err != nil {
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
		value := Flag{Default: *flag.Default, Disabled: flag.Disabled}
		for _, rule := range flag.Rules {
			if rule.Value == nil {
				return Document{}, ErrInvalidDocument
			}
			next := Rule{Subjects: rule.Subjects, Attributes: rule.Attributes, Value: *rule.Value}
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
	if depth > 8 {
		return ErrInvalidDocument
	}
	token, err := decoder.Token()
	if err != nil || token == nil {
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
		for decoder.More() {
			next := "scalar"
			if kind == "rules" {
				next = "rule"
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
