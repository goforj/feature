// Command appsmoke verifies feature adoption in a disposable rendered GoForj HTTP App.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// decision mirrors only the fake business response exposed by the smoke fixture.
type decision struct {
	PID        int      `json:"pid"`
	FirstTheme string   `json:"first_theme"`
	Theme      string   `json:"theme"`
	Checkout   bool     `json:"checkout"`
	Columns    int      `json:"columns"`
	Revisions  []string `json:"revisions"`
}

// health exposes the safe source classification and the held-request checkpoint.
type health struct {
	Revision string `json:"revision"`
	Error    string `json:"error"`
	Captured bool   `json:"captured"`
}

// report records observed adoption checks separately from library unit and integration coverage.
type report struct {
	Mode, FeatureVersion, Renderer, AppGoVersion string
	PID                                          int
	Checks                                       []string
}

// main retains failed smoke artifacts without changing any source checkout or persistent environment.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run renders and exercises a real App using its app-owned Wire providers and lifecycle hooks.
func run() error {
	forj := flag.String("forj", "forj", "renderer executable")
	version := flag.String("version", "", "public feature version or pseudo-version")
	dev := flag.Bool("dev", false, "exercise the default forj dev watchers")
	keep := flag.Bool("keep", false, "keep the successful disposable App")
	output := flag.String("report", ".artifacts/app-smoke.json", "observed smoke report")
	flag.Parse()
	if *version == "" {
		return errors.New("-version is required")
	}
	repo, err := os.Getwd()
	if err != nil {
		return err
	}
	binary, err := exec.LookPath(*forj)
	if err != nil {
		return err
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	project, err := os.MkdirTemp("/tmp", "feature-app-smoke-")
	if err != nil {
		return err
	}
	succeeded := false
	defer func() {
		if succeeded && !*keep {
			_ = os.RemoveAll(project)
		} else {
			fmt.Fprintln(os.Stderr, "Smoke project:", project)
		}
	}()
	configuration := "project_name: Feature Release Smoke\nmodule_name: example.com/feature-release-smoke\nrender:\n  components:\n    cli: true\n    web_api: true\n  starter_kit: none\n  agent_guidance: none\ndev:\n  pre: []\n  down: []\n  down_on_exit: false\n  sound_on_watch_error: false\n  apps:\n    app: {}\n"
	if err := os.WriteFile(filepath.Join(project, ".goforj.yml"), []byte(configuration), 0644); err != nil {
		return err
	}
	flagsFile := filepath.Join(project, "config", "flags.json")
	if err := publish(flagsFile, "blue", false, 2); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	env := append(os.Environ(), "GOENV=off", "GOWORK=off", "GOPRIVATE=", "GONOPROXY=none", "GONOSUMDB=none", "GOPROXY=https://proxy.golang.org", "GOSUMDB=sum.golang.org", "FEATURE_FLAGS_FILE="+flagsFile, "API_HTTP_HOST=127.0.0.1", fmt.Sprintf("API_HTTP_PORT=%d", port), "LIGHTHOUSE_ENABLED=false", "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = project
		cmd.Env = env
		data, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w\n%s", filepath.Base(name), strings.Join(args, " "), err, data)
		}
		return data, nil
	}
	fmt.Println("Rendering disposable App")
	if _, err := command(binary, "render"); err != nil {
		return err
	}
	fixture := filepath.Join(repo, "integration", "testdata", "goforj-smoke")
	if err := copyFixture(fixture, project); err != nil {
		return err
	}
	if _, err := command("go", "get", "github.com/goforj/feature@"+*version); err != nil {
		return err
	}
	if _, err := command(binary, "render"); err != nil {
		return err
	}
	if err := fixtureUnchanged(fixture, project); err != nil {
		return err
	}
	fmt.Println("Building injected App")
	if _, err := command(binary, "build"); err != nil {
		return err
	}
	selected, err := command("go", "list", "-m", "-json", "all")
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(selected))
	selectedFeature := ""
	for decoder.More() {
		var module struct {
			Path, Version string
			Replace       any
		}
		if err := decoder.Decode(&module); err != nil {
			return err
		}
		if module.Replace != nil {
			return fmt.Errorf("smoke selected a replacement for %s", module.Path)
		}
		if module.Path == "github.com/goforj/feature" {
			selectedFeature = module.Version
		}
	}
	if selectedFeature != *version {
		return fmt.Errorf("selected feature %s, expected %s", selectedFeature, *version)
	}
	renderer, err := command(binary, "--version")
	if err != nil {
		return err
	}
	appGo, err := command("go", "list", "-m", "-f", "{{.GoVersion}}")
	if err != nil {
		return err
	}
	evidence := report{Mode: "app run", FeatureVersion: selectedFeature, Renderer: strings.TrimSpace(string(renderer)), AppGoVersion: strings.TrimSpace(string(appGo)), Checks: []string{"app-owned Wire injection", "rerender preserved application wiring", "no module replacements"}}
	program := filepath.Join(project, "bin", "app")
	arguments := []string{"run"}
	if *dev {
		program = binary
		arguments = []string{"dev"}
		evidence.Mode = "forj dev"
	}
	logPath := filepath.Join(project, "smoke-process.log")
	log, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(program, arguments...)
	cmd.Dir = project
	cmd.Env = env
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	var waitError error
	go func() { waitError = cmd.Wait(); close(done) }()
	var stopOnce sync.Once
	var stopError error
	stop := func() error {
		stopOnce.Do(func() {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
				stopError = waitError
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				stopError = errors.New("smoke process did not shut down")
			}
		})
		return stopError
	}
	defer func() { _ = stop() }()
	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1/feature-smoke", port)
	client := &http.Client{Timeout: 5 * time.Second}
	request := func(method, path string, target any) error {
		req, err := http.NewRequest(method, base+path, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("smoke HTTP status %d", response.StatusCode)
		}
		return json.NewDecoder(response.Body).Decode(target)
	}
	var initial decision
	if err := eventually(3*time.Minute, func() bool { return request(http.MethodGet, "", &initial) == nil }); err != nil {
		return fmt.Errorf("App startup: %w; inspect %s", err, logPath)
	}
	if !*dev && initial.PID != cmd.Process.Pid {
		return errors.New("HTTP response did not come from the launched App")
	}
	if *dev {
		time.Sleep(2 * time.Second)
		if err := request(http.MethodGet, "", &initial); err != nil {
			return err
		}
	}
	if err := checkDecision(initial, initial.PID, "blue", false, 2); err != nil {
		return err
	}
	evidence.PID = initial.PID
	var held decision
	heldError := make(chan error, 1)
	go func() { heldError <- request(http.MethodGet, "?hold=true", &held) }()
	if err := eventually(5*time.Second, func() bool { var h health; return request(http.MethodGet, "/status", &h) == nil && h.Captured }); err != nil {
		return err
	}
	if err := publish(flagsFile, "green", true, 3); err != nil {
		return err
	}
	if err := eventually(10*time.Second, func() bool {
		var value decision
		return request(http.MethodGet, "", &value) == nil && checkDecision(value, initial.PID, "green", true, 3) == nil
	}); err != nil {
		return err
	}
	var released map[string]bool
	if err := request(http.MethodPost, "/release", &released); err != nil {
		return err
	}
	if err := <-heldError; err != nil {
		return err
	}
	if err := checkDecision(held, initial.PID, "blue", false, 2); err != nil {
		return err
	}
	evidence.Checks = append(evidence.Checks, "live file update with unchanged App PID", "held request retained one revision across update")
	if err := os.WriteFile(flagsFile+".next", []byte(`{"schema":2,"flags":`), 0600); err != nil {
		return err
	}
	if err := os.Rename(flagsFile+".next", flagsFile); err != nil {
		return err
	}
	if err := expectHealth(client, base, "feature: invalid document"); err != nil {
		return err
	}
	var retained decision
	if err := request(http.MethodGet, "", &retained); err != nil {
		return err
	}
	if err := checkDecision(retained, initial.PID, "green", true, 3); err != nil {
		return err
	}
	if err := os.Remove(flagsFile); err != nil {
		return err
	}
	if err := expectHealth(client, base, "feature: source unavailable"); err != nil {
		return err
	}
	if err := request(http.MethodGet, "", &retained); err != nil {
		return err
	}
	if err := checkDecision(retained, initial.PID, "green", true, 3); err != nil {
		return err
	}
	if err := publish(flagsFile, "sunrise", false, 4); err != nil {
		return err
	}
	if err := expectHealth(client, base, ""); err != nil {
		return err
	}
	if err := eventually(10*time.Second, func() bool {
		var value decision
		return request(http.MethodGet, "", &value) == nil && checkDecision(value, initial.PID, "sunrise", false, 4) == nil
	}); err != nil {
		return err
	}
	evidence.Checks = append(evidence.Checks, "invalid JSON and deletion retained last valid decisions", "source recovery cleared health without restarting")
	if *dev {
		time.Sleep(time.Second)
		var value decision
		if err := request(http.MethodGet, "", &value); err != nil {
			return err
		}
		if err := checkDecision(value, initial.PID, "sunrise", false, 4); err != nil {
			return err
		}
		evidence.Checks = append(evidence.Checks, "JSON changes did not trigger default dev watcher restarts")
	}
	if err := stop(); err != nil {
		return err
	}
	logs, err := os.ReadFile(logPath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(logs), "feature smoke: refresh worker joined") {
		return errors.New("shutdown did not join the feature worker")
	}
	evidence.Checks = append(evidence.Checks, "shutdown canceled and joined refresh worker")
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0644); err != nil {
		return err
	}
	succeeded = true
	fmt.Printf("Passed %s smoke with unchanged App PID %d\n", evidence.Mode, evidence.PID)
	return nil
}

