package feature

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"unicode/utf8"
)

// SchemaVersion identifies the supported document and rollout contracts.
const SchemaVersion = 1

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

// Flag defines a boolean default and ordered rules; Disabled forces false.
type Flag struct {
	Default  bool   `json:"default"`
	Disabled bool   `json:"disabled,omitempty"`
	Rules    []Rule `json:"rules,omitempty"`
}

// Rule returns Value when every supplied condition matches.
type Rule struct {
	Subjects   []string          `json:"subjects,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Rollout    *Rollout          `json:"rollout,omitempty"`
	Value      bool              `json:"value"`
}

// Rollout selects stable identity buckets using a required, non-secret salt.
type Rollout struct {
	BasisPoints int    `json:"basis_points"`
	Salt        string `json:"salt"`
}

// Subject identifies the rollout unit and exact string attributes for a decision.
type Subject struct {
	Key        string
	Attributes map[string]string
}

// compiled is private owned state that is never mutated after publication.
type compiled struct {
	document Document
	revision string
}

// compile validates before allocating an owned normalized document.
func compile(doc Document) (*compiled, error) {
	if doc.Schema != SchemaVersion || doc.Flags == nil || len(doc.Flags) > MaxFlags {
		return nil, ErrInvalidDocument
	}
	budget := 64
	for key, flag := range doc.Flags {
		if !validKey(key) || len(flag.Rules) > MaxRules {
			return nil, ErrInvalidDocument
		}
		budget += len(key)*6 + 64
		if budget > MaxDocumentBytes {
			return nil, ErrInvalidDocument
		}
		for _, rule := range flag.Rules {
			if len(rule.Subjects) > MaxSubjects || !validAttributes(rule.Attributes) ||
				(len(rule.Subjects) == 0 && len(rule.Attributes) == 0 && rule.Rollout == nil) {
				return nil, ErrInvalidDocument
			}
			budget += 128
			for _, subject := range rule.Subjects {
				if subject == "" || !validString(subject) {
					return nil, ErrInvalidDocument
				}
				budget += len(subject)*6 + 4
			}
			for key, value := range rule.Attributes {
				budget += (len(key)+len(value))*6 + 8
			}
			if rule.Rollout != nil {
				if rule.Rollout.BasisPoints < 0 || rule.Rollout.BasisPoints > 10000 ||
					rule.Rollout.Salt == "" || !validString(rule.Rollout.Salt) {
					return nil, ErrInvalidDocument
				}
				budget += len(rule.Rollout.Salt)*6 + 64
			}
			if budget > MaxDocumentBytes {
				return nil, ErrInvalidDocument
			}
		}
	}
	owned := cloneDocument(doc)
	// Only strings, booleans, and validated integers reach the encoder.
	data, _ := json.Marshal(owned)
	hash := sha256.Sum256(data)
	return &compiled{document: owned, revision: hex.EncodeToString(hash[:])}, nil
}

// cloneDocument normalizes unordered subject lists and owns every mutable input.
func cloneDocument(doc Document) Document {
	owned := Document{Schema: doc.Schema, Flags: make(map[string]Flag, len(doc.Flags))}
	for key, flag := range doc.Flags {
		copyFlag := Flag{Default: flag.Default, Disabled: flag.Disabled}
		for _, rule := range flag.Rules {
			copyRule := Rule{Value: rule.Value, Subjects: append([]string(nil), rule.Subjects...)}
			sort.Strings(copyRule.Subjects)
			if len(rule.Attributes) > 0 {
				copyRule.Attributes = make(map[string]string, len(rule.Attributes))
				for key, value := range rule.Attributes {
					copyRule.Attributes[key] = value
				}
			}
			if rule.Rollout != nil {
				rollout := *rule.Rollout
				copyRule.Rollout = &rollout
			}
			copyFlag.Rules = append(copyFlag.Rules, copyRule)
		}
		owned.Flags[key] = copyFlag
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
