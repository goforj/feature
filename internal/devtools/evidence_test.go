package devtools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnitRejectsConcurrentSourceChanges compiles old code, changes its source, and requires verification to reject mixed evidence.
func TestUnitRejectsConcurrentSourceChanges(t *testing.T) {
	root := t.TempDir()
	before := "// Package feature is a disposable verification fixture.\npackage feature\n// Enabled supplies one runtime statement.\nfunc Enabled() bool { return true }\n"
	after := strings.Replace(before, "return true", "return false", 1)
	testSource := fmt.Sprintf(`package feature
import (
 "os"
 "os/exec"
 "path/filepath"
 "testing"
)
// TestChange proves the tested package was compiled before its source changes.
func TestChange(t *testing.T) {
 if !Enabled() { t.Fatal("expected the original compiled behavior") }
 if err := os.WriteFile("feature.go", []byte(%q), 0600); err != nil { t.Fatal(err) }
 binary := filepath.Join(t.TempDir(), "example")
 build := exec.Command("go", "build", "-cover", "-coverpkg=github.com/goforj/feature/...", "-o", binary, "./examples/check")
 if output, err := build.CombinedOutput(); err != nil { t.Fatalf("build: %%v %%s",err,output) }
 run := exec.Command(binary)
 run.Env = append(os.Environ(), "GOCOVERDIR="+os.Getenv("FEATURE_EXAMPLE_COVERAGE_DIR"))
 if output, err := run.CombinedOutput(); err != nil { t.Fatalf("example: %%v %%s",err,output) }
}

`, after)
	files := map[string]string{
		"go.mod":                      "module github.com/goforj/feature\n\ngo 1.24.0\n",
		"feature.go":                  before,
		"feature_test.go":             testSource,
		"scripts/module-manifest.txt": "published .\n",
		"examples/check/main.go":      "// Command check supplies real subprocess coverage.\npackage main\nimport(\"fmt\";\"github.com/goforj/feature\")\n// main executes the newly compiled runtime.\nfunc main(){fmt.Println(feature.Enabled())}\n",
	}
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	err := TestUnit(root, false, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "source changed during unit verification") {
		if events, readErr := os.ReadFile(filepath.Join(root, ".artifacts", "unit.json")); readErr == nil {
			t.Log(string(events))
		}
		t.Fatalf("mixed source revisions were not rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".artifacts", "unit-results.json")); !os.IsNotExist(err) {
		t.Fatal("mixed evidence produced a successful report", err)
	}
}

// TestIntegrationRejectsConcurrentSourceChanges exercises the integration command boundary without external services.
func TestIntegrationRejectsConcurrentSourceChanges(t *testing.T) {
	t.Setenv("INTEGRATION_RUN", "^TestMutation$")
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                      "module github.com/goforj/feature\n\ngo 1.24.0\n",
		"feature.go":                  "package feature\n",
		"examples/check/main.go":      "package main\nfunc main() {}\n",
		"scripts/module-manifest.txt": "published .\ntooling integration\n",
		"integration/go.mod":          "module github.com/goforj/feature/integration\n\ngo 1.24.0\n",
		"integration/mutation_test.go": `package integration
import("os";"testing")
// TestMutation changes an input only after the integration process starts.
func TestMutation(t *testing.T) {
 if err := os.WriteFile("../feature.go", []byte("package feature\n// Changed marks an untested revision.\nconst Changed = true\n"), 0600); err != nil { t.Fatal(err) }
}
`,
	}
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	err := TestIntegration(root, false, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "source changed during integration verification") {
		t.Fatalf("mixed integration revisions were not rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".artifacts", "integration-results.json")); !os.IsNotExist(err) {
		t.Fatal("mixed evidence produced a successful integration report", err)
	}
}
