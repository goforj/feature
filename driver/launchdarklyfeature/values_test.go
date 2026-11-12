package launchdarklyfeature

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/goforj/feature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
)

// TestTypedOfficialSDK checks actual engine values, numeric precision, and returned object ownership.
func TestTypedOfficialSDK(t *testing.T) {
	ctx := context.Background()
	data := ldtestdata.DataSource()
	data.Update(data.Flag("large-float").Variations(ldvalue.Float64(float64(1 << 63))).VariationForAllIndex(0))
	data.Update(data.Flag("large-object").Variations(ldvalue.CopyArbitraryValue(map[string]any{"nested": []any{float64(1 << 63)}})).VariationForAllIndex(0))
	for key, value := range map[string]ldvalue.Value{"string": ldvalue.String("blue"), "int": ldvalue.Int(12), "float": ldvalue.Float64(1.25), "object": ldvalue.CopyArbitraryValue(map[string]any{"color": "blue"}), "fraction": ldvalue.Float64(1.5), "large": ldvalue.Float64(9007199254740992)} {
		data.Update(data.Flag(key).Variations(value).VariationForAllIndex(0))
	}
	driver, err := Open(ctx, Config{SDKKey: "typed-test", SDKConfig: ldclient.Config{DataSource: data, Events: ldcomponents.NoEvents(), Logging: ldcomponents.NoLogging()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Error(err)
		}
	})
	flags := feature.New(driver).For(feature.Target{Key: "tenant:acme"})
	if result := flags.Float(ctx, "large-float", 0); result.Err != nil || result.Value != float64(1<<63) {
		t.Fatal(result)
	}
	if result := flags.Object(ctx, "large-object", map[string]any{}); result.Err != nil || result.Value.(map[string]any)["nested"].([]any)[0] != float64(1<<63) {
		t.Fatal(result)
	}
	if d := flags.String(ctx, "string", "fallback"); d.Value != "blue" || d.Err != nil || d.Variant != "0" {
		t.Fatal(d)
	}
	if d := flags.Int(ctx, "int", 0); d.Value != 12 || d.Err != nil {
		t.Fatal(d)
	}
	if d := flags.Float(ctx, "float", 0); d.Value != 1.25 || d.Err != nil {
		t.Fatal(d)
	}
	object := flags.Object(ctx, "object", map[string]any{})
	if object.Err != nil {
		t.Fatal(object)
	}
	object.Value.(map[string]any)["color"] = "changed"
	if flags.Object(ctx, "object", map[string]any{}).Value.(map[string]any)["color"] != "blue" {
		t.Fatal("mutable SDK result retained")
	}
	for _, key := range []string{"fraction", "large", "string"} {
		if d := flags.Int(ctx, key, 9); d.Value != 9 || d.Err != feature.ErrTypeMismatch {
			t.Fatal(d)
		}
	}
	if d := flags.String(ctx, "absent", "f"); d.Value != "f" || d.Err != feature.ErrNotFound || d.Reason != feature.ReasonMissing {
		t.Fatal(d)
	}
	if d := flags.String(ctx, "int", "f"); d.Value != "f" || d.Err != feature.ErrTypeMismatch {
		t.Fatal(d)
	}
	data.Update(data.Flag("string").Variations(ldvalue.String("new"), ldvalue.String("off")).OffVariationIndex(1).On(false))
	if d := flags.String(ctx, "string", "f"); d.Value != "off" || d.Reason != feature.ReasonDisabled {
		t.Fatal(d)
	}
}

// jsonClient exposes malformed and failed SDK boundaries without replacing engine integration tests.
type jsonClient struct {
	testClient
	value  ldvalue.Value
	detail ldreason.EvaluationDetail
	err    error
	cancel context.CancelFunc
}

// StringVariationDetailCtx exposes native string capability independently of JSON evaluation.
func (c *jsonClient) StringVariationDetailCtx(ctx context.Context, key string, target ldcontext.Context, fallback string) (string, ldreason.EvaluationDetail, error) {
	value, detail, err := c.JSONVariationDetailCtx(ctx, key, target, ldvalue.String(fallback))
	return value.StringValue(), detail, err
}

// Float64VariationDetailCtx exposes numeric evaluation without integer truncation.
func (c *jsonClient) Float64VariationDetailCtx(ctx context.Context, key string, target ldcontext.Context, fallback float64) (float64, ldreason.EvaluationDetail, error) {
	value, detail, err := c.JSONVariationDetailCtx(ctx, key, target, ldvalue.Float64(fallback))
	return value.Float64Value(), detail, err
}

// JSONVariationDetailCtx supplies one controlled native response.
func (c *jsonClient) JSONVariationDetailCtx(context.Context, string, ldcontext.Context, ldvalue.Value) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	if c.cancel != nil {
		c.cancel()
	}
	return c.value, c.detail, c.err
}

