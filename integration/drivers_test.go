//go:build integration

package integration

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/launchdarklyfeature"
	"github.com/goforj/feature/driver/openfeaturefeature"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	"github.com/open-feature/go-sdk-contrib/providers/launchdarkly/pkg"
	"github.com/open-feature/go-sdk/openfeature"
)

// contractFixture supplies the same required behavioral inventory for every driver family.
type contractFixture struct {
	features *feature.Features
	prefix   string
	change   func(bool)
	remove   func()
}

// TestDriverConformance runs identical portable cases against native, vendor, and service-backed engines.
func TestDriverConformance(t *testing.T) {
	for _, name := range []string{"native", "launchdarkly", "openfeature-launchdarkly", "openfeature-goff-remote", "openfeature-goff-inprocess"} {
		t.Run(name, func(t *testing.T) { fixture := newContractFixture(t, name); runContract(t, fixture) })
	}
}

// newContractFixture initializes one driver while letting the real engine own its rule semantics.
func newContractFixture(t *testing.T, name string) contractFixture {
	t.Helper()
	ctx := context.Background()
	prefix := namespace()
	fixture := contractFixture{prefix: prefix}
	switch name {
	case "native":
		document := feature.Document{Schema: 1, Flags: map[string]feature.Flag{
			prefix + "true": {Default: true}, prefix + "false": {}, prefix + "update": {},
			prefix + "identity":  {Rules: []feature.Rule{{Subjects: []string{"tenant:allowed"}, Value: true}}},
			prefix + "attribute": {Rules: []feature.Rule{{Attributes: map[string]string{"plan": "pro"}, Value: true}}},
		}}
		source, err := feature.NewMemorySource(document)
		if err != nil {
			t.Fatal(err)
		}
		engine, err := feature.Open(ctx, source)
		if err != nil {
			t.Fatal(err)
		}
		fixture.features = feature.New(engine)
		publish := func() {
			if err := source.Replace(document); err != nil {
				t.Fatal(err)
			}
			if err := engine.Refresh(ctx); err != nil {
				t.Fatal(err)
			}
		}
		fixture.change = func(value bool) {
			document.Flags[prefix+"update"] = feature.Flag{Default: value}
			publish()
		}
		fixture.remove = func() {
			delete(document.Flags, prefix+"update")
			publish()
		}
	case "launchdarkly", "openfeature-launchdarkly":
		service := launchDarklyRelay(t)
		service.publish(
			relayFlag(prefix+"true", true).Build(), relayFlag(prefix+"false", false).Build(), relayFlag(prefix+"update", false).Build(),
			relayFlag(prefix+"identity", false).AddTarget(1, "tenant:allowed").Build(),
			relayFlag(prefix+"attribute", false).AddRule(ldbuilders.NewRuleBuilder().ID("plan").
				Clauses(ldbuilders.Clause("plan", ldmodel.OperatorIn, ldvalue.String("pro"))).Variation(1)).Build(),
		)
		config := launchdarklyfeature.Config{SDKKey: evaluationKey, SDKConfig: service.sdkConfig()}
		if name == "launchdarkly" {
			driver, err := launchdarklyfeature.Open(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := driver.Close(); err != nil {
					t.Error(err)
				}
			})
			fixture.features = feature.New(driver)
		} else {
			sdk, err := ldclient.MakeCustomClient(config.SDKKey, config.SDKConfig, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := sdk.Close(); err != nil {
					t.Error(err)
				}
			})
			driver, err := openfeaturefeature.Open(ctx, launchdarkly.NewProvider(sdk))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := driver.Close(); err != nil {
					t.Error(err)
				}
			})
			fixture.features = feature.New(driver)
		}
		eventually(t, func() bool { return fixture.features.For(feature.Target{Key: "probe"}).Enabled(ctx, prefix+"true") })
		fixture.change = func(value bool) { service.publish(relayFlag(prefix+"update", value).Build()) }
		fixture.remove = func() { service.publish(relayFlag(prefix+"update", false).Deleted(true).Build()) }
	default:
		service := relay(t)
		service.update(t, map[string]any{
			prefix + "true": remoteFlag(true, ""), prefix + "false": remoteFlag(false, ""), prefix + "update": remoteFlag(false, ""),
			prefix + "identity": remoteFlag(false, `targetingKey eq "tenant:allowed"`), prefix + "attribute": remoteFlag(false, `plan eq "pro"`),
		})
		mode := gofeatureflag.EvaluationTypeRemote
		if name == "openfeature-goff-inprocess" {
			mode = gofeatureflag.EvaluationTypeInProcess
		}
		provider, err := gofeatureflag.NewProvider(gofeatureflag.ProviderOptions{Endpoint: service.endpoint, APIKey: evaluationKey, EvaluationType: mode, FlagChangePollingInterval: 100 * time.Millisecond, DataCollectorDisabled: true, DisableCache: true, HTTPClient: &http.Client{Timeout: 2 * time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		// Initial flag publication is asynchronous in the real service.
		probe, err := openfeaturefeature.Open(ctx, provider)
		if err != nil {
			t.Fatal(err)
		}
		fixture.features = feature.New(probe)
		t.Cleanup(func() {
			if err := probe.Close(); err != nil {
				t.Error(err)
			}
		})
		eventually(t, func() bool { return fixture.features.For(feature.Target{Key: "probe"}).Enabled(ctx, prefix+"true") })
		fixture.change = func(value bool) { service.update(t, map[string]any{prefix + "update": remoteFlag(value, "")}) }
		fixture.remove = func() {
			service.mu.Lock()
			delete(service.flags, prefix+"update")
			service.mu.Unlock()
		}
	}
	return fixture
}

// runContract checks values, fallbacks, owned targeting, cancellation, updates, and concurrent use.
func runContract(t *testing.T, fixture contractFixture) {
	ctx := context.Background()
	scope := fixture.features.For(feature.Target{Key: "tenant:allowed", Attributes: map[string]any{"plan": "pro", "paid": true, "seats": int64(12)}})
	cases := []struct {
		name string
		test func(*testing.T)
	}{
		{"true", func(t *testing.T) {
			if !scope.Enabled(ctx, fixture.prefix+"true") {
				t.Fatal("true value lost")
			}
		}},
		{"false-is-success", func(t *testing.T) {
			d := scope.Evaluate(ctx, fixture.prefix+"false", true)
			if d.Value || d.Err != nil {
				t.Fatal(d)
			}
		}},
		{"missing-false", func(t *testing.T) {
			d := scope.Evaluate(ctx, fixture.prefix+"missing", false)
			if d.Value || d.Err != feature.ErrNotFound || d.Reason != feature.ReasonMissing {
				t.Fatal(d)
			}
		}},
		{"missing-true", func(t *testing.T) {
			d := scope.Evaluate(ctx, fixture.prefix+"missing", true)
			if !d.Value || d.Err != feature.ErrNotFound {
				t.Fatal(d)
			}
		}},
		{"identity", func(t *testing.T) {
			if !scope.Enabled(ctx, fixture.prefix+"identity") || fixture.features.For(feature.Target{Key: "tenant:other"}).Enabled(ctx, fixture.prefix+"identity") {
				t.Fatal("identity targeting changed")
			}
		}},
		{"owned-target", func(t *testing.T) {
			target := feature.Target{Key: "tenant:allowed", Attributes: map[string]any{"plan": "pro"}}
			owned := fixture.features.For(target)
			target.Attributes["plan"] = "free"
			if !owned.Enabled(ctx, fixture.prefix+"attribute") || fixture.features.For(target).Enabled(ctx, fixture.prefix+"attribute") {
				t.Fatal("scope borrowed attributes")
			}
		}},
		{"canceled", func(t *testing.T) {
			child, cancel := context.WithCancel(ctx)
			cancel()
			d := scope.Evaluate(child, fixture.prefix+"true", true)
			if !d.Value || d.Err != context.Canceled {
				t.Fatal(d)
			}
		}},
		{"concurrent", func(t *testing.T) {
			var group sync.WaitGroup
			for i := 0; i < 16; i++ {
				group.Add(1)
				go func() {
					defer group.Done()
					for j := 0; j < 8; j++ {
						if !scope.Enabled(ctx, fixture.prefix+"true") {
							t.Error("concurrent result changed")
						}
					}
				}()
			}
			group.Wait()
		}},
		{"live-update", func(t *testing.T) {
			if d := scope.Evaluate(ctx, fixture.prefix+"update", true); d.Value || d.Err != nil {
				t.Fatal("initial state changed", d)
			}
			fixture.change(true)
			eventually(t, func() bool {
				d := scope.Evaluate(ctx, fixture.prefix+"update", false)
				return d.Value && d.Err == nil
			})
			fixture.change(false)
			eventually(t, func() bool {
				d := scope.Evaluate(ctx, fixture.prefix+"update", true)
				return !d.Value && d.Err == nil
			})
		}},
		{"delete-and-recreate", func(t *testing.T) {
			fixture.change(true)
			eventually(t, func() bool {
				d := scope.Evaluate(ctx, fixture.prefix+"update", false)
				return d.Value && d.Err == nil
			})
			fixture.remove()
			eventually(t, func() bool {
				for _, fallback := range []bool{false, true} {
					d := scope.Evaluate(ctx, fixture.prefix+"update", fallback)
					if d.Value != fallback || d.Err != feature.ErrNotFound || d.Reason != feature.ReasonMissing {
						return false
					}
				}
				return true
			})
			fixture.change(false)
			eventually(t, func() bool {
				d := scope.Evaluate(ctx, fixture.prefix+"update", true)
				return !d.Value && d.Err == nil
			})
		}},
	}
	for _, test := range cases {
		t.Run(test.name, test.test)
	}
}

// TestOpenFeatureSDKTypeMismatch exercises a real typed provider failure through the adapter.
func TestOpenFeatureSDKTypeMismatch(t *testing.T) {
	service := relay(t)
	key := namespace() + "string"
	service.update(t, map[string]any{key: map[string]any{"variations": map[string]any{"text": "not-a-boolean"}, "defaultRule": map[string]any{"variation": "text"}}})
	provider, err := gofeatureflag.NewProvider(gofeatureflag.ProviderOptions{Endpoint: service.endpoint, APIKey: evaluationKey, EvaluationType: gofeatureflag.EvaluationTypeRemote, DataCollectorDisabled: true, DisableCache: true})
	if err != nil {
		t.Fatal(err)
	}
	driver, err := openfeaturefeature.Open(context.Background(), provider)
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	eventually(t, func() bool {
		d := feature.New(driver).For(feature.Target{Key: "tenant:acme"}).Evaluate(context.Background(), key, true)
		return d.Value && d.Err == feature.ErrTypeMismatch
	})
}

var _ openfeature.FeatureProvider = (*gofeatureflag.Provider)(nil)
