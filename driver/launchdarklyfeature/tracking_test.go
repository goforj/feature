package launchdarklyfeature

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/goforj/feature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// trackingClient records native custom event submission and controlled errors.
type trackingClient struct {
	testClient
	data   ldvalue.Value
	target ldcontext.Context
	metric float64
	err    error
	cancel context.CancelFunc
}

// TrackDataCtx records a native event without changing its configured delivery policy.
func (c *trackingClient) TrackDataCtx(_ context.Context, _ string, target ldcontext.Context, data ldvalue.Value) error {
	c.target = target
	c.data = data
	if c.cancel != nil {
		c.cancel()
	}
	return c.err
}

// TrackMetricCtx records a metric and its custom payload through the same SDK boundary.
func (c *trackingClient) TrackMetricCtx(ctx context.Context, name string, target ldcontext.Context, metric float64, data ldvalue.Value) error {
	c.metric = metric
	return c.TrackDataCtx(ctx, name, target, data)
}

// TestNativeSDKTracking verifies identity conversion, event ownership, and safe failures.
func TestNativeSDKTracking(t *testing.T) {
	ctx := context.Background()
	client := &trackingClient{}
	driver := New(client)
	if !driver.SupportsTracking() || New(&testClient{}).SupportsTracking() {
		t.Fatal("SDK tracking capability misclassified")
	}
	target := feature.Target{Key: "tenant", Kind: "organization"}
	attributes := map[string]any{"currency": "USD"}
	metric := 12.5
	if err := driver.Track(ctx, "checkout", target, feature.Event{Attributes: attributes}); err != nil {
		t.Fatal(err)
	}
	attributes["currency"] = "changed"
	if client.data.GetByKey("currency").StringValue() != "USD" || client.target.Kind() != "organization" {
		t.Fatal("event data was not owned")
	}
	if err := driver.Track(ctx, "checkout", target, feature.Event{Value: &metric}); err != nil || client.metric != metric {
		t.Fatal(err)
	}
	client.err = errors.New("private SDK credentials")
	if err := driver.Track(ctx, "x", target, feature.Event{}); err != feature.ErrUnavailable {
		t.Fatal(err)
	}
	child, cancel := context.WithCancel(ctx)
	client.cancel = cancel
	if err := driver.Track(child, "x", target, feature.Event{}); err != context.Canceled {
		t.Fatal(err)
	}
	if err := driver.Track(child, "x", target, feature.Event{}); err != context.Canceled {
		t.Fatal(err)
	}
	if err := New(&testClient{}).Track(ctx, "x", target, feature.Event{}); err != feature.ErrUnsupported {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		target feature.Target
		event  feature.Event
		want   error
	}{
		{"", target, feature.Event{}, feature.ErrInvalidKey},
		{"x", feature.Target{}, feature.Event{}, feature.ErrInvalidSubject},
		{"x", feature.Target{Key: strings.Repeat("x", feature.MaxStringBytes+1)}, feature.Event{}, feature.ErrInvalidSubject},
		{"x", target, feature.Event{Attributes: map[string]any{"bad": func() {}}}, feature.ErrInvalidValue},
	} {
		if err := driver.Track(ctx, tc.name, tc.target, tc.event); err != tc.want {
			t.Fatal(err)
		}
	}
	bad := math.Inf(1)
	if err := driver.Track(ctx, "x", target, feature.Event{Value: &bad}); err != feature.ErrInvalidValue {
		t.Fatal(err)
	}
	_ = driver.Close()
	if err := driver.Track(ctx, "x", target, feature.Event{}); err != feature.ErrClosed {
		t.Fatal(err)
	}
}