// TestTypedSDKFailureBoundaries classifies cancellation, malformed outputs, and unsupported borrowed clients.
func TestTypedSDKFailureBoundaries(t *testing.T) {
	ctx := context.Background()
	target := feature.Target{Key: "user"}
	sdk := &jsonClient{value: ldvalue.String("blue")}
	driver := New(sdk)
	if d := driver.ResolveString(ctx, "flag", "f", target); d.Value != "blue" || d.Reason != feature.ReasonUnknown {
		t.Fatal(d)
	}
	if d := New(&testClient{}).ResolveString(ctx, "flag", "f", target); d.Err != feature.ErrUnsupported {
		t.Fatal(d)
	}
	if d := driver.ResolveString(ctx, "flag", "f", feature.Target{}); d.Err != feature.ErrInvalidSubject {
		t.Fatal(d)
	}
	if d := driver.ResolveString(ctx, "flag", "f", feature.Target{Key: strings.Repeat("x", feature.MaxStringBytes+1)}); d.Err != feature.ErrInvalidSubject {
		t.Fatal(d)
	}
	if d := driver.ResolveFloat(ctx, "flag", math.NaN(), target); d.Err != feature.ErrInvalidValue {
		t.Fatal(d)
	}
	sdk.detail = ldreason.EvaluationDetail{Reason: ldreason.NewEvalReasonTargetMatch()}
	if d := driver.ResolveString(ctx, "flag", "f", target); d.Reason != feature.ReasonRule {
		t.Fatal(d)
	}
	sdk.err = errors.New("private SDK detail")
	if d := driver.ResolveString(ctx, "flag", "f", target); d.Err != feature.ErrUnavailable || d.Value != "f" {
		t.Fatal(d)
	}
	sdk.err = nil
	sdk.value = ldvalue.Raw(json.RawMessage("{"))
	if d := driver.ResolveObject(ctx, "flag", map[string]any{}, target); d.Err != feature.ErrInvalidValue {
		t.Fatal(d)
	}
	sdk.err = errors.New("private SDK detail")
	child, cancel := context.WithCancel(ctx)
	sdk.cancel = cancel
	if d := driver.ResolveString(child, "flag", "f", target); d.Err != context.Canceled {
		t.Fatal(d)
	}
	if d := driver.ResolveString(child, "flag", "f", target); d.Err != context.Canceled {
		t.Fatal(d)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if d := driver.ResolveString(ctx, "flag", "f", target); d.Err != feature.ErrClosed {
		t.Fatal(d)
	}
}

// evaluationHook records the SDK's final native evaluation before the adapter sees it.
type evaluationHook struct {
	ldhooks.Unimplemented
	series ldhooks.EvaluationSeriesContext
	detail ldreason.EvaluationDetail
	calls  int
}

// jsonOnlyClient preserves the optional capability implemented by existing borrowed clients.
type jsonOnlyClient struct {
	testClient
	value ldvalue.Value
}

// JSONVariationDetailCtx supplies values without implementing the newer native typed capabilities.
func (c *jsonOnlyClient) JSONVariationDetailCtx(context.Context, string, ldcontext.Context, ldvalue.Value) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	return c.value, ldreason.EvaluationDetail{}, nil
}

// TestBorrowedJSONOnlyTypedReads preserves existing typed reads through the original optional interface.
func TestBorrowedJSONOnlyTypedReads(t *testing.T) {
	ctx := context.Background()
	target := feature.Target{Key: "user"}
	client := &jsonOnlyClient{value: ldvalue.String("blue")}
	driver := New(client)
	if result := driver.ResolveString(ctx, "flag", "fallback", target); result.Value != "blue" || result.Err != nil {
		t.Fatal(result)
	}
	client.value = ldvalue.Int(12)
	if result := driver.ResolveInt(ctx, "flag", 9, target); result.Value != 12 || result.Err != nil {
		t.Fatal(result)
	}
	client.value = ldvalue.Float64(1.25)
	if result := driver.ResolveFloat(ctx, "flag", 9, target); result.Value != 1.25 || result.Err != nil {
		t.Fatal(result)
	}
	client.value = ldvalue.CopyArbitraryValue(map[string]any{"color": "blue"})
	if result := driver.ResolveObject(ctx, "flag", map[string]any{}, target); result.Err != nil || result.Value.(map[string]any)["color"] != "blue" {
		t.Fatal(result)
	}
	if result := driver.ResolveInt(ctx, "flag", 9, target); result.Value != 9 || result.Err != feature.ErrTypeMismatch {
		t.Fatal(result)
	}
	if result := driver.ResolveString(ctx, "flag", "fallback", target); result.Value != "fallback" || result.Err != feature.ErrTypeMismatch {
		t.Fatal(result)
	}
}

// Metadata identifies this test observer to the SDK.
func (h *evaluationHook) Metadata() ldhooks.Metadata {
	return ldhooks.NewMetadata("typed-evaluation-test")
}

// AfterEvaluation captures native type validation and fallback evidence.
func (h *evaluationHook) AfterEvaluation(_ context.Context, series ldhooks.EvaluationSeriesContext, data ldhooks.EvaluationSeriesData, detail ldreason.EvaluationDetail) (ldhooks.EvaluationSeriesData, error) {
	h.series, h.detail = series, detail
	h.calls++
	return data, nil
}

