package launchdarklyfeature

import (
	"context"
	"unicode/utf8"

	"github.com/goforj/feature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// eventClient keeps native tracking optional for custom borrowed clients.
type eventClient interface {
	TrackDataCtx(context.Context, string, ldcontext.Context, ldvalue.Value) error
	TrackMetricCtx(context.Context, string, ldcontext.Context, float64, ldvalue.Value) error
}

// SupportsTracking reports native SDK submission capability without claiming remote delivery.
func (d *Driver) SupportsTracking() bool { _, supported := d.client.(eventClient); return supported }

// Track submits events through the native SDK's hooks, privacy, and delivery policy.
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
	owned, err := target.Clone()
	if err != nil {
		return err
	}
	native, err := toContext(owned)
	if err != nil {
		return err
	}
	ownedEvent, err := event.Clone()
	if err != nil {
		return feature.ErrInvalidValue
	}
	client, ok := d.client.(eventClient)
	if !ok {
		return feature.ErrUnsupported
	}
	if ownedEvent.Value == nil {
		err = client.TrackDataCtx(ctx, name, native, ldvalue.CopyArbitraryValue(ownedEvent.Attributes))
	} else {
		metric := *ownedEvent.Value
		err = client.TrackMetricCtx(ctx, name, native, metric, ldvalue.CopyArbitraryValue(ownedEvent.Attributes))
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return feature.ErrUnavailable
	}
	return nil
}
