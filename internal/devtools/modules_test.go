package devtools

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModuleInventoryRejectsDrift prevents unlisted runtime or tooling modules from escaping checks.
func TestModuleInventoryRejectsDrift(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "scripts", "module-manifest.txt")
	if _, err := Modules(root); err == nil {
		t.Fatal("missing inventory accepted")
	}
	for _, source := range []string{"", "published driver/one\n", "invalid .\n", "published . extra\n", "published .\npublished .\n", "published .\npublished ../escape\n", "published .\npublished /absolute\n"} {
		if err := os.WriteFile(manifest, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Modules(root); err == nil {
			t.Fatal("invalid inventory accepted", source)
		}
	}
	if err := os.WriteFile(manifest, []byte("# modules\n\npublished .\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckModules(root); err == nil {
		t.Fatal("missing root module accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/root\ngo 1.24.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckModules(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "hidden"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hidden", "go.mod"), []byte("module example.test/hidden\ngo 1.24.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckModules(root); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatal(err)
	}
	if err := TestUnit(root, false, io.Discard); err == nil || !strings.Contains(err.Error(), "module inventory mismatch") {
		t.Fatal("unit coverage accepted an unlisted module", err)
	}
	if err := os.WriteFile(manifest, []byte("published .\ntooling hidden\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckModules(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".git", ".artifacts"} {
		if err := os.MkdirAll(filepath.Join(root, name, "scratch"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "scratch", "go.mod"), []byte("module ignored"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckModules(root); err != nil {
		t.Fatal(err)
	}
}
