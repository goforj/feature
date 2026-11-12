package launchdarklyfeature

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
)

// TestConcurrentCloseSharesCompletion verifies every caller observes one cleanup and its stable result.
func TestConcurrentCloseSharesCompletion(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(fmt.Sprint(fails), func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			var finished atomic.Bool
			var want error
			if fails {
				want = feature.ErrUnavailable
			}
			driver := New(&testClient{})
			driver.close = func() error {
				calls.Add(1)
				close(entered)
				<-release
				finished.Store(true)
				if fails {
					return errors.New("private shutdown diagnostic")
				}
				return nil
			}
			owner := make(chan error, 1)
			go func() { owner <- driver.Close() }()
			<-entered
			if d := driver.Resolve(context.Background(), "checkout", true, feature.Target{}); d.Err != feature.ErrClosed {
				t.Error(d)
			}
			var group sync.WaitGroup
			for i := 0; i < 16; i++ {
				group.Add(1)
				go func() {
					defer group.Done()
					if err := driver.Close(); err != want || !finished.Load() {
						t.Error("close returned before cleanup or lost its result", err)
					}
				}()
			}
			close(release)
			group.Wait()
			if err := <-owner; err != want || calls.Load() != 1 {
				t.Fatal(err, calls.Load())
			}
			if err := driver.Close(); err != want {
				t.Fatal("repeated close lost cleanup error", err)
			}
		})
	}
}

// testClient exposes controlled lifecycle and resolution failures without network access.
type testClient struct {
	resolve    func(context.Context, string, ldcontext.Context, bool) (bool, ldreason.EvaluationDetail, error)
	checks     atomic.Int32
	readyAfter int32
	closes     atomic.Int32
	closeErr   error
}

// BoolVariationDetailCtx supplies structured native SDK resolution evidence.
func (c *testClient) BoolVariationDetailCtx(ctx context.Context, key string, target ldcontext.Context, fallback bool) (bool, ldreason.EvaluationDetail, error) {
	return c.resolve(ctx, key, target, fallback)
}

// Initialized allows deterministic polling progress without timing-dependent goroutines.
func (c *testClient) Initialized() bool { return c.checks.Add(1) > c.readyAfter }

// Close records disposal of owned and partially initialized clients.
func (c *testClient) Close() error { c.closes.Add(1); return c.closeErr }

