package devtools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// testEvent is the bounded metadata needed from go test's JSON event stream.
type testEvent struct{ Action, Test, Output string }

// coverageBlock merges statement evidence without relying on rounded percentages.
type coverageBlock struct{ statements, count int }

// goCommand preserves the requesting process's toolchain and cache configuration.
func goCommand(root string, environment []string, arguments ...string) ([]byte, error) {
	command := exec.Command("go", arguments...)
	command.Dir = root
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("go %s: %w\n%s", strings.Join(arguments, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// MergeCoverage combines root library blocks and rejects missing or conflicting evidence.
func MergeCoverage(profiles ...[]byte) ([]byte, int, int, error) {
	blocks := map[string]coverageBlock{}
	for _, profile := range profiles {
		scanner := bufio.NewScanner(bytes.NewReader(profile))
		if !scanner.Scan() || scanner.Text() != "mode: set" {
			return nil, 0, 0, errors.New("expected set coverage profile")
		}
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) != 3 {
				return nil, 0, 0, errors.New("invalid coverage block")
			}
			location := fields[0]
			relative := strings.TrimPrefix(location, "github.com/goforj/feature/")
			if strings.Contains(strings.SplitN(relative, ":", 2)[0], "/") {
				continue
			}
			var size, count int
			if _, err := fmt.Sscanf(fields[1]+" "+fields[2], "%d %d", &size, &count); err != nil || size < 0 || count < 0 {
				return nil, 0, 0, errors.New("invalid coverage counts")
			}
			previous, ok := blocks[location]
			if ok && previous.statements != size {
				return nil, 0, 0, errors.New("coverage source mismatch")
			}
			blocks[location] = coverageBlock{statements: size, count: max(previous.count, count)}
		}
		if err := scanner.Err(); err != nil {
			return nil, 0, 0, err
		}
	}
	keys := make([]string, 0, len(blocks))
	for key := range blocks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	output.WriteString("mode: set\n")
	total, covered := 0, 0
	for _, key := range keys {
		block := blocks[key]
		hit := 0
		total += block.statements
		if block.count > 0 {
			hit = 1
			covered += block.statements
		}
		fmt.Fprintf(&output, "%s %d %d\n", key, block.statements, hit)
	}
	if total == 0 {
		return nil, 0, 0, errors.New("no library coverage evidence")
	}
	return []byte(output.String()), total, covered, nil
}

// TestUnit runs instrumented examples and unit tests, recording evidence only when requested.
func TestUnit(root string, record bool, output io.Writer) error {
	artifacts := filepath.Join(root, ".artifacts")
	coverage := filepath.Join(artifacts, "example-coverage")
	if err := os.RemoveAll(coverage); err != nil {
		return err
	}
	if err := os.MkdirAll(coverage, 0755); err != nil {
		return err
	}
	environment := append(os.Environ(), "GOWORK=off", "FEATURE_EXAMPLE_COVERAGE_DIR="+coverage)
	events, runErr := goCommand(root, environment, "test", "-json", "-count=1", "-coverpkg=github.com/goforj/feature", "-coverprofile="+filepath.Join(artifacts, "root.cover"), "./...")
	if err := os.WriteFile(filepath.Join(artifacts, "unit.json"), events, 0644); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(events))
	passed := 0
	for decoder.More() {
		var event testEvent
		if err := decoder.Decode(&event); err != nil {
			return err
		}
		if runErr != nil {
			fmt.Fprint(output, event.Output)
		}
		if event.Action == "pass" && event.Test != "" {
			passed++
		}
	}
	if runErr != nil {
		return runErr
	}
	counters, err := filepath.Glob(filepath.Join(coverage, "covcounters.*"))
	if err != nil {
		return err
	}
	if len(counters) == 0 {
		return errors.New("example coverage was not recorded")
	}
	if _, err := goCommand(root, environment, "tool", "covdata", "textfmt", "-i="+coverage, "-o="+filepath.Join(artifacts, "examples.cover")); err != nil {
		return err
	}
	native, err := os.ReadFile(filepath.Join(artifacts, "root.cover"))
	if err != nil {
		return err
	}
	examples, err := os.ReadFile(filepath.Join(artifacts, "examples.cover"))
	if err != nil {
		return err
	}
	profile, total, covered, err := MergeCoverage(native, examples)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(artifacts, "unit.cover"), profile, 0644); err != nil {
		return err
	}
	if covered != total {
		return fmt.Errorf("coverage incomplete: %d/%d library statements", covered, total)
	}
	fingerprint, err := Fingerprint(root)
	if err != nil {
		return err
	}
	programs, err := filepath.Glob(filepath.Join(root, "examples", "*", "main.go"))
	if err != nil {
		return err
	}
	version, err := goCommand(root, environment, "version")
	if err != nil {
		return err
	}
	report := Report{CoveredStatements: covered, Statements: total, PassedTests: passed, ExecutedPrograms: len(programs), GoVersion: strings.TrimSpace(string(version)), SourceFingerprint: fingerprint, Scope: "github.com/goforj/feature runtime package; tooling and executable scaffolding are not runtime library code"}
	serialized, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	serialized = append(serialized, '\n')
	if err := os.WriteFile(filepath.Join(artifacts, "unit-results.json"), serialized, 0644); err != nil {
		return err
	}
	if record {
		if err := os.WriteFile(filepath.Join(root, "docs", "unit-results.json"), serialized, 0644); err != nil {
			return err
		}
	}
	fmt.Fprintf(output, "Passed %d test cases and %d programs; %d/%d library statements covered.\n", passed, len(programs), covered, total)
	return nil
}
