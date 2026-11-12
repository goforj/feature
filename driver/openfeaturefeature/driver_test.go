package openfeaturefeature

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
	"github.com/open-feature/go-sdk/openfeature/memprovider"
)

// clientFunc controls only the SDK call boundary, leaving the adapter policy under test.
type clientFunc func(context.Context, string, bool, openfeature.EvaluationContext) (openfeature.BooleanEvaluationDetails, error)

// BooleanValueDetails exposes a narrow SDK client for failure and ownership scenarios.
func (f clientFunc) BooleanValueDetails(ctx context.Context, key string, fallback bool, target openfeature.EvaluationContext, _ ...openfeature.Option) (openfeature.BooleanEvaluationDetails, error) {
	return f(ctx, key, fallback, target)
}

// detail constructs the SDK's structured evidence without raw provider error prose.
func detail(value bool, reason openfeature.Reason, code openfeature.ErrorCode) openfeature.BooleanEvaluationDetails {
	return openfeature.BooleanEvaluationDetails{Value: value, EvaluationDetails: openfeature.EvaluationDetails{ResolutionDetail: openfeature.ResolutionDetail{Reason: reason, ErrorCode: code, Variant: "variant"}}}
}

// TestSDKResolutionMapping covers values, variants, failures, reserved keys, and owned target data.
func TestSDKResolutionMapping(t *testing.T) {
	ctx := context.Background()
	var result openfeature.BooleanEvaluationDetails
	var providerErr error
	var seen openfeature.EvaluationContext
	sdk := clientFunc(func(_ context.Context, _ string, _ bool, target openfeature.EvaluationContext) (openfeature.BooleanEvaluationDetails, error) {
		seen = target
		return result, providerErr
	})
	driver := New(sdk)
	target := feature.Target{Key: "tenant:acme", Kind: "organization", Attributes: map[string]any{"paid": true, "nested": map[string]any{"plan": "pro"}, openfeature.TargetingKey: "cannot-override"}}
	for _, tc := range []struct {
		reason openfeature.Reason
		want   feature.Reason
	}{{openfeature.DisabledReason, feature.ReasonDisabled}, {openfeature.TargetingMatchReason, feature.ReasonRule}, {openfeature.SplitReason, feature.ReasonSplit}, {openfeature.DefaultReason, feature.ReasonDefault}, {openfeature.StaticReason, feature.ReasonStatic}, {openfeature.UnknownReason, feature.ReasonUnknown}} {
		result = detail(true, tc.reason, "")
		d := driver.Resolve(ctx, "checkout", false, target)
		if !d.Value || d.Reason != tc.want || d.Variant != "variant" || d.Err != nil {
			t.Fatal(d)
		}
	}
	if seen.TargetingKey() != "tenant:acme" || seen.Attribute("kind") != "organization" || seen.Attribute("paid") != true {
		t.Fatal(seen)
	}
	seen.Attribute("nested").(map[string]any)["plan"] = "changed"
	if target.Attributes["nested"].(map[string]any)["plan"] != "pro" || target.Attributes[openfeature.TargetingKey] != "cannot-override" {
		t.Fatal("adapter mutated input")
	}
	for _, tc := range []struct {
		code openfeature.ErrorCode
		want error
	}{{openfeature.FlagNotFoundCode, feature.ErrNotFound}, {openfeature.TypeMismatchCode, feature.ErrTypeMismatch}, {openfeature.TargetingKeyMissingCode, feature.ErrInvalidSubject}, {openfeature.InvalidContextCode, feature.ErrInvalidSubject}, {openfeature.ProviderNotReadyCode, feature.ErrUnavailable}, {"", feature.ErrUnavailable}} {
		result = detail(false, openfeature.ErrorReason, tc.code)
		d := driver.Resolve(ctx, "checkout", true, target)
		if !d.Value || d.Err != tc.want {
			t.Fatal(d)
		}
	}
	result = detail(false, "", "")
	providerErr = errors.New("private provider text")
	if d := driver.Resolve(ctx, "checkout", true, feature.Target{}); d.Err != feature.ErrUnavailable || !d.Value {
		t.Fatal(d)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if d := driver.Resolve(canceled, "checkout", true, target); d.Err != context.Canceled {
		t.Fatal(d)
	}
	if d := driver.Resolve(ctx, "checkout", false, feature.Target{Key: strings.Repeat("x", feature.MaxStringBytes+1)}); d.Err != feature.ErrInvalidSubject {
		t.Fatal(d)
	}
	child, cancel := context.WithCancel(ctx)
	driver.client = clientFunc(func(context.Context, string, bool, openfeature.EvaluationContext) (openfeature.BooleanEvaluationDetails, error) {
		cancel()
		return detail(false, openfeature.ErrorReason, openfeature.FlagNotFoundCode), context.Canceled
	})
	if d := driver.Resolve(child, "checkout", true, target); d.Err != context.Canceled || d.Reason != feature.ReasonError {
		t.Fatal(d)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if d := driver.Resolve(ctx, "checkout", true, target); d.Err != feature.ErrClosed {
		t.Fatal(d)
	}
}

// testAPI provides controlled lifecycle failures around a real SDK client.
type testAPI struct {
	base    *openfeature.EvaluationAPI
	start   func(context.Context) error
	stops   int
	stopErr error
}

// SetProviderAndWait invokes the initialization scenario supplied by the test.
func (a *testAPI) SetProviderAndWait(ctx context.Context, _ openfeature.FeatureProvider, _ ...openfeature.APIOption) error {
	return a.start(ctx)
}

// NewClient returns a real SDK client without exposing a provider shortcut.
func (a *testAPI) NewClient(_ ...openfeature.APIOption) *openfeature.Client {
	return a.base.NewClient()
}

// Shutdown records cleanup and can surface an injected SDK disposal failure.
func (a *testAPI) Shutdown(context.Context) error { a.stops++; return a.stopErr }

// TestOwningLifecycleFailures covers pre-cancellation, initialization errors, and close errors.
func TestOwningLifecycleFailures(t *testing.T) {
	ctx := context.Background()
	base := isolated.NewAPI()
	defer base.Shutdown(ctx)
	api := &testAPI{base: base, start: func(context.Context) error { return errors.New("private initialization response") }}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Open(canceled, openfeature.NoopProvider{}); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := open(canceled, openfeature.NoopProvider{}, api); err != context.Canceled || api.stops != 1 {
		t.Fatal(err, api.stops)
	}
	if _, err := open(ctx, openfeature.NoopProvider{}, api); err != feature.ErrUnavailable || api.stops != 2 {
		t.Fatal(err, api.stops)
	}
	child, cancel := context.WithCancel(ctx)
	api.start = func(context.Context) error { cancel(); return context.Canceled }
	if _, err := open(child, openfeature.NoopProvider{}, api); err != context.Canceled || api.stops != 3 {
		t.Fatal(err, api.stops)
	}
	api.start = func(context.Context) error { return nil }
	api.stopErr = errors.New("private cleanup data")
	driver, err := open(ctx, openfeature.NoopProvider{}, api)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != feature.ErrUnavailable {
		t.Fatal(err)
	}
}

// observingHook proves evaluations still pass through the SDK's precedence and hook machinery.
type observingHook struct {
	openfeature.UnimplementedHook
	calls atomic.Int32
}

// Before confirms invocation precedence and supplies a final hook override.
func (h *observingHook) Before(_ context.Context, input openfeature.HookContext, _ openfeature.HookHints) (*openfeature.EvaluationContext, error) {
	h.calls.Add(1)
	attributes := input.EvaluationContext().Attributes()
	if attributes["winner"] != "invocation" {
		return nil, errors.New("invocation context was not merged")
	}
	attributes["winner"] = "hook"
	result := openfeature.NewEvaluationContext(input.EvaluationContext().TargetingKey(), attributes)
	return &result, nil
}

// TestRealSDKHooksContextsAndUpdates verifies the adapter never bypasses SDK evaluation.
func TestRealSDKHooksContextsAndUpdates(t *testing.T) {
	ctx := context.Background()
	api := isolated.NewAPI()
	defer api.Shutdown(ctx)
	evaluate := func(_ memprovider.InMemoryFlag, target openfeature.FlattenedContext) (any, openfeature.ProviderResolutionDetail) {
		return target["winner"] == "hook" && target["paid"] == true && target["seats"] == int64(12) && target["region"] == "local", openfeature.ProviderResolutionDetail{Reason: openfeature.TargetingMatchReason}
	}
	provider := memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{"checkout": {Key: "checkout", State: memprovider.Enabled, DefaultVariant: "on", Variants: map[string]any{"on": true}, ContextEvaluator: &evaluate}})
	if err := api.SetProviderAndWait(ctx, provider); err != nil {
		t.Fatal(err)
	}
	api.SetEvaluationContext(openfeature.NewEvaluationContext("", map[string]any{"winner": "global"}))
	sdk := api.NewClient()
	sdk.SetEvaluationContext(openfeature.NewEvaluationContext("", map[string]any{"winner": "client"}))
	hook := &observingHook{}
	sdk.AddHooks(hook)
	driver := New(sdk)
	features := feature.New(driver).For(feature.Target{Key: "tenant:acme", Attributes: map[string]any{"winner": "invocation", "paid": true, "seats": int64(12)}})
	transaction := openfeature.WithTransactionContext(ctx, openfeature.NewEvaluationContext("", map[string]any{"winner": "transaction", "region": "local"}))
	if !features.Enabled(transaction, "checkout") || hook.calls.Load() != 1 {
		t.Fatal("SDK hook or context merge bypassed")
	}
	provider.UpdateFlags(map[string]memprovider.InMemoryFlag{"checkout": {Key: "checkout", State: memprovider.Enabled, DefaultVariant: "off", Variants: map[string]any{"off": false}}})
	if features.Enabled(transaction, "checkout") {
		t.Fatal("SDK update was not adopted")
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sdk.BooleanValue(transaction, "checkout", true, openfeature.NewEvaluationContext("", map[string]any{"winner": "invocation"})); err != nil {
		t.Fatal("borrowed SDK client was shut down", err)
	}
}

// lifecycleProvider exposes native SDK initialization and shutdown observations.
type lifecycleProvider struct {
	*memprovider.InMemoryProvider
	stops   atomic.Int32
	initErr error
}

// Init implements the provider's legacy lifecycle contract for SDK compatibility.
func (p *lifecycleProvider) Init(openfeature.EvaluationContext) error { return p.initErr }

// Shutdown records resource release by the owning isolated API.
func (p *lifecycleProvider) Shutdown() { p.stops.Add(1) }

// TestRealOwningAPIShutsDownProvider verifies successful and failed native initialization.
func TestRealOwningAPIShutsDownProvider(t *testing.T) {
	ctx := context.Background()
	provider := &lifecycleProvider{InMemoryProvider: memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{})}
	driver, err := Open(ctx, provider)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != nil || provider.stops.Load() != 1 {
		t.Fatal(err, provider.stops.Load())
	}
	failed := &lifecycleProvider{InMemoryProvider: memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{}), initErr: errors.New("private provider credentials")}
	if _, err := Open(ctx, failed); err != feature.ErrUnavailable || failed.stops.Load() != 1 {
		t.Fatal(err, failed.stops.Load())
	}
}
