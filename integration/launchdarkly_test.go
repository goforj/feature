//go:build integration

package integration

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/launchdarklyfeature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
)

// TestLaunchDarklyStreamingAndEvents exercises the official SDK's actual streaming and analytics transports.
func TestLaunchDarklyStreamingAndEvents(t *testing.T) {
	key := namespace() + "stream"
	flag := ldbuilders.NewFlagBuilder(key).Version(1).On(true).
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).FallthroughVariation(0).OffVariation(1).TrackEvents(true).Build()
	data := ldservices.NewServerSDKData().Flags(&flag)
	handler, stream := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	var streams, active, events atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != evaluationKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/all" {
			streams.Add(1)
			active.Add(1)
			defer active.Add(-1)
			handler.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/bulk" {
			var body io.Reader = io.LimitReader(r.Body, 1<<20)
			if r.Header.Get("Content-Encoding") == "gzip" {
				reader, err := gzip.NewReader(body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer reader.Close()
				body = io.LimitReader(reader, 1<<20)
			}
			var batch []map[string]any
			if err := json.NewDecoder(body).Decode(&batch); err != nil {
				t.Error(err)
			}
			for _, event := range batch {
				if event["kind"] == "feature" && event["key"] == key {
					events.Add(1)
				}
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	defer stream.Close()
	driver, err := launchdarklyfeature.Open(context.Background(), launchdarklyfeature.Config{
		SDKKey: evaluationKey, InitTimeout: 2 * time.Second,
		SDKConfig: ldclient.Config{
			Logging: ldcomponents.NoLogging(), ServiceEndpoints: ldcomponents.RelayProxyEndpoints(server.URL),
			DataSource: ldcomponents.StreamingDataSource().InitialReconnectDelay(10 * time.Millisecond),
			Events:     ldcomponents.SendEvents().FlushInterval(time.Hour),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	scope := feature.New(driver).For(feature.Target{Key: "tenant:acme", Attributes: map[string]any{"privateAttributes": []string{"email"}, "email": "example@example.invalid"}})
	if d := scope.Evaluate(context.Background(), key, true); d.Value || d.Err != nil || d.Variant != "0" {
		t.Fatal(d)
	}
	flag.Version = 2
	flag.On = false
	updated := ldservices.NewServerSDKData().Flags(&flag)
	stream.Enqueue(updated.ToPutEvent())
	eventually(t, func() bool {
		d := scope.Evaluate(context.Background(), key, false)
		return d.Value && d.Err == nil && d.Reason == feature.ReasonDisabled && d.Variant == "1"
	})
	// A broken stream must retain the last valid native SDK configuration.
	stream.Close()
	eventually(t, func() bool { return active.Load() == 0 })
	if d := scope.Evaluate(context.Background(), key, false); !d.Value || d.Err != nil {
		t.Fatal("stream interruption lost the valid off-variation", d)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if streams.Load() == 0 || events.Load() == 0 {
		t.Fatalf("native streaming or analytics transport was bypassed: streams=%d events=%d", streams.Load(), events.Load())
	}
	if d := scope.Evaluate(context.Background(), key, true); !d.Value || d.Err != feature.ErrClosed {
		t.Fatal(d)
	}
}

// TestLaunchDarklyStartupFailures verifies real HTTP failures and cancellation release SDK workers.
func TestLaunchDarklyStartupFailures(t *testing.T) {
	for _, name := range []string{"unauthorized", "malformed", "canceled"} {
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case entered <- struct{}{}:
				default:
				}
				switch name {
				case "unauthorized":
					http.Error(w, "private diagnostics", http.StatusUnauthorized)
				case "malformed":
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "event: put\ndata: invalid\n\n")
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				default:
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				driver, err := launchdarklyfeature.Open(ctx, launchdarklyfeature.Config{SDKKey: evaluationKey, InitTimeout: 250 * time.Millisecond, SDKConfig: ldclient.Config{
					Logging: ldcomponents.NoLogging(), Events: ldcomponents.NoEvents(),
					ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: server.URL, Polling: server.URL, Events: server.URL},
				}})
				if err == nil {
					_ = driver.Close()
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("SDK did not contact local endpoint")
			}
			want := context.DeadlineExceeded
			if name == "canceled" {
				cancel()
				want = context.Canceled
			}
			select {
			case err := <-done:
				if err != want {
					t.Fatalf("got %v, want %v", err, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("SDK initialization did not terminate")
			}
		})
	}
}

// TestLaunchDarklyBorrowedOwnership verifies a closed adapter leaves the caller's real SDK usable.
func TestLaunchDarklyBorrowedOwnership(t *testing.T) {
	data := ldservices.NewServerSDKData()
	flag := ldbuilders.NewFlagBuilder("owned").On(true).Variations(ldvalue.Bool(true)).FallthroughVariation(0).Build()
	data.Flags(&flag)
	handler, stream := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	server := httptest.NewServer(handler)
	defer server.Close()
	defer stream.Close()
	sdk, err := ldclient.MakeCustomClient(evaluationKey, ldclient.Config{Logging: ldcomponents.NoLogging(), Events: ldcomponents.NoEvents(), ServiceEndpoints: ldcomponents.RelayProxyEndpoints(server.URL)}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	driver := launchdarklyfeature.New(sdk)
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if value, err := sdk.BoolVariation("owned", ldcontext.New("tenant:acme"), false); !value || err != nil {
		t.Fatal("borrowed SDK was closed", value, err)
	}
}
