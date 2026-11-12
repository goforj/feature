package feature

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
)

// TestTrackingOwnershipAndObservations verifies one bounded observation and owned event per operation.
func TestTrackingOwnershipAndObservations(t *testing.T) {
	ctx := context.Background()
	client, _ := openMemory(t)
	var mu sync.Mutex
	var observations []Observation
	var seen Target
	var event Event
	sink := TrackerFunc(func(_ context.Context, _ string, target Target, input Event) error {
		seen = target
		event = input
		return nil
	})
	observer := ObserverFunc(func(_ context.Context, event Observation) {
		mu.Lock()
		observations = append(observations, event)
		mu.Unlock()
	})
	flags := New(client, WithTracker(sink), WithObserver(observer))
	if !flags.SupportsTracking() || New(client).SupportsTracking() || client.SupportsTracking() {
		t.Fatal("tracking configuration was not discoverable")
	}
	if New(evaluatorFunc(func(context.Context, string, bool, Target) Decision { return Decision{} })).SupportsTracking() {
		t.Fatal("boolean-only driver advertised tracking")
	}
	scope := flags.For(Target{Key: "tenant:acme", Attributes: map[string]any{"plan": "pro"}})
	metric := 12.5
	attributes := map[string]any{"items": []any{"checkout"}}
	if err := scope.Track(ctx, "checkout", Event{Value: &metric, Attributes: attributes}); err != nil {
		t.Fatal(err)
	}
	metric = 0
	attributes["items"].([]any)[0] = "mutated"
	if seen.Key != "tenant:acme" || *event.Value != 12.5 || event.Attributes["items"].([]any)[0] != "checkout" {
		t.Fatal("analytics borrowed caller data")
	}
	if err := flags.Track(ctx, "untargeted", Event{}); err != nil || seen.Key != "" {
		t.Fatal(err, seen)
	}
	_ = flags.Enabled(ctx, "checkout-v2")
	_ = scope.Evaluate(ctx, "checkout-v2", false)
	_ = flags.String(ctx, "checkout-v2", "f")
	_ = flags.For(Target{Attributes: map[string]any{"bad": func() {}}}).Evaluate(ctx, "checkout-v2", true)
	if len(observations) != 6 || observations[0].Operation != "track" || observations[2].Type != TypeBoolean || observations[4].Err != ErrTypeMismatch || observations[5].Err != ErrInvalidSubject {
		t.Fatal(observations)
	}
	for _, observation := range observations {
		if observation.Duration < 0 {
			t.Fatal(observation)
		}
	}
	if err := New(client).Track(ctx, "checkout", Event{}); err != ErrUnsupported {
		t.Fatal(err)
	}
	if err := New(client, WithTracker(TrackerFunc(func(context.Context, string, Target, Event) error { return errors.New("private exporter diagnostic") }))).Track(ctx, "checkout", Event{}); err != ErrUnavailable {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := scope.Track(canceled, "checkout", Event{}); err != context.Canceled {
		t.Fatal(err)
	}
	if err := flags.Track(ctx, "", Event{}); err != ErrInvalidKey {
		t.Fatal(err)
	}
	if err := flags.For(Target{Attributes: map[string]any{"bad": func() {}}}).Track(ctx, "x", Event{}); err != ErrInvalidSubject {
		t.Fatal(err)
	}
	if err := submitEvent(ctx, "x", Target{Attributes: map[string]any{"bad": func() {}}}, nil, Event{}, sink); err != ErrInvalidSubject {
		t.Fatal(err)
	}
	if err := flags.Track(ctx, "x", Event{Attributes: map[string]any{"bad": func() {}}}); err != ErrInvalidValue {
		t.Fatal(err)
	}
	for _, metric := range []float64{math.NaN(), math.Inf(1)} {
		if _, err := (Event{Value: &metric}).Clone(); err != ErrInvalidValue {
			t.Fatal(err)
		}
	}
}

// TestNativeEventSink is discovered through the same portable tracking capability.
func TestNativeEventSink(t *testing.T) {
	ctx := context.Background()
	source, _ := NewMemorySource(sampleDocument())
	calls := 0
	engine, err := Open(ctx, source, WithEventSink(TrackerFunc(func(context.Context, string, Target, Event) error { calls++; return nil })))
	if err != nil {
		t.Fatal(err)
	}
	if err := New(engine).Track(ctx, "checkout", Event{}); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
