package feature

import (
	"context"
	"math"
	"time"
)

// Event carries bounded custom analytics without prescribing a vendor event model.
type Event struct {
	Value      *float64       `json:"value,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// Tracker submits custom events; successful submission does not imply remote delivery.
// Implementations must support concurrent use and must not mutate inputs.
type Tracker interface {
	Track(context.Context, string, Target, Event) error
}

// trackingCapability distinguishes configured support from an optional method set.
type trackingCapability interface{ SupportsTracking() bool }

// SupportsTracking reports configured tracking support, including native sink selection.
// Borrowed OpenFeature clients can report SDK submission support without proving provider delivery.
func (f *Features) SupportsTracking() bool { return trackingEnabled(f.tracker) }

// trackingEnabled asks an optional capability reporter without submitting an event.
func trackingEnabled(tracker Tracker) bool {
	if tracker == nil {
		return false
	}
	if capability, ok := tracker.(trackingCapability); ok {
		return capability.SupportsTracking()
	}
	return true
}

// TrackerFunc adapts an application exporter or analytics sink to Tracker.
type TrackerFunc func(context.Context, string, Target, Event) error

// Track delegates custom event submission to the injected sink.
func (f TrackerFunc) Track(ctx context.Context, name string, target Target, event Event) error {
	return f(ctx, name, target, event)
}

// Observation describes a completed operation without recording values or targeting data.
// Key can be high-cardinality; observers own their metric-label policy.
type Observation struct {
	Operation string        `json:"operation"`
	Key       string        `json:"key"`
	Type      ValueType     `json:"value_type,omitempty"`
	Reason    Reason        `json:"reason,omitempty"`
	Revision  string        `json:"revision,omitempty"`
	Variant   string        `json:"variant,omitempty"`
	Err       error         `json:"-"`
	Duration  time.Duration `json:"duration_ns"`
}

// Observer receives synchronous operation results and must support concurrent use.
type Observer interface {
	Observe(context.Context, Observation)
}

// ObserverFunc adapts an application metrics, tracing, or audit callback.
type ObserverFunc func(context.Context, Observation)

// Observe invokes the explicitly configured callback without background workers.
func (f ObserverFunc) Observe(ctx context.Context, event Observation) { f(ctx, event) }

// Option configures optional facade observability and tracking ownership.
type Option func(*featuresConfig)

// featuresConfig distinguishes optional observers and explicit tracking overrides.
type featuresConfig struct {
	observer Observer
	tracker  Tracker
}

// WithObserver attaches an application-owned observer without logging values or identities.
func WithObserver(observer Observer) Option {
	return func(config *featuresConfig) { config.observer = observer }
}

// WithTracker directs custom events to an application-owned sink instead of the driver.
func WithTracker(tracker Tracker) Option {
	return func(config *featuresConfig) { config.tracker = tracker }
}

// observe keeps the optional observer outside provider-specific evaluation logic.
func (f *Features) observe(ctx context.Context, event Observation) {
	if f.observer != nil {
		f.observer.Observe(ctx, event)
	}
}

// Clone owns custom attributes and an optional finite metric before submission.
func (event Event) Clone() (Event, error) {
	value, err := CloneValue(event.Attributes)
	if err != nil {
		return Event{}, ErrInvalidValue
	}
	result := Event{Attributes: value.(map[string]any)}
	if event.Value != nil {
		metric := *event.Value
		if math.IsNaN(metric) || math.IsInf(metric, 0) {
			return Event{}, ErrInvalidValue
		}
		result.Value = &metric
	}
	return result, nil
}

// Track submits an untargeted custom event through the configured capability.
func (f *Features) Track(ctx context.Context, name string, event Event) error {
	return f.track(ctx, name, Target{}, nil, event)
}

// Track submits a custom event with the scope's owned request targeting.
func (s *Scope) Track(ctx context.Context, name string, event Event) error {
	return s.features.track(ctx, name, s.target, s.err, event)
}

// track validates and owns all analytics input before any sink can observe it.
func (f *Features) track(ctx context.Context, name string, target Target, targetErr error, event Event) (err error) {
	started := time.Now()
	defer func() {
		f.observe(ctx, Observation{Operation: "track", Key: name, Err: err, Duration: time.Since(started)})
	}()
	return submitEvent(ctx, name, target, targetErr, event, f.tracker)
}

// submitEvent owns analytics input before invoking an optional tracking capability.
func submitEvent(ctx context.Context, name string, target Target, targetErr error, event Event, tracker Tracker) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if targetErr != nil {
		return targetErr
	}
	if name == "" || !validString(name) {
		return ErrInvalidKey
	}
	owned, err := target.Clone()
	if err != nil {
		return err
	}
	data, err := event.Clone()
	if err != nil {
		return err
	}
	if tracker == nil {
		return ErrUnsupported
	}
	if err := tracker.Track(ctx, name, owned, data); err != nil {
		return safeError(err)
	}
	return nil
}
