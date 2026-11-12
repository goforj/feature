//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestBackendVersionPins prevents support metadata from diverging from image constants or selected SDK modules.
func TestBackendVersionPins(t *testing.T) {
	data, err := os.ReadFile("versions.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, pin := range []struct{ name, version, image string }{
		{"go-feature-flag", relayVersion, relayImage},
		{"launchdarkly-relay", launchDarklyRelayVersion, launchDarklyRelayImage},
	} {
		var recorded struct{ Version, Image string }
		if err := json.Unmarshal(manifest[pin.name], &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.Version != pin.version || recorded.Image != pin.image {
			t.Fatalf("%s support metadata differs from the tested container pin", pin.name)
		}
	}
	modules := map[string]string{
		"launchdarkly-go-sdk":                  "github.com/launchdarkly/go-server-sdk/v7",
		"openfeature-go-sdk":                   "github.com/open-feature/go-sdk",
		"openfeature-go-feature-flag-provider": "github.com/open-feature/go-sdk-contrib/providers/go-feature-flag",
		"openfeature-launchdarkly-provider":    "github.com/open-feature/go-sdk-contrib/providers/launchdarkly",
		"testcontainers-go":                    "github.com/testcontainers/testcontainers-go",
	}
	args := []string{"list", "-m", "-f", "{{.Path}} {{.Version}}"}
	for _, path := range modules {
		args = append(args, path)
	}
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("selected module inventory: %v\n%s", err, output)
	}
	selected := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid selected module metadata: %s", line)
		}
		selected[fields[0]] = fields[1]
	}
	for name, path := range modules {
		var version string
		if err := json.Unmarshal(manifest[name], &version); err != nil {
			t.Fatal(err)
		}
		if selected[path] != version {
			t.Fatalf("%s report=%s selected=%s", name, version, selected[path])
		}
	}
}
