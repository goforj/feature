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
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
)

// TestTypedOfficialSDK checks actual engine values, numeric precision, and returned object ownership.
func TestTypedOfficialSDK(t *testing.T) {
	ctx := context.Background()
	data := ldtestdata.DataSource()
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
