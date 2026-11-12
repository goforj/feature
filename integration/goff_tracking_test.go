//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/openfeaturefeature"
	"github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
)

// TestGOFeatureFlagCustomTracking verifies native collector acceptance in both actual evaluation modes.
func TestGOFeatureFlagCustomTracking(t *testing.T) {
	service := relay(t)
	backend, err := url.Parse(service.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []gofeatureflag.EvaluationType{gofeatureflag.EvaluationTypeRemote, gofeatureflag.EvaluationTypeInProcess} {
		t.Run(string(mode), func(t *testing.T) {
			key := namespace() + "purchase"
			proxy := httputil.NewSingleHostReverseProxy(backend)
			var mu sync.Mutex
			var accepted []byte
			front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/data/collector" {
					proxy.ServeHTTP(w, r)
					return
				}
				data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				if err != nil {
					http.Error(w, "fixture body failed", 400)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(data))
				response := httptest.NewRecorder()
				proxy.ServeHTTP(response, r)
				for name, values := range response.Header() {
					w.Header()[name] = values
				}
				w.WriteHeader(response.Code)
				_, _ = w.Write(response.Body.Bytes())
				if response.Code >= 200 && response.Code < 300 {
					mu.Lock()
					accepted = append([]byte(nil), data...)
					mu.Unlock()
				}
			}))
			t.Cleanup(front.Close)
			provider, err := gofeatureflag.NewProvider(gofeatureflag.ProviderOptions{Endpoint: front.URL, APIKey: evaluationKey, EvaluationType: mode, DataCollectorCollectInterval: 50 * time.Millisecond, HTTPClient: &http.Client{Timeout: time.Second}})
			if err != nil {
				t.Fatal(err)
			}
			driver, err := openfeaturefeature.Open(context.Background(), provider)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := driver.Close(); err != nil {
					t.Error(err)
				}
			})
			flags := feature.New(driver).For(feature.Target{Key: "tenant:acme", Attributes: map[string]any{"plan": "pro"}})
			if !feature.New(driver).SupportsTracking() {
				t.Fatal("GO Feature Flag tracking unavailable")
			}
			if err := flags.Track(context.Background(), key, feature.Event{Attributes: map[string]any{"currency": "USD", "amount": 12.5}}); err != nil {
				t.Fatal(err)
			}
			eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return bytes.Contains(accepted, []byte(key)) })
			mu.Lock()
			data := append([]byte(nil), accepted...)
			mu.Unlock()
			var batch struct {
				Events []struct {
					Kind, Key, UserKey   string
					EvaluationContext    map[string]any
					TrackingEventDetails map[string]any
				}
			}
			if err := json.Unmarshal(data, &batch); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, event := range batch.Events {
				if event.Key == key {
					found = true
					if event.Kind != "tracking" || event.UserKey != "tenant:acme" || event.EvaluationContext["plan"] != "pro" || event.TrackingEventDetails["currency"] != "USD" || event.TrackingEventDetails["amount"] != 12.5 {
						t.Fatal("provider tracking payload changed")
					}
				}
			}
			if !found {
				t.Fatal("collector accepted no custom event")
			}
		})
	}
}
