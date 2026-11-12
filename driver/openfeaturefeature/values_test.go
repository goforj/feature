package openfeaturefeature

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
	"github.com/open-feature/go-sdk/openfeature/memprovider"
)

// TestTypedSDKFailures exercises typed SDK errors and bounded adapter output.
func TestTypedSDKFailures(t *testing.T) {
	for _, reason := range []openfeature.Reason{openfeature.DisabledReason, openfeature.SplitReason, openfeature.StaticReason, openfeature.UnknownReason} {
		if portableReason(reason) == "" {
			t.Fatal(reason)
		}
	}
	if native, err := evaluationContext(feature.Target{Key: "a", Kind: "organization"}); err != nil || native.Attribute("kind") != "organization" {
		t.Fatal(native, err)
	}
	ctx := context.Background()
	api := isolated.NewAPI()
	t.Cleanup(func() { _ = api.Shutdown(ctx) })
	provider := memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{
		"float":  {Key: "float", State: memprovider.Enabled, DefaultVariant: "on", Variants: map[string]any{"on": math.NaN()}},
		"object": {Key: "object", State: memprovider.Enabled, DefaultVariant: "on", Variants: map[string]any{"on": func() {}}},
	})
	if err := api.SetProviderAndWait(ctx, provider); err != nil {
		t.Fatal(err)
	}
	driver := New(api.NewClient())
	if d := driver.ResolveString(ctx, "missing", "fallback", feature.Target{}); d.Value != "fallback" || d.Err != feature.ErrNotFound {
		t.Fatal(d)
	}
	if d := driver.ResolveFloat(ctx, "float", 1, feature.Target{}); d.Value != 1 || d.Err != feature.ErrInvalidValue {
		t.Fatal(d)
	}
	if d := driver.ResolveObject(ctx, "object", map[string]any{}, feature.Target{}); d.Err != feature.ErrInvalidValue {
		t.Fatal(d)
	}
	if d := driver.ResolveFloat(ctx, "float", math.Inf(1), feature.Target{}); d.Err != feature.ErrInvalidValue {
		t.Fatal(d)
	}
	if d := driver.ResolveString(ctx, "x", "f", feature.Target{Key: strings.Repeat("x", feature.MaxStringBytes+1)}); d.Err != feature.ErrInvalidSubject {
		t.Fatal(d)
	}
	if d := New(clientFunc(nil)).ResolveString(ctx, "x", "f", feature.Target{}); d.Err != feature.ErrUnsupported {
		t.Fatal(d)
	}
	child, cancel := context.WithCancel(ctx)
	d := resolveValue(driver, child, "x", "f", feature.TypeString, feature.Target{}, func(valueClient, string, openfeature.EvaluationContext) (openfeature.GenericEvaluationDetails[string], error) {
		cancel()
		return openfeature.StringEvaluationDetails{}, errors.New("private SDK diagnostic")
	})
	if d.Err != context.Canceled || d.Value != "f" {
		t.Fatal(d)
	}
	if d := driver.ResolveString(child, "x", "f", feature.Target{}); d.Err != context.Canceled {
		t.Fatal(d)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if d := driver.ResolveString(ctx, "x", "f", feature.Target{}); d.Err != feature.ErrClosed {
		t.Fatal(d)
	}
}