// publish atomically replaces a complete fake flag inventory at the selected app-owned path.
func publish(path, theme string, checkout bool, columns int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{"schema": 2, "flags": map[string]any{"theme": map[string]any{"default": theme}, "checkout": map[string]any{"default": false, "rules": []any{map[string]any{"attributes": map[string]string{"plan": "pro"}, "value": checkout}}}, "layout": map[string]any{"default": map[string]int{"columns": columns}}}})
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".next", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".next", path)
}

// copyFixture installs only app-owned code into the disposable rendered project.
func copyFixture(source, target string) error {
	return fs.WalkDir(os.DirFS(source), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		destination := filepath.Join(target, path)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0755)
		}
		data, err := os.ReadFile(filepath.Join(source, path))
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0644)
	})
}

// fixtureUnchanged verifies rerendering preserves application customization byte for byte.
func fixtureUnchanged(source, target string) error {
	return fs.WalkDir(os.DirFS(source), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		want, err := os.ReadFile(filepath.Join(source, path))
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(target, path))
		if err != nil {
			return err
		}
		if !bytes.Equal(want, got) {
			return fmt.Errorf("rerender replaced app-owned file %s", path)
		}
		return nil
	})
}

// checkDecision requires consistent fake business values and a single captured revision.
func checkDecision(value decision, pid int, theme string, checkout bool, columns int) error {
	if value.PID != pid || value.FirstTheme != theme || value.Theme != theme || value.Checkout != checkout || value.Columns != columns || len(value.Revisions) != 4 || value.Revisions[0] == "" {
		return fmt.Errorf("unexpected decision %+v", value)
	}
	for _, revision := range value.Revisions {
		if revision != value.Revisions[0] {
			return errors.New("mixed request revisions")
		}
	}
	return nil
}

// eventually bounds readiness and live-publication waits without inferring success from elapsed time.
func eventually(timeout time.Duration, condition func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("smoke condition did not converge")
}

// expectHealth polls only classified source health, never source diagnostics.
func expectHealth(client *http.Client, base, classification string) error {
	return eventually(10*time.Second, func() bool {
		response, err := client.Get(base + "/status")
		if err != nil {
			return false
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return false
		}
		var value health
		return json.NewDecoder(response.Body).Decode(&value) == nil && value.Error == classification
	})
}
