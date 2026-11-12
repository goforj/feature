package feature

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDecodeStrictDocument rejects ambiguous or incomplete input without revealing its contents.
func TestDecodeStrictDocument(t *testing.T) {
	invalid := []string{
		"", "null", "[]", "true", "{", "{,", "{\"schema\"", "{\"schema\":", "{\"schema\":1,}",
		`{ "schema": 1, "flags": {} } {}`, `{ "schema": 1, "flags": {} } garbage`,
		`{ "schema": 1, "flags": {}, "Schema": 1 }`,
		`{ "schema": 1, "flags": {}, "schema": 1 }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "default": true}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "rules": [{"attributes":{"x":"a","x":"b"},"value":true}]}} }`,
		`{ "schema": 2, "flags": {} }`, `{ "flags": {} }`, `{ "schema": 1 }`,
		`{ "schema": 1, "flags": null }`, `{ "schema": 1, "flags": [] }`,
		`{ "schema": 1, "flags": {"checkout": {}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": null}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": "false"}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "typo": true}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "rules": [{"subjects":["staff"]}]}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "rules": [{"subjects":["staff"],"value":true,"typo":true}]}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "rules": [{"rollout":{"salt":"x"},"value":true}]}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "rules": [{"rollout":{"basis_points":1},"value":true}]}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "rules": [{"rollout":{"basis_points":1,"salt":"x","typo":true},"value":true}]}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": false, "rules": [{"value":true}]}} }`,
		`{ "schema": 1, "flags": {"checkout": {"default": {"bad":true}}} }`,
		strings.Repeat("[", 12) + strings.Repeat("]", 12),
		string([]byte{255}), strings.Repeat(" ", MaxDocumentBytes+1),
	}
	for _, input := range invalid {
		t.Run(input[:min(len(input), 80)], func(t *testing.T) {
			if _, err := Decode([]byte(input)); err != ErrInvalidDocument {
				t.Fatalf("got %v", err)
			}
		})
	}
	valid := []string{
		`{"schema":1,"flags":{}}`,
		`{"schema":1,"flags":{"checkout":{"default":false,"disabled":false,"rules":[{"subjects":["staff"],"attributes":{"plan":"pro"},"rollout":{"basis_points":10000,"salt":"v1"},"value":true}]}}}`,
	}
	for _, input := range valid {
		if _, err := Decode([]byte(input)); err != nil {
			t.Fatal(err)
		}
	}
}

// TestFileSourceRefresh uses real files, atomic replacement, deletion, and recovery.
func TestFileSourceRefresh(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "flags.json")
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(path, `{"schema":1,"flags":{"checkout":{"default":false}}}`)
	source := NewFileSource(path)
	c, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	old := c.Snapshot()
	write(path+".next", `{"schema":1,"flags":{"checkout":{"default":true}}}`)
	if err := os.Rename(path+".next", path); err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if !c.Enabled(ctx, "checkout", false, Subject{}) || old.Enabled(ctx, "checkout", true, Subject{}) {
		t.Fatal("replacement consistency failed")
	}
	before := c.Status().Revision
	write(path, `{"private":"credential"}`)
	if err := c.Refresh(ctx); err != ErrInvalidDocument || c.Status().Revision != before {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(ctx); err != ErrSource || c.Status().Revision != before {
		t.Fatal(err)
	}
	write(path, `{"schema":1,"flags":{}}`)
	if err := c.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, path+".link"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileSource(path + ".link").Load(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileSource(dir).Load(ctx); err != ErrSource {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := source.Load(canceled); err != context.Canceled {
		t.Fatal(err)
	}
	write(path, strings.Repeat(" ", MaxDocumentBytes+1))
	if _, err := source.Load(ctx); err != ErrInvalidDocument {
		t.Fatal(err)
	}
	write(path, `{"schema":1,"flags":{}}`)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	if os.Geteuid() != 0 {
		if _, err := source.Load(ctx); err != ErrSource {
			t.Fatal(err)
		}
	}
}

// TestReadFileFailures exercises real invalid descriptors and cancellation boundaries.
func TestReadFileFailures(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "flags.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"flags":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := readFile(canceled, file); err != context.Canceled {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readFile(ctx, file); err != ErrSource {
		t.Fatal(err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if _, err := readFile(ctx, directory); err != ErrSource {
		t.Fatal(err)
	}
	writeOnly, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writeOnly.Close()
	if _, err := readFile(ctx, writeOnly); !errors.Is(err, ErrSource) {
		t.Fatal(err)
	}
}

// FuzzDecode checks untrusted documents cannot panic or escape the validation contract.
func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"schema":1,"flags":{}}`))
	f.Add([]byte(`{"schema":1,"flags":{"a":{"default":false}}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Decode(data)
		if err != nil {
			if err != ErrInvalidDocument {
				t.Fatal(err)
			}
			return
		}
		if _, err := compile(doc); err != nil {
			t.Fatal(err)
		}
	})
}

// TestDecodeStructuralBounds rejects excessive inventories before typed decoding allocates them.
func TestDecodeStructuralBounds(t *testing.T) {
	docs := []Document{}
	flags := map[string]Flag{}
	for i := 0; i <= MaxFlags; i++ {
		flags[fmt.Sprint(i)] = Flag{}
	}
	docs = append(docs, Document{Schema: 1, Flags: flags})
	attrs := map[string]string{}
	for i := 0; i <= MaxAttributes; i++ {
		attrs[fmt.Sprint(i)] = "a"
	}
	docs = append(docs, Document{Schema: 1, Flags: map[string]Flag{"test": {Rules: []Rule{{Attributes: attrs}}}}})
	rules := make([]Rule, MaxRules+1)
	for i := range rules {
		rules[i] = Rule{Subjects: []string{"tenant"}}
	}
	docs = append(docs, Document{Schema: 1, Flags: map[string]Flag{"test": {Rules: rules}}})
	subjects := make([]string, MaxSubjects+1)
	for i := range subjects {
		subjects[i] = "tenant"
	}
	docs = append(docs, Document{Schema: 1, Flags: map[string]Flag{"test": {Rules: []Rule{{Subjects: subjects}}}}})
	for _, doc := range docs {
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(data); err != ErrInvalidDocument {
			t.Fatal(err)
		}
	}
}
