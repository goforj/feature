package openfeaturefeature

import (
	"context"
	"unicode/utf8"

	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
)

// eventClient preserves SDK-managed context merging and configured tracking behavior.
type eventClient interface {
	Track(context.Context, string, openfeature.EvaluationContext, openfeature.TrackingEventDetails)
}

// Track submits to the SDK; its provider decides how events are processed or delivered.
// OpenFeature's Track has no error result, so this reports submission only.
func (d *Driver) Track(ctx context.Context, name string, target feature.Target, event feature.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" || len(name) > feature.MaxStringBytes || !utf8.ValidString(name) {
		return feature.ErrInvalidKey
	}
	if d.closed.Load() {
		return feature.ErrClosed
	}
	if d.trackingUnavailable {
		return feature.ErrUnsupported
	}
	native, err := evaluationContext(target)
	if err != nil {
		return err
	}
	ownedEvent, err := event.Clone()
	if err != nil {
		return feature.ErrInvalidValue
	}
	metric := float64(0)
	if ownedEvent.Value != nil {
		metric = *ownedEvent.Value
	}
	details := openfeature.NewTrackingEventDetails(metric)
	for key, value := range ownedEvent.Attributes {
		details = details.Add(key, value)
	}
	client, ok := d.client.(eventClient)
	if !ok {
		return feature.ErrUnsupported
	}
	client.Track(ctx, name, native, details)
	return nil
}

// SupportsTracking reports known provider support or borrowed SDK submission capability.
func (d *Driver) SupportsTracking() bool {
	_, supported := d.client.(eventClient)
	return supported && !d.trackingUnavailable
}

// SupportsTracking reports the evaluator's configured event-sink capability.
func (p *Provider) SupportsTracking() bool { return p.features.SupportsTracking() }

// Track forwards SDK custom events to the evaluator's optional tracking capability.
func (p *Provider) Track(ctx context.Context, name string, target openfeature.EvaluationContext, event openfeature.TrackingEventDetails) {
	if p.closed.Load() {
		return
	}
	metric := event.Value()
	flat := openfeature.FlattenedContext(target.Attributes())
	if target.TargetingKey() != "" {
		flat[openfeature.TargetingKey] = target.TargetingKey()
	}
	owned, err := providerTarget(flat)
	if err != nil {
		return
	}
	_ = p.features.For(owned).Track(ctx, name, feature.Event{Value: &metric, Attributes: event.Attributes()})
}
