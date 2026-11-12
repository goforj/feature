//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/goforj/feature"
)

// TestLaunchDarklyCustomTracking proves actual custom events and metrics cross the production Relay.
func TestLaunchDarklyCustomTracking(t *testing.T) {
	service := launchDarklyRelay(t)
	flags, sdk := relayAdapter(t, service, "direct")
	prefix := namespace()
	secret := prefix + "@example.invalid"
	service.mu.Lock()
	service.privateLeaks[secret] = false
	service.mu.Unlock()
	scope := flags.For(feature.Target{Key: prefix + "user", Attributes: map[string]any{"email": secret, "privateAttributes": []string{"email"}}})
	ctx := context.Background()
	metric := 12.5
	if !flags.SupportsTracking() {
		t.Fatal("native SDK tracking unavailable")
	}
	if err := scope.Track(ctx, prefix+"purchase", feature.Event{Attributes: map[string]any{"currency": "USD"}}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Track(ctx, prefix+"revenue", feature.Event{Value: &metric, Attributes: map[string]any{"currency": "USD"}}); err != nil {
		t.Fatal(err)
	}
	if !sdk.FlushAndWait(2 * time.Second) {
		t.Fatal("native SDK custom event flush timed out")
	}
	eventually(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		_, purchase := service.customEvents[prefix+"purchase"]
		_, revenue := service.customEvents[prefix+"revenue"]
		return purchase && revenue
	})
	service.mu.Lock()
	purchase, revenue, leaked := service.customEvents[prefix+"purchase"], service.customEvents[prefix+"revenue"], service.privateLeaks[secret]
	service.mu.Unlock()
	if purchase.MetricValue != nil || revenue.MetricValue == nil || *revenue.MetricValue != metric || leaked {
		t.Fatal("custom event type, metric, or privacy changed")
	}
	var data struct{ Currency string }
	if err := json.Unmarshal(revenue.Data, &data); err != nil || data.Currency != "USD" {
		t.Fatal("custom event data missing", err)
	}
}

// TestOpenFeatureLaunchDarklyTrackingCapability records the pinned community provider's actual limitation.
func TestOpenFeatureLaunchDarklyTrackingCapability(t *testing.T) {
	service := launchDarklyRelay(t)
	flags, _ := relayAdapter(t, service, "openfeature")
	if flags.SupportsTracking() {
		t.Fatal("provider without Tracker advertised custom event support")
	}
	if err := flags.For(feature.Target{Key: "tenant:acme"}).Track(context.Background(), "purchase", feature.Event{}); err != feature.ErrUnsupported {
		t.Fatal(err)
	}
}
