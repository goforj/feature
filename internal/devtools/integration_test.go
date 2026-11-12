package devtools

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestIntegrationEvidenceRejectsIncompleteRuns prevents green focused or skipped tests from certifying all drivers.
func TestIntegrationEvidenceRejectsIncompleteRuns(t *testing.T) {
	for _, events := range []string{"", "invalid", `{"Action":"skip","Test":"required"}`, `{"Action":"fail"}`, `{"Action":"pass"}`} {
		if _, err := integrationCases([]byte(events), false); err == nil {
			t.Fatal("invalid evidence accepted", events)
		}
	}
	focused := []byte(`{"Action":"pass","Test":"TestDriverConformance/native/true"}`)
	if count, err := integrationCases(focused, false); count != 1 || err != nil {
		t.Fatal(count, err)
	}
	if _, err := integrationCases(focused, true); err == nil {
		t.Fatal("focused evidence certified the full suite")
	}
	var events []byte
	add := func(name string) {
		data, err := json.Marshal(testEvent{Action: "pass", Test: name})
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, data...)
		events = append(events, '\n')
	}
	for _, name := range []string{"native", "launchdarkly", "openfeature-launchdarkly", "openfeature-goff-remote", "openfeature-goff-inprocess"} {
		for _, scenario := range []string{"true", "false-is-success", "missing-false", "missing-true", "identity", "owned-target", "canceled", "concurrent", "live-update", "delete-and-recreate"} {
			add("TestDriverConformance/" + name + "/" + scenario)
		}
	}
	if _, err := integrationCases(events, true); err == nil {
		t.Fatal("missing native and failure verification accepted")
	}
	for _, name := range []string{"TestFileRefreshLifecycle", "TestLaunchDarklyStreamingAndEvents", "TestLaunchDarklyStartupFailures", "TestLaunchDarklyBorrowedOwnership", "TestRelayAuthentication", "TestRelayContainerOutage", "TestRelayTransportFailures", "TestRelayTypedTargeting", "TestOpenFeatureSDKTypeMismatch"} {
		add(name)
	}
	if _, err := integrationCases(events, true); err == nil {
		t.Fatal("missing vendor container verification accepted")
	}
	for _, scenario := range []string{"configured-off-variation", "authentication", "container-outage-and-recovery", "analytics-on-close"} {
		add("TestLaunchDarklyRelay/" + scenario)
	}
	if _, err := integrationCases(events, true); err == nil {
		t.Fatal("missing backend version verification accepted")
	}
	add("TestBackendVersionPins")
	if _, err := integrationCases(events, true); err == nil {
		t.Fatal("missing provider context verification accepted")
	}
	for _, adapter := range []string{"direct", "openfeature"} {
		for _, scenario := range []string{"multi-identity", "typed-targeting", "off-variation", "privacy"} {
			add("TestLaunchDarklyRelayContexts/" + adapter + "/" + scenario)
		}
	}
	if count, err := integrationCases(events, true); count != 72 || err != nil {
		t.Fatal(count, err)
	}
	for _, name := range []string{"native", "launchdarkly", "openfeature-launchdarkly", "openfeature-goff-remote", "openfeature-goff-inprocess"} {
		t.Run(name+"-deletion-required", func(t *testing.T) {
			missing := strings.Replace(string(events), "TestDriverConformance/"+name+"/delete-and-recreate", "unrelated-case", 1)
			if _, err := integrationCases([]byte(missing), true); err == nil {
				t.Fatal("flag deletion verification was optional")
			}
		})
	}
}
