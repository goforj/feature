package feature

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// booleanOnlySnapshotEvaluator supplies a driver without any snapshot capability.
type booleanOnlySnapshotEvaluator struct{}

// Resolve keeps the unsupported-capability probe independent of the native engine.
func (booleanOnlySnapshotEvaluator) Resolve(context.Context, string, bool, Target) Decision {
	return Decision{Value: true}
}

// TestFacadeSnapshotPreservesPolicies freezes decisions while retaining explicit observation and tracking.
func TestFacadeSnapshotPreservesPolicies(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	var observed, tracked atomic.Int64
	source, err := NewMemorySource(Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{
		"theme":  {Default: "dark"},
		"launch": {Default: false, Rules: []Rule{{Start: &start, Value: true}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := Open(ctx, source, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	flags := New(engine, WithObserver(ObserverFunc(func(context.Context, Observation) { observed.Add(1) })), WithTracker(TrackerFunc(func(_ context.Context, _ string, target Target, _ Event) error {
		if target.Key != "tenant" {
			t.Error("target not retained", target)
		}
		tracked.Add(1)
		return nil
	})))
	captured, err := flags.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if captured == flags || captured.evaluator == flags.evaluator {
		t.Fatal("live facade reused")
	}
	scope := captured.For(Target{Key: "tenant"})
	if err := source.Replace(Document{Schema: TypedSchemaVersion, Flags: map[string]Flag{"theme": {Default: "light"}, "launch": {Default: false}}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	now = start.Add(-time.Hour)
	if captured.String(ctx, "theme", "fallback").Value != "dark" || flags.String(ctx, "theme", "fallback").Value != "light" || !scope.Enabled(ctx, "launch") {
		t.Fatal("snapshot did not freeze definitions and clock")
	}
	if observed.Load() != 3 {
		t.Fatal("observer not retained", observed.Load())
	}
	if !captured.SupportsTracking() || scope.Track(ctx, "event", Event{}) != nil || tracked.Load() != 1 {
		t.Fatal("tracker not retained")
	}
	second, err := captured.Snapshot()
	if err != nil || second.evaluator != captured.evaluator || !second.Enabled(ctx, "launch") {
		t.Fatal("recapture changed frozen state", err)
	}
}

// TestFacadeSnapshotCapabilities preserves native sinks and exposes unsupported drivers honestly.
func TestFacadeSnapshotCapabilities(t *testing.T) {
	ctx := context.Background()
	var tracked int
	source, err := NewMemorySource(Document{Schema: SchemaVersion, Flags: map[string]Flag{}})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := Open(ctx, source, WithEventSink(TrackerFunc(func(context.Context, string, Target, Event) error { tracked++; return nil })))
	if err != nil {
		t.Fatal(err)
	}
	captured, err := New(engine).Snapshot()
	if err != nil || !captured.SupportsTracking() || captured.Track(ctx, "event", Event{}) != nil || tracked != 1 {
		t.Fatal("native sink not retained", err)
	}
	disabled, err := New(engine, WithTracker(nil)).Snapshot()
	if err != nil || disabled.SupportsTracking() || disabled.Track(ctx, "event", Event{}) != ErrUnsupported {
		t.Fatal("tracking override ignored", err)
	}
	if captured, err := New(booleanOnlySnapshotEvaluator{}).Snapshot(); captured != nil || err != ErrUnsupported {
		t.Fatal("unsupported driver claimed frozen state", captured, err)
	}
}
