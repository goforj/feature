// Command consumercheck verifies public root and driver installations without local replacements.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// probes contains consumer-owned applications that exercise the selected public modules.
//
//go:embed testdata/*.go.txt
var probes embed.FS

// versionPattern rejects directive injection while accepting release and Go pseudo-versions.
var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// fixture defines an independently selected public dependency graph and its supported compiler.
type fixture struct {
	name, minimum, toolchain string
	drivers                  []string
}

// module records selected public versions and checksum evidence.
type module struct {
	Path, Version, Sum, GoModSum string
	Replace                      *module `json:",omitempty"`
}

// result records one executed compiler and behavior check.
type result struct {
	Consumer  string
	GoVersion string
	Modules   []module
}

// main keeps publication verification separate from ordinary source tests.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run verifies isolated public installations while leaving persistent Go and Git settings alone.
func run() error {
	version := flag.String("version", "", "root version or public pseudo-version")
	driverVersion := flag.String("drivers", "", "driver version; defaults to root version")
	minimumRoot := flag.String("minimum-root", "", "expected adapter-only root selection; defaults to root version")
	only := flag.String("only", "", "one consumer: native, launchdarkly, openfeature, or combined")
	report := flag.String("report", ".artifacts/public-consumers.json", "execution report path")
	flag.Parse()
	if *version == "" {
		return fmt.Errorf("-version is required")
	}
	if *driverVersion == "" {
		*driverVersion = *version
	}
	if *minimumRoot == "" {
		*minimumRoot = *version
	}
	for _, version := range []string{*version, *driverVersion, *minimumRoot} {
		if !versionPattern.MatchString(version) {
			return fmt.Errorf("invalid module version %q", version)
		}
	}
	fixtures := []fixture{
		{"native", "1.24.0", "go1.24.13", nil},
		{"launchdarkly", "1.25.0", "go1.25.5", []string{"launchdarklyfeature"}},
		{"openfeature", "1.26.0", "go1.26.0", []string{"openfeaturefeature"}},
		{"combined", "1.27.4", "go1.27.4", []string{"launchdarklyfeature", "openfeaturefeature"}},
	}
	root, err := os.MkdirTemp("", "feature-public-consumers-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	var results []result
	for _, fixture := range fixtures {
		if *only != "" && *only != fixture.name {
			continue
		}
		directory := filepath.Join(root, fixture.name)
		if err := os.MkdirAll(directory, 0755); err != nil {
			return err
		}
		var requirements []string
		if fixture.name == "native" || fixture.name == "combined" {
			requirements = append(requirements, "github.com/goforj/feature "+*version)
		}
		for _, driver := range fixture.drivers {
			requirements = append(requirements, "github.com/goforj/feature/driver/"+driver+" "+*driverVersion)
		}
		data := fmt.Sprintf("module consumer.test/%s\n\ngo %s\n\nrequire (\n%s\n)\n", fixture.name, fixture.minimum, strings.Join(requirements, "\n"))
		if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(data), 0644); err != nil {
			return err
		}
		program, err := probes.ReadFile("testdata/" + fixture.name + ".go.txt")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, "main.go"), program, 0644); err != nil {
			return err
		}
		env := environment(filepath.Join(root, "modules"), fixture.toolchain)
		command := func(arguments ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", arguments...)
			cmd.Dir = directory
			cmd.Env = env
			output, err := cmd.CombinedOutput()
			if err != nil {
				return nil, fmt.Errorf("%s: go %s: %w\n%s", fixture.name, strings.Join(arguments, " "), err, output)
			}
			return output, nil
		}
		if _, err := command("mod", "tidy"); err != nil {
			return err
		}
		if _, err := command("run", "-race", "."); err != nil {
			return err
		}
		if _, err := command("mod", "verify"); err != nil {
			return err
		}
		selected, err := command("list", "-m", "-json", "all")
		if err != nil {
			return err
		}
		entry := result{Consumer: fixture.name}
		decoder := json.NewDecoder(strings.NewReader(string(selected)))
		found := false
		for {
			var selected module
			if err := decoder.Decode(&selected); err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			if selected.Replace != nil {
				return fmt.Errorf("%s selected a replacement for %s", fixture.name, selected.Path)
			}
			if selected.Path == "github.com/goforj/feature" {
				expected := *version
				if fixture.name != "native" && fixture.name != "combined" {
					expected = *minimumRoot
				}
				if selected.Version != expected {
					return fmt.Errorf("%s selected root %s, expected %s", fixture.name, selected.Version, expected)
				}
				found = true
			}
			if selected.Path == "github.com/goforj/feature" || strings.HasPrefix(selected.Path, "github.com/goforj/feature/driver/") {
				if selected.Path != "github.com/goforj/feature" && selected.Version != *driverVersion {
					return fmt.Errorf("%s selected driver %s, expected %s", fixture.name, selected.Version, *driverVersion)
				}
				if selected.Sum == "" || selected.GoModSum == "" {
					return fmt.Errorf("%s has no public checksum evidence", selected.Path)
				}
				entry.Modules = append(entry.Modules, selected)
			}
		}
		if !found {
			return fmt.Errorf("%s did not select the root module", fixture.name)
		}
		compiler, err := command("version")
		if err != nil {
			return err
		}
		entry.GoVersion = strings.TrimSpace(string(compiler))
		results = append(results, entry)
		fmt.Printf("%s: passed (%s)\n", fixture.name, entry.GoVersion)
	}
	if len(results) == 0 {
		return fmt.Errorf("unknown consumer %q", *only)
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*report), 0755); err != nil {
		return err
	}
	return os.WriteFile(*report, append(data, '\n'), 0644)
}

// environment forces normal public proxy and checksum verification in a fresh module cache.
func environment(cache, toolchain string) []string {
	settings := map[string]string{
		"GOENV": "off", "GOWORK": "off", "GOFLAGS": "", "GOPRIVATE": "", "GONOPROXY": "none", "GONOSUMDB": "none",
		"GOPROXY": "https://proxy.golang.org", "GOSUMDB": "sum.golang.org", "GOMODCACHE": cache, "GOTOOLCHAIN": toolchain,
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replace := settings[key]; !replace && key != "GH_TOKEN" && key != "GITHUB_TOKEN" {
			env = append(env, entry)
		}
	}
	for key, value := range settings {
		env = append(env, key+"="+value)
	}
	return env
}
