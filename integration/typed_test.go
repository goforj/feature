//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/launchdarklyfeature"
	"github.com/goforj/feature/driver/openfeaturefeature"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	"github.com/open-feature/go-sdk-contrib/providers/launchdarkly/pkg"
)

// typedFixture keeps vendor definitions and update mechanisms outside portable assertions.
type typedFixture struct {
	flags  *feature.Features
	prefix string
	change func()
}

// newTypedFixture constructs real native, SDK, or container-backed typed evaluation.
func newTypedFixture(t *testing.T, name string) typedFixture {
	t.Helper()
	ctx := context.Background()
	prefix := namespace()
	fixture := typedFixture{prefix: prefix}
	values := map[string]any{"string": "blue", "int": int64(12), "float": 1.25, "object": map[string]any{"color": "blue"}, "update": "before"}
	switch name {
	case "native", "openfeature-native":
		doc := feature.Document{Schema: 2, Flags: map[string]feature.Flag{}}
		for key, value := range values {
			doc.Flags[prefix+key] = feature.Flag{Default: value}
		}
		source, err := feature.NewMemorySource(doc)
		if err != nil {
			t.Fatal(err)
		}
		engine, err := feature.Open(ctx, source)
		if err != nil {
			t.Fatal(err)
		}
		fixture.flags = feature.New(engine)
		if name == "openfeature-native" {
			driver, err := openfeaturefeature.Open(ctx, openfeaturefeature.NewProvider(engine))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := driver.Close(); err != nil {
					t.Error(err)
				}
			})
			fixture.flags = feature.New(driver)
		}
		fixture.change = func() {
			doc.Flags[prefix+"update"] = feature.Flag{Default: "after"}
			if err := source.Replace(doc); err != nil {
				t.Fatal(err)
			}
			if err := engine.Refresh(ctx); err != nil {
				t.Fatal(err)
			}
		}
	case "launchdarkly", "openfeature-launchdarkly":
		service := launchDarklyRelay(t)
		for key, value := range values {
			service.publish(relayFlag(prefix+key, false).Variations(ldvalue.CopyArbitraryValue(value)).FallthroughVariation(0).OffVariation(0).Build())
		}
		config := service.sdkConfig()
		if name == "launchdarkly" {
			driver, err := launchdarklyfeature.Open(ctx, launchdarklyfeature.Config{SDKKey: evaluationKey, SDKConfig: config})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := driver.Close(); err != nil {
					t.Error(err)
				}
			})
			fixture.flags = feature.New(driver)
		} else {
			sdk, err := ldclient.MakeCustomClient(evaluationKey, config, 2*time.Second)
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
			fixture.flags = feature.New(driver)
		}
		fixture.change = func() {
			service.publish(relayFlag(prefix+"update", false).Variations(ldvalue.String("after")).FallthroughVariation(0).OffVariation(0).Build())
		}
	default:
		service := relay(t)
		definitions := map[string]any{}
		for key, value := range values {
			definitions[prefix+key] = map[string]any{"variations": map[string]any{"value": value}, "defaultRule": map[string]any{"variation": "value"}}
		}
		service.update(t, definitions)
		mode := gofeatureflag.EvaluationTypeRemote
		if name == "openfeature-goff-inprocess" {
			mode = gofeatureflag.EvaluationTypeInProcess
		}
		fixture.flags = feature.New(openRelay(t, service.endpoint, evaluationKey, mode))
		fixture.change = func() {
			service.update(t, map[string]any{prefix + "update": map[string]any{"variations": map[string]any{"value": "after"}, "defaultRule": map[string]any{"variation": "value"}}})
		}
	}
	eventually(t, func() bool {
		// Relay publishes flags independently, so one visible flag cannot establish fixture readiness.
		scope := fixture.flags.For(feature.Target{Key: "probe"})
		text := scope.String(ctx, prefix+"string", "fallback")
		integer := scope.Int(ctx, prefix+"int", 0)
		decimal := scope.Float(ctx, prefix+"float", 0)
		object := scope.Object(ctx, prefix+"object", map[string]any{})
		update := scope.String(ctx, prefix+"update", "fallback")
		attributes, ok := object.Value.(map[string]any)
		return text.Err == nil && text.Value == "blue" &&
			integer.Err == nil && integer.Value == 12 &&
			decimal.Err == nil && decimal.Value == 1.25 &&
			object.Err == nil && ok && attributes["color"] == "blue" &&
			update.Err == nil && update.Value == "before"
	})
	return fixture
}

