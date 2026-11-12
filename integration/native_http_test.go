//go:build integration

package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/openfeaturefeature"
)

// TestNativeHTTPModelWorkflow combines real HTTP configuration, typed models, SDK use, and webhook delivery.
func TestNativeHTTPModelWorkflow(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	var clock atomic.Int64
	clock.Store(start.Add(-time.Second).UnixNano())
	doc := feature.Document{Schema: 2, Flags: map[string]feature.Flag{
		"theme":    {Variants: map[string]any{"control": "blue", "treatment": "green"}, DefaultVariant: "control", Rules: []feature.Rule{{Conditions: []feature.Condition{{Attribute: "paid", Operator: feature.OpEqual, Value: true}, {Attribute: "seats", Operator: feature.OpGreaterOrEqual, Value: 10}, {Attribute: "/organization/plan", Operator: feature.OpEqual, Value: "pro"}}, Variant: "treatment"}}},
		"launch":   {Default: false, Rules: []feature.Rule{{Start: &start, End: &end, Value: true}}},
		"progress": {Default: false, Rules: []feature.Rule{{Rollout: &feature.Rollout{Salt: "v1", Progressive: &feature.Progressive{Start: start, End: end, From: 0, To: 10000}}, Value: true}}},
		"split":    {Variants: map[string]any{"control": "blue", "treatment": "green"}, DefaultVariant: "control", Rules: []feature.Rule{{Split: &feature.Split{Salt: "v1", Allocations: []feature.Allocation{{Variant: "control", BasisPoints: 5000}, {Variant: "treatment", BasisPoints: 5000}}}}}},
	}}
	encoded, err := feature.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	body := encoded
	bad := false
	notifyFailed := false
	var exports []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet {
			if bad {
				_, _ = io.WriteString(w, `{"schema":2,"flags":`)
				return
			}
			_, _ = w.Write(body)
			return
		}
		if notifyFailed {
			http.Error(w, "private notification failure", 500)
			return
		}
		data, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		exports = append(exports, string(data))
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	source, err := feature.NewHTTPSource(feature.HTTPConfig{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	exporter, err := feature.NewHTTPExporter(feature.ExporterConfig{HTTP: feature.HTTPConfig{URL: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := feature.Open(ctx, source, feature.WithNotifier(exporter), feature.WithEventSink(exporter), feature.WithClock(func() time.Time { return time.Unix(0, clock.Load()).UTC() }))
	if err != nil {
		t.Fatal(err)
	}
	driver, err := openfeaturefeature.Open(ctx, openfeaturefeature.NewProvider(engine))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Error(err)
		}
	})
	target := feature.Target{Key: "tenant:acme", Attributes: map[string]any{"paid": true, "seats": 12, "organization": map[string]any{"plan": "pro"}}}
	flags := feature.New(driver).For(target)
	snapshot := feature.New(engine.Snapshot()).For(target)
	t.Run("typed-conditions", func(t *testing.T) {
		if d := flags.String(ctx, "theme", "fallback"); d.Value != "green" || d.Variant != "treatment" || d.Err != nil {
			t.Fatal(d)
		}
	})
	t.Run("calendar-snapshot", func(t *testing.T) {
		clock.Store(start.UnixNano())
		if !flags.Enabled(ctx, "launch") || snapshot.Enabled(ctx, "launch") {
			t.Fatal("calendar view changed")
		}
		clock.Store(end.UnixNano())
		if flags.Enabled(ctx, "launch") || !flags.Enabled(ctx, "progress") || snapshot.Enabled(ctx, "progress") {
			t.Fatal("progressive endpoint or snapshot changed")
		}
	})
	t.Run("weighted-variants", func(t *testing.T) {
		before := flags.String(ctx, "split", "fallback")
		after := flags.String(ctx, "split", "fallback")
		if before.Err != nil || before != after || before.Variant == "" || before.Reason != feature.ReasonSplit {
			t.Fatal(before, after)
		}
	})
	t.Run("custom-analytics", func(t *testing.T) {
		metric := 12.5
		if err := flags.Track(ctx, "purchase", feature.Event{Value: &metric, Attributes: map[string]any{"currency": "USD"}}); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		found := false
		for _, payload := range exports {
			if strings.Contains(payload, `"name":"purchase"`) {
				found = true
				if strings.Contains(payload, "tenant:acme") || !strings.Contains(payload, `"value":12.5`) {
					t.Fatal("analytics payload lost its metric or exported an identity")
				}
			}
		}
		if !found {
			t.Fatal("native SDK custom analytics did not reach the endpoint")
		}
	})
	t.Run("refresh-and-notification", func(t *testing.T) {
		flag := doc.Flags["theme"]
		flag.Rules[0].Variant = "control"
		doc.Flags["theme"] = flag
		next, err := feature.Encode(doc)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		body = next
		mu.Unlock()
		if err := engine.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
		if flags.String(ctx, "theme", "fallback").Value != "blue" || snapshot.String(ctx, "theme", "fallback").Value != "green" {
			t.Fatal("configuration snapshot changed")
		}
		mu.Lock()
		defer mu.Unlock()
		if !strings.Contains(exports[len(exports)-1], `"type":"change"`) || !strings.Contains(exports[len(exports)-1], `"keys":["theme"]`) {
			t.Fatal("revision notification missing")
		}
	})
	t.Run("last-valid-retention", func(t *testing.T) {
		mu.Lock()
		bad = true
		mu.Unlock()
		before := engine.Status().Revision
		if err := engine.Refresh(ctx); err != feature.ErrInvalidDocument {
			t.Fatal(err)
		}
		if engine.Status().Revision != before || flags.String(ctx, "theme", "fallback").Value != "blue" {
			t.Fatal("malformed HTTP source replaced valid state")
		}
		mu.Lock()
		bad = false
		mu.Unlock()
	})
	t.Run("notification-failure-isolation", func(t *testing.T) {
		flag := doc.Flags["theme"]
		flag.Rules[0].Variant = "treatment"
		doc.Flags["theme"] = flag
		next, err := feature.Encode(doc)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		body = next
		notifyFailed = true
		mu.Unlock()
		if err := engine.Refresh(ctx); err != nil {
			t.Fatal("notification failure rejected publication", err)
		}
		if engine.Status().LastNotifyError != feature.ErrUnavailable || flags.String(ctx, "theme", "fallback").Value != "green" {
			t.Fatal("notification failure changed source health")
		}
	})
}
