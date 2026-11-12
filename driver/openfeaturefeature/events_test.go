package openfeaturefeature

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

// awaitEvent uses the native SDK callback as the synchronization boundary.
func awaitEvent(t *testing.T, events <-chan openfeature.EventDetails) openfeature.EventDetails {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("SDK event not delivered")
		return openfeature.EventDetails{}
	}
}

// TestNativeProviderEventsThroughSDK verifies updates, stale state, recovery, and retained decisions.
func TestNativeProviderEventsThroughSDK(t *testing.T) {
	ctx := context.Background()
	fail := false
	doc := feature.Document{Schema: 1, Flags: map[string]feature.Flag{"flag": {Default: true}}}
	engine, err := feature.Open(ctx, feature.SourceFunc(func(context.Context) (feature.Document, error) {
		if fail {
			return feature.Document{}, errors.New("private source text")
		}
		return doc, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	provider := NewProvider(engine)
	api := isolated.NewAPI()
	t.Cleanup(func() { _ = api.Shutdown(ctx) })
	if err := api.SetProviderAndWait(ctx, provider); err != nil {
		t.Fatal(err)
	}
	sdk := api.NewClient()
	updates := make(chan openfeature.EventDetails, 4)
	stale := make(chan openfeature.EventDetails, 2)
	ready := make(chan openfeature.EventDetails, 2)
	onUpdate := func(event openfeature.EventDetails) { updates <- event }
	onStale := func(event openfeature.EventDetails) { stale <- event }
	onReady := func(event openfeature.EventDetails) { ready <- event }
	sdk.AddHandler(openfeature.ProviderConfigChange, &onUpdate)
	sdk.AddHandler(openfeature.ProviderStale, &onStale)
	doc.Flags["second"] = feature.Flag{Default: false}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	update := awaitEvent(t, updates)
	if (update.FlagChanges != nil && (len(update.FlagChanges) != 1 || update.FlagChanges[0] != "second")) || update.EventMetadata["revision"] != engine.Status().Revision {
		t.Fatal(update)
	}
	fail = true
	if err := engine.Refresh(ctx); err != feature.ErrSource {
		t.Fatal(err)
	}
	if event := awaitEvent(t, stale); strings.Contains(event.Message, "private") {
		t.Fatal(event)
	}
	if value, err := sdk.BooleanValue(ctx, "flag", false, openfeature.NewTargetlessEvaluationContext(nil)); !value || err != nil {
		t.Fatal("stale provider discarded its valid snapshot", value, err)
	}
	sdk.AddHandler(openfeature.ProviderReady, &onReady)
	fail = false
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, ready)
	awaitEvent(t, updates)
	if err := provider.ShutdownWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	if !engine.Enabled(ctx, "flag", false, feature.Subject{}) {
		t.Fatal("provider shutdown stopped borrowed engine")
	}
}

// booleanEvaluator supplies a custom model with no native subscription capability.
type booleanEvaluator struct{}

// Resolve returns a portable result without native lifecycle resources.
func (booleanEvaluator) Resolve(context.Context, string, bool, feature.Target) feature.Decision {
	return feature.Decision{Value: true}
}

// TestProviderListenerOwnership verifies optional subscriptions and bounded cleanup.
func TestProviderListenerOwnership(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider(booleanEvaluator{})
	if err := provider.InitWithContext(ctx, openfeature.NewTargetlessEvaluationContext(nil)); err != nil {
		t.Fatal(err)
	}
	if err := provider.InitWithContext(ctx, openfeature.NewTargetlessEvaluationContext(nil)); err != nil {
		t.Fatal(err)
	}
	provider.Shutdown()
	if err := provider.startEvents(ctx); err != feature.ErrClosed {
		t.Fatal(err)
	}
	engine, _, _, _ := nativeProviderFixture(t)
	var subscriptions []*feature.Subscription
	for i := 0; i < feature.MaxSubscriptions-1; i++ {
		subscription, err := engine.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	blocked := NewProvider(engine)
	if err := blocked.InitWithContext(ctx, openfeature.NewTargetlessEvaluationContext(nil)); err != feature.ErrUnavailable {
		t.Fatal(err)
	}
	blocked.Shutdown()
	for _, subscription := range subscriptions {
		_ = subscription.Close()
	}
	closed := NewProvider(engine)
	if err := closed.InitWithContext(ctx, openfeature.NewTargetlessEvaluationContext(nil)); err != nil {
		t.Fatal(err)
	}
	_ = closed.subscription.Close()
	select {
	case <-closed.done:
	case <-time.After(time.Second):
		t.Fatal("closed source listener leaked forwarder")
	}
	closed.Shutdown()
	stopped := NewProvider(booleanEvaluator{})
	stopped.stop = make(chan struct{})
	close(stopped.stop)
	for i := 0; i < cap(stopped.events); i++ {
		stopped.events <- openfeature.Event{}
	}
	if stopped.emitChange(openfeature.ProviderConfigChange, feature.Change{}) {
		t.Fatal("stopped event dispatch remained blocked")
	}
}

// TestBlockedProviderDispatchStops makes stale, recovery, and config delivery interruptible.
func TestBlockedProviderDispatchStops(t *testing.T) {
	for _, stage := range []string{"stale", "ready", "config"} {
		t.Run(stage, func(t *testing.T) {
			provider := NewProvider(booleanEvaluator{})
			provider.stop = make(chan struct{})
			provider.done = make(chan struct{})
			provider.events = make(chan openfeature.Event)
			changes := make(chan feature.Change)
			go provider.forwardChanges(&feature.Subscription{Changes: changes})
			if stage == "ready" {
				changes <- feature.Change{Err: feature.ErrSource}
				<-provider.events
			}
			if stage == "stale" {
				changes <- feature.Change{Err: feature.ErrSource}
			} else {
				changes <- feature.Change{Revision: "next", PreviousRevision: "previous"}
			}
			close(provider.stop)
			select {
			case <-provider.done:
			case <-time.After(time.Second):
				t.Fatal("blocked provider event leaked forwarder")
			}
		})
	}
}

// TestSDKCustomTracking verifies merged identity, native submission, and observable sink failures.
func TestSDKCustomTracking(t *testing.T) {
	ctx := context.Background()
	events := make(chan feature.Event, 4)
	targets := make(chan feature.Target, 4)
	observations := make(chan feature.Observation, 4)
	source, _ := feature.NewMemorySource(feature.Document{Schema: 1, Flags: map[string]feature.Flag{}})
	engine, err := feature.Open(ctx, source, feature.WithEventSink(feature.TrackerFunc(func(_ context.Context, _ string, target feature.Target, event feature.Event) error {
		targets <- target
		events <- event
		return errors.New("private sink error")
	})))
	if err != nil {
		t.Fatal(err)
	}
	provider := NewProvider(engine, feature.WithObserver(feature.ObserverFunc(func(_ context.Context, event feature.Observation) { observations <- event })))
	api := isolated.NewAPI()
	t.Cleanup(func() { _ = api.Shutdown(ctx) })
	if err := api.SetProviderAndWait(ctx, provider); err != nil {
		t.Fatal(err)
	}
	sdk := api.NewClient()
	sdk.SetEvaluationContext(openfeature.NewEvaluationContext("client", map[string]any{"plan": "pro"}))
	driver := New(sdk)
	if !driver.SupportsTracking() || !provider.SupportsTracking() {
		t.Fatal("configured SDK tracking was not discoverable")
	}
	flags := feature.New(driver).For(feature.Target{Key: "request", Kind: "organization"})
	metric := 12.5
	if err := flags.Track(ctx, "checkout", feature.Event{Value: &metric, Attributes: map[string]any{"currency": "USD"}}); err != nil {
		t.Fatal(err)
	}
	if target := <-targets; target.Key != "request" || target.Kind != "organization" || target.Attributes["plan"] != "pro" {
		t.Fatal(target)
	}
	if event := <-events; event.Value == nil || *event.Value != metric || event.Attributes["currency"] != "USD" {
		t.Fatal(event)
	}
	if event := <-observations; event.Err != feature.ErrUnavailable {
		t.Fatal("void SDK tracking hid the observer result", event)
	}
	if err := driver.Track(ctx, "unscoped", feature.Target{}, feature.Event{}); err != nil {
		t.Fatal(err)
	}
	<-targets
	<-events
	<-observations
	if err := New(clientFunc(nil)).Track(ctx, "x", feature.Target{}, feature.Event{}); err != feature.ErrUnsupported {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		target feature.Target
		event  feature.Event
		want   error
	}{
		{"", feature.Target{}, feature.Event{}, feature.ErrInvalidKey},
		{"x", feature.Target{Attributes: map[string]any{"bad": func() {}}}, feature.Event{}, feature.ErrInvalidSubject},
		{"x", feature.Target{}, feature.Event{Attributes: map[string]any{"bad": func() {}}}, feature.ErrInvalidValue},
	} {
		if err := driver.Track(ctx, tc.name, tc.target, tc.event); err != tc.want {
			t.Fatal(err)
		}
	}
	bad := math.NaN()
	if err := driver.Track(ctx, "x", feature.Target{}, feature.Event{Value: &bad}); err != feature.ErrInvalidValue {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := driver.Track(canceled, "x", feature.Target{}, feature.Event{}); err != context.Canceled {
		t.Fatal(err)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if err := driver.Track(ctx, "x", feature.Target{}, feature.Event{}); err != feature.ErrClosed {
		t.Fatal(err)
	}
	provider.Track(ctx, "invalid", openfeature.NewEvaluationContext("x", map[string]any{"kind": 12}), openfeature.NewTrackingEventDetails(1))
	provider.Shutdown()
	provider.Track(ctx, "closed", openfeature.NewTargetlessEvaluationContext(nil), openfeature.NewTrackingEventDetails(1))
}

// evaluationOnlyProvider models a provider without the SDK's optional tracking interface.
type evaluationOnlyProvider struct{ openfeature.FeatureProvider }

// TestOwnedUnsupportedTracking rejects known provider gaps rather than accepting silent SDK no-ops.
func TestOwnedUnsupportedTracking(t *testing.T) {
	ctx := context.Background()
	driver, err := Open(ctx, evaluationOnlyProvider{FeatureProvider: openfeature.NoopProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	if driver.SupportsTracking() {
		t.Fatal("non-tracking provider advertised tracking")
	}
	if err := driver.Track(ctx, "checkout", feature.Target{}, feature.Event{}); err != feature.ErrUnsupported {
		t.Fatal(err)
	}
	if New(clientFunc(nil)).SupportsTracking() {
		t.Fatal("narrow borrowed client advertised tracking")
	}
	engine, _, _, _ := nativeProviderFixture(t)
	native, err := Open(ctx, NewProvider(engine))
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	if native.SupportsTracking() {
		t.Fatal("native provider without a sink advertised tracking")
	}
}
