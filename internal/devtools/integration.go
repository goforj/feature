package devtools

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// integrationReport distinguishes actual local service execution from hosted verification and unit coverage.
type integrationReport struct {
	PassedTests       int             `json:"passed_tests"`
	GoVersion         string          `json:"go_version"`
	Race              bool            `json:"race"`
	SourceFingerprint string          `json:"source_fingerprint"`
	Versions          json.RawMessage `json:"versions"`
	Scope             string          `json:"scope"`
}

// IntegrationFingerprint includes runtime, harness, fixture versions, and module requirements.
func IntegrationFingerprint(root string) (string, error) {
	runtime, err := Fingerprint(root)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write([]byte(runtime))
	err = filepath.WalkDir(filepath.Join(root, "integration"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !(strings.HasSuffix(path, ".go") || entry.Name() == "go.mod" || entry.Name() == "go.sum" || entry.Name() == "versions.json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		hash.Write([]byte(filepath.ToSlash(relative)))
		hash.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// integrationCases rejects skipped or incomplete runs before recording complete support evidence.
func integrationCases(events []byte, full bool) (int, error) {
	passed := map[string]bool{}
	decoder := json.NewDecoder(bytes.NewReader(events))
	for decoder.More() {
		var event testEvent
		if err := decoder.Decode(&event); err != nil {
			return 0, err
		}
		if event.Action == "skip" || event.Action == "fail" {
			return 0, errors.New("integration tests failed or skipped required verification")
		}
		if event.Action == "pass" && event.Test != "" {
			passed[event.Test] = true
		}
	}
	if len(passed) == 0 {
		return 0, errors.New("no integration tests executed")
	}
	if full {
		for _, name := range []string{"native", "launchdarkly", "openfeature-launchdarkly", "openfeature-goff-remote", "openfeature-goff-inprocess"} {
			for _, scenario := range []string{"true", "false-is-success", "missing-false", "missing-true", "identity", "owned-target", "canceled", "concurrent", "live-update", "delete-and-recreate"} {
				if !passed["TestDriverConformance/"+name+"/"+scenario] {
					return 0, fmt.Errorf("missing integration contract case: %s/%s", name, scenario)
				}
			}
		}
		for _, name := range []string{"TestFileRefreshLifecycle", "TestLaunchDarklyStreamingAndEvents", "TestLaunchDarklyStartupFailures", "TestLaunchDarklyBorrowedOwnership", "TestRelayAuthentication", "TestRelayContainerOutage", "TestRelayTransportFailures", "TestRelayTypedTargeting", "TestOpenFeatureSDKTypeMismatch"} {
			if !passed[name] {
				return 0, fmt.Errorf("missing integration test: %s", name)
			}
		}
		for _, scenario := range []string{"configured-off-variation", "authentication", "container-outage-and-recovery", "analytics-on-close"} {
			if !passed["TestLaunchDarklyRelay/"+scenario] {
				return 0, fmt.Errorf("missing LaunchDarkly Relay integration: %s", scenario)
			}
		}
		if !passed["TestBackendVersionPins"] {
			return 0, errors.New("missing selected backend version verification")
		}
		for _, adapter := range []string{"direct", "openfeature"} {
			for _, scenario := range []string{"multi-identity", "typed-targeting", "off-variation", "privacy"} {
				if !passed["TestLaunchDarklyRelayContexts/"+adapter+"/"+scenario] {
					return 0, fmt.Errorf("missing Relay context verification: %s/%s", adapter, scenario)
				}
			}
		}
	}
	return len(passed), nil
}

// TestIntegration runs real required services with race instrumentation and separate coverage evidence.
func TestIntegration(root string, record bool, output io.Writer) error {
	filter := os.Getenv("INTEGRATION_RUN")
	if filter != "" && record {
		return errors.New("focused integration runs cannot overwrite complete evidence")
	}
	initialFingerprint, err := IntegrationFingerprint(root)
	if err != nil {
		return err
	}
	artifacts := filepath.Join(root, ".artifacts")
	if err := os.MkdirAll(artifacts, 0755); err != nil {
		return err
	}
	name := "integration"
	if filter != "" {
		name += "-focused"
	}
	arguments := []string{"test", "-tags=integration", "-race", "-timeout=5m", "-count=1", "-json", "-covermode=atomic",
		"-coverpkg=github.com/goforj/feature,github.com/goforj/feature/driver/launchdarklyfeature,github.com/goforj/feature/driver/openfeaturefeature",
		"-coverprofile=" + filepath.Join(artifacts, name+".cover")}
	if filter != "" {
		arguments = append(arguments, "-run="+filter)
	}
	arguments = append(arguments, "./...")
	events, runErr := goCommand(filepath.Join(root, "integration"), append(os.Environ(), "GOWORK=off"), arguments...)
	if err := os.WriteFile(filepath.Join(artifacts, name+".json"), events, 0644); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(events))
	for decoder.More() {
		var event testEvent
		if err := decoder.Decode(&event); err != nil {
			return err
		}
		fmt.Fprint(output, event.Output)
	}
	if runErr != nil {
		return runErr
	}
	passed, err := integrationCases(events, filter == "")
	if err != nil {
		return err
	}
	fingerprint, err := IntegrationFingerprint(root)
	if err != nil {
		return err
	}
	if fingerprint != initialFingerprint {
		return errors.New("source changed during integration verification; rerun against a stable source tree")
	}
	if filter != "" {
		fmt.Fprintf(output, "Passed %d focused integration cases; full-run evidence preserved.\n", passed)
		return nil
	}
	versions, err := os.ReadFile(filepath.Join(root, "integration", "versions.json"))
	if err != nil {
		return err
	}
	version, err := goCommand(root, os.Environ(), "version")
	if err != nil {
		return err
	}
	report := integrationReport{passed, strings.TrimSpace(string(version)), true, fingerprint, versions,
		"native files and memory; official LaunchDarkly SDK and OpenFeature LaunchDarkly provider through a real Docker LaunchDarkly Relay with a local FDv2 upstream fixture; real Docker GO Feature Flag relay in remote and in-process modes; SDK protocol failure fixtures. No hosted LaunchDarkly account verification; no integration coverage used for the unit target."}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(artifacts, "integration-results.json"), data, 0644); err != nil {
		return err
	}
	if record {
		if err := os.WriteFile(filepath.Join(root, "docs", "integration-results.json"), data, 0644); err != nil {
			return err
		}
	}
	fmt.Fprintf(output, "Passed %d integration cases with the race detector; coverage remains separate from unit coverage.\n", passed)
	return nil
}
