package devtools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture copies the actual authoritative sources into a disposable generator workspace.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	paths, err := filepath.Glob("../../*.go")
	if err != nil {
		t.Fatal(err)
	}
	examples, err := filepath.Glob("../../examples/*/main.go")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, examples...)
	drivers, err := filepath.Glob("../../driver/*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, drivers...)
	assets, err := filepath.Glob("../../docs/assets/*.svg")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, assets...)
	paths = append(paths, "../../go.mod", "../../scripts/module-manifest.txt", "../../docs/README.tmpl.md")
	for _, path := range paths {
		destination := filepath.Join(root, strings.TrimPrefix(filepath.ToSlash(path), "../../"))
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	recordFixture(t, root)
	return root
}

// recordFixture isolates generator tests with explicit synthetic test-only evidence.
func recordFixture(t *testing.T, root string) {
	t.Helper()
	fingerprint, err := Fingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(Report{Statements: 1, CoveredStatements: 1, PassedTests: 1, SourceFingerprint: fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "unit-results.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// change replaces one source fragment and requires the fixture to contain it.
func change(t *testing.T, root, path, old, next string) {
	t.Helper()
	target := filepath.Join(root, path)
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("missing fixture text: %s", old)
	}
	if err := os.WriteFile(target, []byte(strings.ReplaceAll(string(data), old, next)), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestGenerateNavigation proves each API has its own operation and unambiguous anchor.
func TestGenerateNavigation(t *testing.T) {
	root := fixture(t)
	if err := Generate(root); err != nil {
		t.Fatal(err)
	}
	if err := CheckGenerated(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	if strings.Contains(readme, "{{") {
		t.Fatal("unresolved marker")
	}
	quickstart := strings.SplitN(strings.SplitN(readme, "## Quick start\n", 2)[1], "\n## Available sources", 2)[0]
	for _, text := range []string{"flags.Enabled(", "flags.String(", "flags.Int(", "flags.Float(", "flags.Object(", "engine.Snapshot()", "source.Replace(document)", "engine.Refresh(ctx)", "// Theme: midnight\n\n", "// Live theme: sunrise\n\n", "// Captured request theme: midnight"} {
		if !strings.Contains(quickstart, text) {
			t.Fatal("quick start is missing a typed, refresh, or spacing demonstration", text)
		}
	}
	features := strings.SplitN(strings.SplitN(readme, "## Features\n", 2)[1], "\n## Quick start", 2)[0]
	if strings.Count(features, `width="18" height="18" alt="" align="absmiddle"`) != 4 ||
		strings.Count(features, `<td width="38%" valign="middle">`) != 4 || strings.Contains(features, "nowrap") {
		t.Fatal("feature table must preserve icon size, middle alignment, and wrapping labels")
	}
	for _, entry := range Entries() {
		anchor := "api-" + strings.ToLower(entry.Name)
		if strings.Count(readme, `<a id="`+anchor+`"></a>`) != 1 || !strings.Contains(readme, "["+entry.Name+"](#"+anchor+")") {
			t.Fatal("invalid anchor", entry.Name)
		}
		section := strings.SplitN(readme, `<a id="`+anchor+`"></a>`, 2)[1]
		code := strings.SplitN(section, "```go\n", 2)
		if len(code) != 2 || !demonstrates(strings.SplitN(code[1], "```", 2)[0], entry.Name) {
			t.Fatal("missing operation", entry.Name)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("outdated"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckGenerated(root); err == nil {
		t.Fatal("changed projection accepted")
	}
}

// TestGenerateFailures rejects drift in source evidence, snippets, and template ownership.
func TestGenerateFailures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{"stale", func(t *testing.T, root string) {
			change(t, root, "doc.go", "package feature", "// changed\npackage feature")
		}, "stale"},
		{"coverage", func(t *testing.T, root string) {
			change(t, root, "docs/unit-results.json", `"covered_statements":1`, `"covered_statements":0`)
		}, "exactly 100"},
		{"operation", func(t *testing.T, root string) {
			change(t, root, "examples/quickstart/main.go", "engine.Enabled(", "engine.Other(")
			recordFixture(t, root)
		}, "does not demonstrate"},
		{"snippet", func(t *testing.T, root string) {
			change(t, root, "examples/quickstart/main.go", "snippet:end enabled", "snippet:end absent")
			recordFixture(t, root)
		}, "complete snippet"},
		{"quickstart values", func(t *testing.T, root string) {
			change(t, root, "examples/quickstart/main.go", "snippet:end quickstart-values", "snippet:end absent")
			recordFixture(t, root)
		}, "complete snippet"},
		{"quickstart refresh", func(t *testing.T, root string) {
			change(t, root, "examples/quickstart/main.go", "snippet:end quickstart-refresh", "snippet:end absent")
			recordFixture(t, root)
		}, "complete snippet"},
		{"godoc", func(t *testing.T, root string) {
			change(t, root, "client.go", "// Open loads", "// Wrong loads")
			recordFixture(t, root)
		}, "name-leading"},
		{"template", func(t *testing.T, root string) { change(t, root, "docs/README.tmpl.md", "{{INDEX}}", "{{UNKNOWN}}") }, "unresolved"},
		{"style", func(t *testing.T, root string) {
			change(t, root, "docs/README.tmpl.md", "{{INDEX}}", string(rune(0x2014)))
		}, "em dash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			tc.mutate(t, root)
			if err := Generate(root); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal(err)
			}
		})
	}
}

// TestModuleInventoryInvalidatesEvidence prevents a changed validation scope from reusing old coverage claims.
func TestModuleInventoryInvalidatesEvidence(t *testing.T) {
	root := fixture(t)
	change(t, root, "scripts/module-manifest.txt", "# classification module-directory", "# changed module inventory")
	if err := Generate(root); err == nil || !strings.Contains(err.Error(), "unit evidence is stale") {
		t.Fatal("changed module inventory reused old unit evidence", err)
	}
}

// TestGoDocAndSnippetErrors verifies malformed inputs never invent API descriptions.
func TestGoDocAndSnippetErrors(t *testing.T) {
	root := fixture(t)
	if _, err := GoDoc(root, "MissingAPI"); err == nil {
		t.Fatal("unknown API accepted")
	}
	if _, err := Snippet(root, "quickstart", "missing"); err == nil {
		t.Fatal("missing snippet accepted")
	}
	if _, err := Snippet(root, "missing", "enabled"); err == nil {
		t.Fatal("missing program accepted")
	}
	change(t, root, "client.go", "package feature", "invalid Go syntax")
	if _, err := GoDoc(root, "Open"); err == nil {
		t.Fatal("invalid source accepted")
	}
	if _, err := Fingerprint(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing source accepted")
	}
}

// TestDemonstratesUsesAST ignores API-shaped strings and comments.
func TestDemonstratesUsesAST(t *testing.T) {
	for _, code := range []string{`fmt.Println("Enabled(")`, `// Enabled()`, `invalid code!`} {
		if demonstrates(code, "Enabled") {
			t.Fatal(code)
		}
	}
	if !demonstrates(`features.Enabled(ctx,"key",false,subject)`, "Enabled") || !demonstrates(`Open(ctx,source)`, "Open") {
		t.Fatal("valid call rejected")
	}
	for _, code := range []string{`feature.New(engine)`, `New(sdk)`, `other.New(sdk)`, `factory().New(sdk)`} {
		if demonstrates(code, "launchdarklyfeature.New") {
			t.Fatal("unrelated constructor matched a driver API", code)
		}
	}
	if !demonstrates(`launchdarklyfeature.New(sdk)`, "launchdarklyfeature.New") || !demonstrates(`flags.For(target)`, "Features.For") {
		t.Fatal("qualified call rejected")
	}
}

// TestBadgeEscapesMetadata keeps generated SVGs well-formed for display.
func TestBadgeEscapesMetadata(t *testing.T) {
	svg := Badge("a&b", "<value>", "#555")
	if !strings.Contains(svg, "a&amp;b") || !strings.Contains(svg, "&lt;value&gt;") {
		t.Fatal(svg)
	}
}

// TestMergeCoverage requires exact block identity and preserves subprocess coverage.
func TestMergeCoverage(t *testing.T) {
	a := []byte("mode: set\ngithub.com/goforj/feature/a.go:1.1,2.1 2 0\n")
	b := []byte("mode: set\ngithub.com/goforj/feature/a.go:1.1,2.1 2 1\ngithub.com/goforj/feature/examples/main.go:1.1,2.1 9 0\n")
	profile, total, covered, err := MergeCoverage(a, b)
	if err != nil || total != 2 || covered != 2 || !strings.Contains(string(profile), "2 1") {
		t.Fatal(string(profile), total, covered, err)
	}
	if _, total, covered, err := MergeCoverage(a, []byte("mode: set\ngithub.com/goforj/feature/searchtest/helper.go:1.1,2.1 3 0\n")); err != nil || total != 5 || covered != 0 {
		t.Fatal("unclassified packages must not disappear from coverage", total, covered, err)
	}
	for _, bad := range []string{"", "mode: count\n", "mode: set\nbad\n", "mode: set\ngithub.com/goforj/feature/a.go:1.1,2.1 -1 0\n", "mode: set\ngithub.com/goforj/feature/a.go:1.1,2.1 bad 0\n", "mode: set\ngithub.com/goforj/feature/a.go:1.1,2.1 3 1\n"} {
		if _, _, _, err := MergeCoverage(a, []byte(bad)); err == nil {
			t.Fatal("accepted invalid coverage", bad)
		}
	}
	if _, _, _, err := MergeCoverage([]byte("mode: set\n")); err == nil {
		t.Fatal("empty evidence accepted")
	}
}