// TestTypedDriverConformance proves the same typed application workflow against all supported configurations.
func TestTypedDriverConformance(t *testing.T) {
	for _, name := range []string{"native", "openfeature-native", "launchdarkly", "openfeature-launchdarkly", "openfeature-goff-remote", "openfeature-goff-inprocess"} {
		t.Run(name, func(t *testing.T) {
			fixture := newTypedFixture(t, name)
			scope := fixture.flags.For(feature.Target{Key: "tenant:acme"})
			ctx := context.Background()
			t.Run("string", func(t *testing.T) {
				if d := scope.String(ctx, fixture.prefix+"string", "fallback"); d.Value != "blue" || d.Err != nil {
					t.Fatal(d)
				}
			})
			t.Run("integer", func(t *testing.T) {
				if d := scope.Int(ctx, fixture.prefix+"int", 0); d.Value != 12 || d.Err != nil {
					t.Fatal(d)
				}
			})
			t.Run("float", func(t *testing.T) {
				if d := scope.Float(ctx, fixture.prefix+"float", 0); d.Value != 1.25 || d.Err != nil {
					t.Fatal(d)
				}
			})
			t.Run("object", func(t *testing.T) {
				d := scope.Object(ctx, fixture.prefix+"object", map[string]any{})
				if d.Err != nil || d.Value.(map[string]any)["color"] != "blue" {
					t.Fatal(d)
				}
				d.Value.(map[string]any)["color"] = "mutated"
				if scope.Object(ctx, fixture.prefix+"object", map[string]any{}).Value.(map[string]any)["color"] != "blue" {
					t.Fatal("SDK returned shared object")
				}
			})
			t.Run("wrong-type", func(t *testing.T) {
				if d := scope.String(ctx, fixture.prefix+"int", "fallback"); d.Value != "fallback" || d.Err != feature.ErrTypeMismatch {
					t.Fatal(d)
				}
			})
			t.Run("missing", func(t *testing.T) {
				if d := scope.String(ctx, fixture.prefix+"missing", "fallback"); d.Value != "fallback" || d.Err != feature.ErrNotFound {
					t.Fatal(d)
				}
				if d := scope.Int(ctx, fixture.prefix+"missing", 12); d.Value != 12 || d.Err != feature.ErrNotFound {
					t.Fatal(d)
				}
				if d := scope.Float(ctx, fixture.prefix+"missing", 1.25); d.Value != 1.25 || d.Err != feature.ErrNotFound {
					t.Fatal(d)
				}
				if d := scope.Object(ctx, fixture.prefix+"missing", map[string]any{"fallback": true}); d.Err != feature.ErrNotFound || d.Value.(map[string]any)["fallback"] != true {
					t.Fatal(d)
				}
			})
			t.Run("canceled", func(t *testing.T) {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				if scope.String(canceled, fixture.prefix+"string", "f").Err != context.Canceled || scope.Int(canceled, fixture.prefix+"int", 0).Err != context.Canceled || scope.Float(canceled, fixture.prefix+"float", 0).Err != context.Canceled || scope.Object(canceled, fixture.prefix+"object", map[string]any{}).Err != context.Canceled {
					t.Fatal("typed cancellation lost")
				}
			})
			t.Run("live-update", func(t *testing.T) {
				eventually(t, func() bool {
					d := scope.String(ctx, fixture.prefix+"update", "fallback")
					return d.Value == "before" && d.Err == nil
				})
				fixture.change()
				eventually(t, func() bool {
					d := scope.String(ctx, fixture.prefix+"update", "fallback")
					return d.Value == "after" && d.Err == nil
				})
			})
		})
	}
}
