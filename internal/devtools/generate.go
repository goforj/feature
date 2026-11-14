package devtools

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry associates a documented API with its authoritative executable snippet.
type Entry struct{ Group, Name, Marker, Example string }

// Entries returns the curated consumer-facing API inventory.
func Entries() []Entry {
	return []Entry{
		{"Portable evaluation", "New", "portable", "portable"},
		{"Portable evaluation", "Features.For", "portable", "portable"},
		{"Portable evaluation", "Features.Enabled", "portable", "portable"},
		{"Portable evaluation", "Scope.Evaluate", "portable", "portable"},
		{"Typed evaluation", "Features.String", "typed-values", "typed"},
		{"Typed evaluation", "Features.Int", "typed-values", "typed"},
		{"Typed evaluation", "Features.Float", "typed-values", "typed"},
		{"Typed evaluation", "Features.Object", "typed-values", "typed"},
		{"Typed evaluation", "ObjectAs", "typed-decoding", "typed"},
		{"Definitions", "Validate", "validation", "validation"},
		{"Native rules", "WithClock", "native-rules", "rules"},
		{"Observability", "WithEventSink", "events", "events"},
		{"Observability", "WithObserver", "events", "events"},
		{"Observability", "Features.SupportsTracking", "events", "events"},
		{"Observability", "Features.Track", "events", "events"},
		{"Observability", "Scope.Track", "events", "events"},
		{"Changes", "Subscribe", "events", "events"},
		{"HTTP", "NewHTTPSource", "http-sources", "http"},
		{"HTTP", "NewHTTPExporter", "http-sources", "http"},
		{"HTTP", "WithNotifier", "http-sources", "http"},
		{"LaunchDarkly", "launchdarklyfeature.New", "launchdarkly-borrowed", "launchdarkly"},
		{"LaunchDarkly", "launchdarklyfeature.Open", "launchdarkly-owned", "launchdarkly"},
		{"OpenFeature", "openfeaturefeature.Open", "openfeature-owned", "openfeature"},
		{"OpenFeature", "openfeaturefeature.New", "openfeature-borrowed", "openfeature"},
		{"OpenFeature", "openfeaturefeature.NewProvider", "native-provider", "native-provider"},
		{"Construction", "NewMemorySource", "construction", "quickstart"},
		{"Construction", "Open", "construction", "quickstart"},
		{"Native compatibility", "Enabled", "enabled", "quickstart"},
		{"Native compatibility", "Evaluate", "evaluate", "quickstart"},
		{"Consistency", "Features.Snapshot", "snapshot", "refresh"},
		{"Sources", "Decode", "decode", "targeting"},
		{"Sources", "NewFileSource", "file", "refresh"},
		{"Refresh", "Replace", "replace", "polling"},
		{"Refresh", "Refresh", "refresh", "refresh"},
		{"Refresh", "Run", "run", "polling"},
		{"Inspection", "Status", "status", "refresh"},
	}
}

// Report records actual successful execution rather than declared test counts.
type Report struct {
	CoveredStatements int    `json:"covered_statements"`
	ExecutedPrograms  int    `json:"executed_programs"`
	GoVersion         string `json:"go_version"`
	PassedTests       int    `json:"passed_tests"`
	Scope             string `json:"scope"`
	SourceFingerprint string `json:"source_fingerprint"`
	Statements        int    `json:"statements"`
}