// TestResolveNativeEvidence verifies successful values, variation IDs, and every error class.
func TestResolveNativeEvidence(t *testing.T) {
	ctx := context.Background()
	var detail ldreason.EvaluationDetail
	var providerErr error
	sdk := &testClient{resolve: func(context.Context, string, ldcontext.Context, bool) (bool, ldreason.EvaluationDetail, error) {
		return true, detail, providerErr
	}}
	driver := New(sdk)
	target := feature.Target{Key: "tenant:acme"}
	reasons := []struct {
		reason ldreason.EvaluationReason
		want   feature.Reason
	}{
		{ldreason.NewEvalReasonOff(), feature.ReasonDisabled}, {ldreason.NewEvalReasonFallthrough(), feature.ReasonDefault},
		{ldreason.NewEvalReasonTargetMatch(), feature.ReasonRule}, {ldreason.NewEvalReasonRuleMatch(1, "rule"), feature.ReasonRule},
		{ldreason.NewEvalReasonPrerequisiteFailed("other"), feature.ReasonUnknown},
	}
	for _, tc := range reasons {
		detail = ldreason.EvaluationDetail{Reason: tc.reason, VariationIndex: ldvalue.NewOptionalInt(1)}
		d := driver.Resolve(ctx, "flag", false, target)
		if !d.Value || d.Reason != tc.want || d.Variant != "1" || d.Err != nil {
			t.Fatal(d)
		}
	}
	for _, tc := range []struct {
		kind ldreason.EvalErrorKind
		want error
	}{{ldreason.EvalErrorFlagNotFound, feature.ErrNotFound}, {ldreason.EvalErrorWrongType, feature.ErrTypeMismatch}, {ldreason.EvalErrorUserNotSpecified, feature.ErrInvalidSubject}, {ldreason.EvalErrorClientNotReady, feature.ErrUnavailable}, {"", feature.ErrUnavailable}} {
		detail = ldreason.EvaluationDetail{Reason: ldreason.NewEvalReasonError(tc.kind)}
		d := driver.Resolve(ctx, "flag", false, target)
		if d.Value || d.Err != tc.want {
			t.Fatal(d)
		}
	}
	detail = ldreason.EvaluationDetail{}
	providerErr = errors.New("private credentials")
	if d := driver.Resolve(ctx, "flag", false, target); d.Err != feature.ErrUnavailable {
		t.Fatal(d)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if d := driver.Resolve(canceled, "flag", true, target); d.Err != context.Canceled || !d.Value {
		t.Fatal(d)
	}
	if d := driver.Resolve(ctx, "flag", false, feature.Target{Key: strings.Repeat("x", feature.MaxStringBytes+1)}); d.Err != feature.ErrInvalidSubject {
		t.Fatal(d)
	}
	if d := driver.Resolve(ctx, "flag", false, feature.Target{}); d.Err != feature.ErrInvalidSubject {
		t.Fatal(d)
	}
	child, cancel := context.WithCancel(ctx)
	sdk.resolve = func(context.Context, string, ldcontext.Context, bool) (bool, ldreason.EvaluationDetail, error) {
		cancel()
		return false, ldreason.EvaluationDetail{}, context.Canceled
	}
	if d := driver.Resolve(child, "flag", true, target); d.Err != context.Canceled || !d.Value {
		t.Fatal(d)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if d := driver.Resolve(ctx, "flag", true, target); d.Err != feature.ErrClosed || !d.Value {
		t.Fatal(d)
	}
	if err := driver.Close(); err != nil || sdk.closes.Load() != 0 {
		t.Fatal("borrowed SDK was closed", err)
	}
}

// TestContextMapping preserves typed attributes and privacy while rejecting malformed identities.
func TestContextMapping(t *testing.T) {
	native, err := toContext(feature.Target{Key: "tenant:acme", Kind: "organization", Attributes: map[string]any{"paid": true, "seats": int64(12), "anonymous": true, "privateAttributes": []string{"email"}, "email": "fake@example.test", "key": "cannot-override", "kind": "user", "targetingKey": "cannot-override"}})
	if err != nil || native.Key() != "tenant:acme" || native.Kind() != "organization" || !native.GetValue("paid").BoolValue() || native.GetValue("seats").IntValue() != 12 || !native.Anonymous() || native.PrivateAttributeCount() != 1 {
		t.Fatal(native, err)
	}
	multi, err := toContext(feature.Target{Kind: "multi", Attributes: map[string]any{"user": map[string]any{"key": "user:7"}, "organization": map[string]any{"key": "tenant:acme", "paid": true}}})
	if err != nil || multi.IndividualContextCount() != 2 || multi.IndividualContextKeyByKind("organization") != "tenant:acme" {
		t.Fatal(multi, err)
	}
	for _, target := range []feature.Target{
		{Key: ""}, {Key: "user", Kind: "kind"}, {Key: "user", Attributes: map[string]any{"anonymous": "true"}},
		{Key: "user", Attributes: map[string]any{"privateAttributes": "email"}},
		{Kind: "multi"}, {Kind: "multi", Attributes: map[string]any{"user": "wrong"}},
		{Kind: "multi", Attributes: map[string]any{"user": map[string]any{"key": true}}},
		{Kind: "multi", Attributes: map[string]any{"user": map[string]any{"key": ""}}},
	} {
		if _, err := toContext(target); err != feature.ErrInvalidSubject {
			t.Fatal(target, err)
		}
	}
}

// TestOpenOwnershipAndFailures verifies every initialization, cancellation, and cleanup path.
func TestOpenOwnershipAndFailures(t *testing.T) {
	ctx := context.Background()
	config := Config{SDKKey: "public-test-placeholder"}
	sdk := &testClient{}
	create := func(string, ldclient.Config) (managedClient, error) { return sdk, nil }
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := open(canceled, config, create); err != context.Canceled {
		t.Fatal(err)
	}
	for _, bad := range []Config{{}, {SDKKey: "x", InitTimeout: -1}} {
		if _, err := open(ctx, bad, create); err != ErrInvalidConfig {
			t.Fatal(err)
		}
	}
	if _, err := open(ctx, config, func(string, ldclient.Config) (managedClient, error) { return nil, errors.New("private SDK key") }); err != feature.ErrUnavailable {
		t.Fatal(err)
	}
	if _, err := open(ctx, config, func(string, ldclient.Config) (managedClient, error) { return sdk, errors.New("partial start") }); err != feature.ErrUnavailable || sdk.closes.Load() != 1 {
		t.Fatal(err)
	}
	sdk = &testClient{readyAfter: 1}
	driver, err := open(ctx, config, func(string, ldclient.Config) (managedClient, error) { return sdk, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != nil || sdk.closes.Load() != 1 {
		t.Fatal(err)
	}
	sdk = &testClient{readyAfter: 100000}
	short := config
	short.InitTimeout = time.Millisecond
	if _, err := open(ctx, short, func(string, ldclient.Config) (managedClient, error) { return sdk, nil }); err != context.DeadlineExceeded || sdk.closes.Load() != 1 {
		t.Fatal(err)
	}
	child, cancel := context.WithCancel(ctx)
	sdk = &testClient{}
	if _, err := open(child, config, func(string, ldclient.Config) (managedClient, error) { cancel(); return sdk, nil }); err != context.Canceled || sdk.closes.Load() != 1 {
		t.Fatal(err)
	}
	sdk = &testClient{closeErr: errors.New("private SDK data")}
	driver, err = open(ctx, config, func(string, ldclient.Config) (managedClient, error) { return sdk, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != feature.ErrUnavailable {
		t.Fatal(err)
	}
}

// TestOfficialSDKConfigurationFailure returns a safe error when the native constructor produces a nil client.
func TestOfficialSDKConfigurationFailure(t *testing.T) {
	driver, err := Open(context.Background(), Config{
		SDKKey: "public-test-placeholder",
		SDKConfig: ldclient.Config{
			Logging: ldcomponents.NoLogging(), Events: ldcomponents.NoEvents(),
			HTTP: ldcomponents.HTTPConfiguration().CACertFile(filepath.Join(t.TempDir(), "missing-private-ca.pem")),
		},
	})
	if driver != nil {
		_ = driver.Close()
	}
	if driver != nil || err != feature.ErrUnavailable {
		t.Fatalf("driver=%v error=%v", driver, err)
	}
}

// TestFailedCreationKeepsCancellation checks error precedence even when SDK construction only partially succeeds.
func TestFailedCreationKeepsCancellation(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sdk := &testClient{}
			driver, err := open(ctx, Config{SDKKey: "public-test-placeholder"}, func(string, ldclient.Config) (managedClient, error) {
				cancel()
				if partial {
					return sdk, errors.New("private constructor diagnostic")
				}
				return nil, errors.New("private constructor diagnostic")
			})
			if driver != nil {
				_ = driver.Close()
			}
			if driver != nil || err != context.Canceled {
				t.Fatalf("driver=%v error=%v", driver, err)
			}
			want := int32(0)
			if partial {
				want = 1
			}
			if sdk.closes.Load() != want {
				t.Fatal("partial SDK cleanup changed", sdk.closes.Load())
			}
		})
	}
}

// TestOfficialSDKThroughOwnedDriver exercises the actual engine and live updates without cloud credentials.
func TestOfficialSDKThroughOwnedDriver(t *testing.T) {
	ctx := context.Background()
	data := ldtestdata.DataSource()
	data.Update(data.Flag("checkout").BooleanFlag().VariationForAll(true))
	driver, err := Open(ctx, Config{SDKKey: "public-test-placeholder", SDKConfig: ldclient.Config{DataSource: data, Events: ldcomponents.NoEvents(), Logging: ldcomponents.NoLogging()}})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	features := feature.New(driver).For(feature.Target{Key: "tenant:acme"})
	if !features.Enabled(ctx, "checkout") {
		t.Fatal("real SDK result lost")
	}
	data.Update(data.Flag("checkout").BooleanFlag().OffVariation(true).On(false))
	if d := features.Evaluate(ctx, "checkout", false); !d.Value || d.Reason != feature.ReasonDisabled {
		t.Fatal(d)
	}
	data.Update(data.Flag("checkout").BooleanFlag().VariationForAll(false))
	if features.Enabled(ctx, "checkout") {
		t.Fatal("real SDK update lost")
	}
}
