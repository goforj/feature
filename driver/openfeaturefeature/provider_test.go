package openfeaturefeature

import (
	"context"
	"testing"

	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

// nativeProviderFixture exposes actual native definitions through an ordinary SDK binding.
func nativeProviderFixture(t *testing.T) (*feature.Client, *feature.MemorySource, *Provider, *openfeature.Client) {
	t.Helper()
	source, err := feature.NewMemorySource(feature.Document{Schema: 2, Flags: map[string]feature.Flag{
		"bool": {Default: true}, "string": {Default: "blue", Rules: []feature.Rule{{Subjects: []string{"early"}, Value: "green"}}}, "int": {Default: int64(9007199254740993)}, "float": {Default: 1.25}, "object": {Default: map[string]any{"color": "blue"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := feature.Open(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	provider := NewProvider(engine)
	api := isolated.NewAPI()
	t.Cleanup(func() {
		if err := api.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := api.SetProviderAndWait(context.Background(), provider); err != nil {
		t.Fatal(err)
	}
	return engine, source, provider, api.NewClient()
}

// TestNativeProviderThroughSDK proves all value types, precision, targeting, updates, and borrowed ownership.
func TestNativeProviderThroughSDK(t *testing.T) {
	ctx := context.Background()
	engine, source, provider, sdk := nativeProviderFixture(t)
	flat := openfeature.NewEvaluationContext("early", nil)
	if value, err := sdk.BooleanValue(ctx, "bool", false, flat); !value || err != nil {
		t.Fatal(value, err)
	}
	if value, err := sdk.StringValue(ctx, "string", "fallback", flat); value != "green" || err != nil {
		t.Fatal(value, err)
	}
	if value, err := sdk.IntValue(ctx, "int", 0, flat); value != 9007199254740993 || err != nil {
		t.Fatal(value, err)
	}
	if value, err := sdk.FloatValue(ctx, "float", 0, flat); value != 1.25 || err != nil {
		t.Fatal(value, err)
	}
	if value, err := sdk.ObjectValue(ctx, "object", map[string]any{}, flat); err != nil || value.(map[string]any)["color"] != "blue" {
		t.Fatal(value, err)
	}
	detail, err := sdk.StringValueDetails(ctx, "string", "fallback", flat)
	if err != nil || detail.FlagMetadata["revision"] != engine.Status().Revision || detail.Reason != openfeature.TargetingMatchReason {
		t.Fatal(detail, err)
	}
	driver := New(sdk)
	t.Cleanup(func() { _ = driver.Close() })
	flags := feature.New(driver).For(feature.Target{Key: "early"})
	if flags.String(ctx, "string", "f").Value != "green" || flags.Int(ctx, "int", 0).Value != 9007199254740993 || flags.Float(ctx, "float", 0).Value != 1.25 || flags.Object(ctx, "object", map[string]any{}).Err != nil {
		t.Fatal("typed round trip failed")
	}
	if err := source.Replace(feature.Document{Schema: 2, Flags: map[string]feature.Flag{"string": {Default: "updated"}}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if value, err := sdk.StringValue(ctx, "string", "f", flat); value != "updated" || err != nil {
		t.Fatal(value, err)
	}
	if err := provider.ShutdownWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	if d := provider.StringEvaluation(ctx, "string", "f", nil); d.Value != "f" || d.ResolutionDetail().ErrorCode != openfeature.ProviderNotReadyCode {
		t.Fatal(d)
	}
	if feature.New(engine).String(ctx, "string", "f").Value != "updated" {
		t.Fatal("provider closed borrowed engine")
	}
}

// TestProviderFailureMapping checks every native classification and reserved context field.
func TestProviderFailureMapping(t *testing.T) {
	_, _, provider, sdk := nativeProviderFixture(t)
	ctx := context.Background()
	empty := openfeature.NewTargetlessEvaluationContext(nil)
	if value, err := sdk.StringValue(ctx, "missing", "fallback", empty); value != "fallback" || err == nil {
		t.Fatal(value, err)
	}
	if value, err := sdk.StringValue(ctx, "bool", "fallback", empty); value != "fallback" || err == nil {
		t.Fatal(value, err)
	}
	for _, flat := range []openfeature.FlattenedContext{{openfeature.TargetingKey: 12}, {"kind": 12}} {
		if d := provider.StringEvaluation(ctx, "string", "fallback", flat); d.Value != "fallback" || d.ResolutionDetail().ErrorCode != openfeature.InvalidContextCode {
			t.Fatal(d)
		}
	}
	if target, err := providerTarget(openfeature.FlattenedContext{openfeature.TargetingKey: "a", "kind": "organization", "paid": true}); err != nil || target.Kind != "organization" || target.Attributes["paid"] != true {
		t.Fatal(target, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := provider.InitWithContext(canceled, empty); err != context.Canceled {
		t.Fatal(err)
	}
	if d := provider.StringEvaluation(canceled, "string", "f", nil); d.Value != "f" || d.Error() == nil {
		t.Fatal(d)
	}
	if err := provider.Init(empty); err != nil {
		t.Fatal(err)
	}
	provider.Shutdown()
	if err := provider.Init(empty); err != feature.ErrClosed {
		t.Fatal(err)
	}
	for _, reason := range []feature.Reason{feature.ReasonDefault, feature.ReasonRule, feature.ReasonDisabled, feature.ReasonSplit, feature.ReasonStatic, feature.ReasonUnknown} {
		if d := providerDetail(reason, "", "", nil); d.Reason == "" {
			t.Fatal(d)
		}
	}
	for _, err := range []error{feature.ErrNotFound, feature.ErrTypeMismatch, feature.ErrInvalidSubject, feature.ErrClosed, feature.ErrInvalidValue} {
		if d := providerDetail(feature.ReasonError, "r", "v", err); d.Error() == nil {
			t.Fatal(d)
		}
	}
}