// Fingerprint binds recorded evidence to library, examples, and their maintenance tooling.
func Fingerprint(root string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(filepath.Join(root, "examples"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (strings.HasSuffix(path, ".go") || entry.Name() == "go.mod" || entry.Name() == "go.sum") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	for _, directory := range []string{"internal/devtools", "tools"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !entry.IsDir() && (strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".go.txt")) {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	err = filepath.WalkDir(filepath.Join(root, "driver"), func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() && (strings.HasSuffix(path, ".go") || entry.Name() == "go.mod" || entry.Name() == "go.sum") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	paths = append(paths, filepath.Join(root, "go.mod"), filepath.Join(root, "scripts", "module-manifest.txt"))
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(filepath.ToSlash(relative)))
		hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Snippet extracts one function-body region without duplicating its implementation.
func Snippet(root, example, marker string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "examples", example, "main.go"))
	if err != nil {
		return "", err
	}
	start, end := "// snippet:start "+marker, "// snippet:end "+marker
	var lines []string
	active := false
	starts, ends := 0, 0
	for _, line := range strings.Split(string(data), "\n") {
		switch strings.TrimSpace(line) {
		case start:
			starts++
			active = true
		case end:
			ends++
			active = false
		default:
			if active {
				lines = append(lines, strings.TrimPrefix(line, "\t"))
			}
		}
	}
	if starts != 1 || ends != 1 || len(lines) == 0 || active {
		return "", fmt.Errorf("expected exactly one complete snippet: %s", marker)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

// GoDoc reads comments attached to actual exported functions and methods using Go's parser.
func GoDoc(root, name string) (string, error) {
	receiver, symbol, qualified := strings.Cut(name, ".")
	if !qualified {
		symbol = name
	}
	paths, err := filepath.Glob(filepath.Join(root, "*.go"))
	if qualified && (receiver == "launchdarklyfeature" || receiver == "openfeaturefeature") {
		paths, err = filepath.Glob(filepath.Join(root, "driver", receiver, "*.go"))
		qualified = false
	}
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			return "", err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != symbol {
				continue
			}
			if qualified {
				if function.Recv == nil {
					continue
				}
				typeExpr := function.Recv.List[0].Type
				if pointer, ok := typeExpr.(*ast.StarExpr); ok {
					typeExpr = pointer.X
				}
				identifier, ok := typeExpr.(*ast.Ident)
				if !ok || identifier.Name != receiver {
					continue
				}
			}
			text := strings.Join(strings.Fields(function.Doc.Text()), " ")
			if !strings.HasPrefix(text, symbol+" ") {
				return "", fmt.Errorf("missing name-leading GoDoc: %s", name)
			}
			return text, nil
		}
	}
	return "", fmt.Errorf("unknown public API: %s", name)
}

// Badge renders compact repository-owned SVG status metadata.
func Badge(label, value, color string) string {
	left, right := len(label)*6+12, len(value)*6+12
	width := left + right
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">
<title>%s: %s</title><clipPath id="r"><rect width="%d" height="20" rx="3"/></clipPath><g clip-path="url(#r)"><path fill="#555" d="M0 0h%dv20H0z"/><path fill="%s" d="M%d 0h%dv20H%dz"/></g><g fill="#fff" text-anchor="middle" font-family="Verdana, sans-serif" font-size="11"><text x="%g" y="14">%s</text><text x="%g" y="14">%s</text></g></svg>
`, width, html.EscapeString(label), html.EscapeString(value), html.EscapeString(label), html.EscapeString(value), width, left, color, left, right, left, float64(left)/2, html.EscapeString(label), float64(left)+float64(right)/2, html.EscapeString(value))
}

// section groups APIs demonstrated by the same canonical snippet.
type section struct {
	group, marker, example, code string
	names, docs                  []string
}

// Generate verifies evidence and projects GoDoc and runnable snippets into README and badges.
func Generate(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "docs", "unit-results.json"))
	if err != nil {
		return err
	}
	var evidence Report
	if err := json.Unmarshal(data, &evidence); err != nil {
		return err
	}
	fingerprint, err := Fingerprint(root)
	if err != nil {
		return err
	}
	if evidence.SourceFingerprint != fingerprint {
		return errors.New("unit evidence is stale; run make record-unit")
	}
	if evidence.Statements == 0 || evidence.CoveredStatements != evidence.Statements {
		return errors.New("library unit coverage must be exactly 100%")
	}
	var groups []string
	index := map[string][]string{}
	var sections []*section
	for _, entry := range Entries() {
		code, err := Snippet(root, entry.Example, entry.Marker)
		if err != nil {
			return err
		}
		if !demonstrates(code, entry.Name) {
			return fmt.Errorf("%s does not demonstrate %s", entry.Marker, entry.Name)
		}
		description, err := GoDoc(root, entry.Name)
		if err != nil {
			return err
		}
		if _, ok := index[entry.Group]; !ok {
			groups = append(groups, entry.Group)
		}
		index[entry.Group] = append(index[entry.Group], fmt.Sprintf("[%s](#api-%s)", entry.Name, strings.ToLower(entry.Name)))
		var current *section
		for _, candidate := range sections {
			if candidate.group == entry.Group && candidate.marker == entry.Marker && candidate.example == entry.Example {
				current = candidate
				break
			}
		}
		if current == nil {
			current = &section{group: entry.Group, marker: entry.Marker, example: entry.Example, code: code}
			sections = append(sections, current)
		}
		current.names = append(current.names, entry.Name)
		current.docs = append(current.docs, description)
	}
	var table strings.Builder
	table.WriteString("| Group | Functions |\n| ---: | :--- |\n")
	for _, group := range groups {
		fmt.Fprintf(&table, "| **%s** | %s |\n", group, strings.Join(index[group], " · "))
	}
	var examples []string
	previous := ""
	for _, current := range sections {
		if current.group != previous {
			examples = append(examples, "## "+current.group)
			previous = current.group
		}
		for _, name := range current.names {
			examples = append(examples, fmt.Sprintf(`<a id="api-%s"></a>`, strings.ToLower(name)))
		}
		examples = append(examples, "### "+strings.Join(current.names, " · "), strings.Join(current.docs, "\n\n"), "```go\n"+current.code+"\n```")
	}
	template, err := os.ReadFile(filepath.Join(root, "docs", "README.tmpl.md"))
	if err != nil {
		return err
	}
	construction, err := Snippet(root, "quickstart", "construction")
	if err != nil {
		return err
	}
	values, err := Snippet(root, "quickstart", "quickstart-values")
	if err != nil {
		return err
	}
	refresh, err := Snippet(root, "quickstart", "quickstart-refresh")
	if err != nil {
		return err
	}
	readme := strings.NewReplacer("{{VERSION}}", releaseVersion, "{{QUICKSTART}}", construction+"\n\n"+values, "{{QUICKSTART_REFRESH}}", refresh, "{{INDEX}}", strings.TrimSuffix(table.String(), "\n"), "{{EXAMPLES}}", strings.Join(examples, "\n\n")).Replace(string(template))
	if strings.Contains(readme, "{{") {
		return errors.New("unresolved template marker")
	}
	if strings.ContainsRune(readme, 0x2014) {
		return errors.New("em dash is not permitted")
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0644); err != nil {
		return err
	}
	badges := []struct{ name, label, value, color string }{
		{"unit", "unit tests", fmt.Sprint(evidence.PassedTests), "#4c1"},
		{"coverage", "library coverage", "100%", "#4c1"},
		{"status", "release", releaseVersion, "#007ec6"},
		{"ci", "CI", "prepared", "#777"},
		{"security", "Security", "prepared", "#777"},
		{"license", "license", "MIT", "#007ec6"},
		{"go", "Go", "1.24+", "#007ec6"},
	}
	for _, item := range badges {
		if err := os.WriteFile(filepath.Join(root, "docs", "assets", item.name+".svg"), []byte(Badge(item.label, item.value, item.color)), 0644); err != nil {
			return err
		}
	}
	return nil
}

// demonstrates checks actual call expressions rather than matching comment or string text.
func demonstrates(code, name string) bool {
	qualifier, symbol, qualified := strings.Cut(name, ".")
	packageCall := qualified && (qualifier == "launchdarklyfeature" || qualifier == "openfeaturefeature")
	if qualified {
		name = symbol
	}
	file, err := parser.ParseFile(token.NewFileSet(), "snippet.go", "package snippet\nfunc example(){\n"+code+"\n}", 0)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.Ident:
			found = found || !packageCall && function.Name == name
		case *ast.SelectorExpr:
			if !packageCall {
				found = found || function.Sel.Name == name
			} else if receiver, ok := function.X.(*ast.Ident); ok {
				found = found || receiver.Name == qualifier && function.Sel.Name == name
			}
		}
		return true
	})
	return found
}

// GeneratedFiles captures checked-in projections for a two-pass parity check.
func GeneratedFiles(root string) (map[string][]byte, error) {
	paths, err := filepath.Glob(filepath.Join(root, "docs", "assets", "*.svg"))
	if err != nil {
		return nil, err
	}
	paths = append(paths, filepath.Join(root, "README.md"))
	files := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files[path] = data
	}
	return files, nil
}

// CheckGenerated requires both generations to preserve every existing projection byte.
func CheckGenerated(root string) error {
	before, err := GeneratedFiles(root)
	if err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		if err := Generate(root); err != nil {
			return err
		}
		after, err := GeneratedFiles(root)
		if err != nil {
			return err
		}
		if len(before) != len(after) {
			return errors.New("generated inventory changed")
		}
		for path, data := range before {
			if !bytes.Equal(data, after[path]) {
				return errors.New("generated files differ; run make generate")
			}
		}
	}
	return nil
}
