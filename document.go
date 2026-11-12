package feature

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
	"unicode/utf8"
)

// SchemaVersion identifies the supported document and rollout contracts.
const SchemaVersion = 1

// TypedSchemaVersion enables typed defaults and off-values without changing schema 1.
const TypedSchemaVersion = 2

const (
	// MaxDocumentBytes bounds JSON input and compiled document size.
	MaxDocumentBytes = 1 << 20
	// MaxFlags bounds both snapshot size and the known flag inventory.
	MaxFlags = 1024
	// MaxRules bounds the ordered rules on one flag.
	MaxRules = 64
	// MaxSubjects bounds the exact identity list on one rule.
	MaxSubjects = 1024
	// MaxAttributes bounds targeting attributes on one rule or subject.
	MaxAttributes = 32
	// MaxStringBytes bounds identities, salts, and attribute values.
	MaxStringBytes = 1024
	// MaxKeyBytes bounds flag and attribute names.
	MaxKeyBytes = 128
)

var (
	// ErrInvalidDocument identifies a rejected complete document without exposing its contents.
	ErrInvalidDocument = errors.New("feature: invalid document")
	// ErrSource identifies source failures without exposing paths or source error text.
	ErrSource = errors.New("feature: source unavailable")
	// ErrNotFound identifies a missing flag, for which evaluation uses the caller fallback.
	ErrNotFound = errors.New("feature: flag not found")
	// ErrInvalidSubject identifies an oversized or invalid evaluation subject.
	ErrInvalidSubject = errors.New("feature: invalid subject")
	// ErrInvalidKey identifies an invalid flag lookup key.
	ErrInvalidKey = errors.New("feature: invalid flag key")
	// ErrAlreadyRunning identifies a second refresh loop on the same client.
	ErrAlreadyRunning = errors.New("feature: refresh loop already running")
	// ErrInvalidInterval identifies a refresh interval below the supported minimum.
	ErrInvalidInterval = errors.New("feature: refresh interval must be at least 10ms")
)

// Document is a complete replacement for a source's flag definitions.
type Document struct {
	Schema int             `json:"schema"`
	Flags  map[string]Flag `json:"flags"`
}

// Flag defines an explicit default and ordered rules of one value type.
// Schema 1 is boolean-only. Schema 2 infers Type when omitted; set TypeFloat
// explicitly when encoding integral floating-point definitions outside Encode.
// Disabled boolean flags default to false; other disabled types require Off.
type Flag struct {
	Default        any            `json:"default,omitempty"`
	Disabled       bool           `json:"disabled,omitempty"`
	Rules          []Rule         `json:"rules,omitempty"`
	Type           ValueType      `json:"type,omitempty"`
	Off            any            `json:"off,omitempty"`
	Variants       map[string]any `json:"variants,omitempty"`
	DefaultVariant string         `json:"default_variant,omitempty"`
	OffVariant     string         `json:"off_variant,omitempty"`
}

// Rule returns a value of its flag's type when every supplied condition matches.
type Rule struct {
	Subjects   []string          `json:"subjects,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Rollout    *Rollout          `json:"rollout,omitempty"`
	Value      any               `json:"value,omitempty"`
	Variant    string            `json:"variant,omitempty"`
	Split      *Split            `json:"split,omitempty"`
	Conditions []Condition       `json:"conditions,omitempty"`
	Start      *time.Time        `json:"start,omitempty"`
	End        *time.Time        `json:"end,omitempty"`
}

// Rollout selects stable identity buckets using a required, non-secret salt.
type Rollout struct {
	BasisPoints int          `json:"basis_points"`
	Salt        string       `json:"salt"`
	Progressive *Progressive `json:"progressive,omitempty"`
}

// Subject identifies the rollout unit and exact string attributes for a decision.
type Subject struct {
	Key        string
	Attributes map[string]string
	values     map[string]any
	kind       string
}

// compiled is private owned state that is never mutated after publication.
type compiled struct {
	document Document
	revision string
}

// compile validates before allocating an owned normalized document.
func compile(doc Document) (*compiled, error) {
	if (doc.Schema != SchemaVersion && doc.Schema != TypedSchemaVersion) || doc.Flags == nil || len(doc.Flags) > MaxFlags {
		return nil, ErrInvalidDocument
	}
	if err := validatePatternBudget(doc); err != nil {
		return nil, err
	}
	owned := Document{Schema: doc.Schema, Flags: make(map[string]Flag, len(doc.Flags))}
	budget := 64
	for key, flag := range doc.Flags {
		if !validKey(key) {
			return nil, ErrInvalidDocument
		}
		normalized, cost, err := normalizeFlag(flag, doc.Schema)
		if err != nil {
			return nil, ErrInvalidDocument
		}
		budget += len(key)*6 + cost
		if budget > MaxDocumentBytes {
			return nil, ErrInvalidDocument
		}
		owned.Flags[key] = normalized
	}
	data, err := encodeDocument(owned)
	if err != nil || len(data) > MaxDocumentBytes {
		return nil, ErrInvalidDocument
	}
	hash := sha256.Sum256(data)
	return &compiled{document: owned, revision: hex.EncodeToString(hash[:])}, nil
}

// cloneDocument owns all mutable definition data without changing canonical rule ordering.
func cloneDocument(doc Document) Document {
	owned := Document{Schema: doc.Schema, Flags: make(map[string]Flag, len(doc.Flags))}
	for key, flag := range doc.Flags {
		owned.Flags[key], _, _ = normalizeFlag(flag, doc.Schema)
	}
	return owned
}

// validKey keeps inventories portable and telemetry labels bounded.
func validKey(key string) bool {
	if key == "" || len(key) > MaxKeyBytes {
		return false
	}
	for _, char := range key {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}

// validString prevents inconsistent JSON normalization and bounds evaluation work.
func validString(value string) bool {
	return len(value) <= MaxStringBytes && utf8.ValidString(value)
}

// validAttributes keeps source and evaluation conditions on the same bounded contract.
func validAttributes(attributes map[string]string) bool {
	if len(attributes) > MaxAttributes {
		return false
	}
	for key, value := range attributes {
		if !validKey(key) || !validString(value) {
			return false
		}
	}
	return true
}
