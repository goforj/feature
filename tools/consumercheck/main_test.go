package main

import (
	"strings"
	"testing"
)

// TestPublicEnvironment prevents private-module and workspace bypasses from masquerading as public verification.
func TestPublicEnvironment(t *testing.T) {
	for key, value := range map[string]string{"GOENV": "persisted", "GOWORK": "/tmp/unrelated.work", "GOFLAGS": "-modfile=/tmp/unrelated.mod", "GOPRIVATE": "*", "GONOSUMDB": "*", "GONOPROXY": "*", "GOPROXY": "direct", "GOSUMDB": "off", "GH_TOKEN": "unused", "GITHUB_TOKEN": "unused"} {
		t.Setenv(key, value)
	}
	values := map[string]string{}
	for _, entry := range environment("fresh-modules", "go1.24.13") {
		key, value, _ := strings.Cut(entry, "=")
		if _, duplicate := values[key]; duplicate {
			t.Fatal("duplicate setting", key)
		}
		values[key] = value
	}
	for key, want := range map[string]string{"GOENV": "off", "GOWORK": "off", "GOFLAGS": "", "GOPRIVATE": "", "GONOSUMDB": "none", "GONOPROXY": "none", "GOPROXY": "https://proxy.golang.org", "GOSUMDB": "sum.golang.org", "GOMODCACHE": "fresh-modules", "GOTOOLCHAIN": "go1.24.13"} {
		if values[key] != want {
			t.Fatal("public verification setting differs", key)
		}
	}
	if _, present := values["GH_TOKEN"]; present {
		t.Fatal("credential inherited")
	}
	if _, present := values["GITHUB_TOKEN"]; present {
		t.Fatal("credential inherited")
	}
}

// TestVersionPattern prevents version arguments from adding local replacements to consumer modules.
func TestVersionPattern(t *testing.T) {
	for _, version := range []string{"v0.1.0", "v0.0.0-20261113224422-3e7b49d5eec5", "v1.0.0-rc.1", "v2.0.0+incompatible"} {
		if !versionPattern.MatchString(version) {
			t.Fatal("valid module version rejected", version)
		}
	}
	for _, version := range []string{"", "main", "v0.1.0\nreplace example.com/a => /tmp/a", "v0.1.0/../../a", "v0.1.0 //comment"} {
		if versionPattern.MatchString(version) {
			t.Fatal("invalid module directive accepted")
		}
	}
}