// TestTypedSDKHooksObserveNativeTypes guards type checks inside the SDK's hook and analytics boundary.
func TestTypedSDKHooksObserveNativeTypes(t *testing.T) {
	ctx := context.Background()
	target := feature.Target{Key: "user"}
	data := ldtestdata.DataSource()
	data.Update(data.Flag("number").Variations(ldvalue.Float64(4.5)).VariationForAllIndex(0))
	data.Update(data.Flag("string").Variations(ldvalue.String("blue")).VariationForAllIndex(0))
	data.Update(data.Flag("object").Variations(ldvalue.CopyArbitraryValue(map[string]any{"color": "blue"})).VariationForAllIndex(0))
	hook := &evaluationHook{}
	driver, err := Open(ctx, Config{SDKKey: "hook-test", SDKConfig: ldclient.Config{DataSource: data, Events: ldcomponents.NoEvents(), Logging: ldcomponents.NoLogging(), Hooks: []ldhooks.Hook{hook}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Error(err)
		}
	})
	checks := []struct {
		method   string
		fallback ldvalue.Value
		resolve  func() error
	}{
		{"LDClient.StringVariationDetailCtx", ldvalue.String("fallback"), func() error {
			result := driver.ResolveString(ctx, "number", "fallback", target)
			if result.Value != "fallback" {
				t.Fatal(result)
			}
			return result.Err
		}},
		{"LDClient.Float64VariationDetailCtx", ldvalue.Float64(7.5), func() error {
			result := driver.ResolveFloat(ctx, "string", 7.5, target)
			if result.Value != 7.5 {
				t.Fatal(result)
			}
			return result.Err
		}},
		{"LDClient.Float64VariationDetailCtx", ldvalue.Float64(9), func() error {
			result := driver.ResolveInt(ctx, "string", 9, target)
			if result.Value != 9 {
				t.Fatal(result)
			}
			return result.Err
		}},
	}
	for _, check := range checks {
		before := hook.calls
		if err := check.resolve(); err != feature.ErrTypeMismatch {
			t.Fatal(err)
		}
		if hook.calls != before+1 || hook.series.Method() != check.method || hook.detail.Reason.GetErrorKind() != ldreason.EvalErrorWrongType || !hook.detail.Value.Equal(check.fallback) || !hook.series.DefaultValue().Equal(check.fallback) {
			t.Fatal(hook.series, hook.detail, hook.calls)
		}
	}
	if result := driver.ResolveObject(ctx, "object", map[string]any{}, target); result.Err != nil || hook.series.Method() != "LDClient.JSONVariationDetailCtx" {
		t.Fatal(result, hook.series)
	}
}

// TestTypedCapabilitiesAndValidation covers optional native capabilities and portable result constraints.
func TestTypedCapabilitiesAndValidation(t *testing.T) {
	ctx := context.Background()
	target := feature.Target{Key: "user"}
	booleanOnly := New(&testClient{})
	if result := booleanOnly.ResolveFloat(ctx, "flag", 1, target); result.Err != feature.ErrUnsupported {
		t.Fatal(result)
	}
	if result := booleanOnly.ResolveInt(ctx, "flag", 1, target); result.Err != feature.ErrUnsupported {
		t.Fatal(result)
	}
	if result := booleanOnly.ResolveObject(ctx, "flag", map[string]any{}, target); result.Err != feature.ErrUnsupported {
		t.Fatal(result)
	}
	if _, _, err := nativeValue(&testClient{}, ctx, "flag", ldcontext.New("user"), nil, feature.ValueType("invalid")); err != feature.ErrUnsupported {
		t.Fatal(err)
	}
	sdk := &jsonClient{value: ldvalue.String(strings.Repeat("x", feature.MaxValueBytes+1))}
	if result := New(sdk).ResolveString(ctx, "flag", "fallback", target); result.Err != feature.ErrInvalidValue {
		t.Fatal(result)
	}
}

// TestNativeValueCopyBounds checks allocation limits before copying SDK containers and raw JSON.
func TestNativeValueCopyBounds(t *testing.T) {
	deep := ldvalue.Null()
	for index := 0; index < 10; index++ {
		deep = ldvalue.ArrayOf(deep)
	}
	for _, value := range []ldvalue.Value{
		ldvalue.Raw(json.RawMessage(strings.Repeat(" ", 6*feature.MaxValueBytes+1))),
		ldvalue.Raw(json.RawMessage("{")),
		ldvalue.CopyArbitraryValue(make([]any, feature.MaxSubjects+1)),
		ldvalue.ArrayOf(ldvalue.String(strings.Repeat("x", feature.MaxValueBytes))),
		ldvalue.ObjectBuild().Set(strings.Repeat("x", feature.MaxValueBytes), ldvalue.Null()).Build(),
		deep,
	} {
		budget := 0
		if _, err := copyNativeValue(value, 0, &budget); err != feature.ErrInvalidValue {
			t.Fatal(err)
		}
	}
	budget := 0
	value, err := copyNativeValue(ldvalue.Raw(json.RawMessage(`{"nested":[9223372036854776000]}`)), 0, &budget)
	if err != nil || value.(map[string]any)["nested"].([]any)[0] != float64(1<<63) {
		t.Fatal(value, err)
	}
}
