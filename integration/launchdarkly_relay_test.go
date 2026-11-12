//go:build integration

package integration

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/launchdarklyfeature"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	launchDarklyRelayImage   = "launchdarkly/ld-relay:9.0.0@sha256:0ea7bd6f3c31f0fb3b406111dc1b7e2b6d8c4bcb798cefd16946ea6f5e53b901"
	launchDarklyRelayVersion = "9.0.0"
)

// launchDarklyFixture serves native SDK traffic through one official Relay container per package run.
type launchDarklyFixture struct {
	container testcontainers.Container
	upstream  *httptest.Server
	endpoint  string
	stop      chan struct{}
	err       error
	mu        sync.Mutex
	flags     map[string]ldmodel.FeatureFlag
	listeners map[chan struct{}]bool
	events    map[string]int
	version   int
	streams   atomic.Int64
}

var launchDarklyPool = struct {
	once    sync.Once
	fixture launchDarklyFixture
}{}

// launchDarklyRelay starts the vendor image with every upstream endpoint redirected to public test data.
func launchDarklyRelay(t *testing.T) *launchDarklyFixture {
	t.Helper()
	launchDarklyPool.once.Do(func() {
		fixture := &launchDarklyPool.fixture
		fixture.stop = make(chan struct{})
		fixture.flags = make(map[string]ldmodel.FeatureFlag)
		fixture.listeners = make(map[chan struct{}]bool)
		fixture.events = make(map[string]int)
		fixture.publish(relayFlag("bootstrap", false).Build())
		fixture.upstream = httptest.NewServer(http.HandlerFunc(fixture.serve))
		port := fixture.upstream.Listener.Addr().(*net.TCPAddr).Port
		upstream := fmt.Sprintf("http://host.testcontainers.internal:%d", port)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{
			Image: launchDarklyRelayImage, ExposedPorts: []string{"8030/tcp"}, HostAccessPorts: []int{port},
			Env: map[string]string{
				"LD_ENV_test": evaluationKey, "STREAM_URI": upstream, "BASE_URI": upstream,
				"CLIENT_SIDE_BASE_URI": upstream, "EVENTS_HOST": upstream,
				"USE_EVENTS": "true", "EVENTS_FLUSH_INTERVAL": "100ms", "LOG_LEVEL": "warn",
				// Host forwarding starts after readiness; the vendor SDK reconnects after this initial wait.
				"INIT_TIMEOUT": "100ms", "GRACEFUL_SHUTDOWN_TIMEOUT": "1s",
			},
			WaitingFor: wait.ForHTTP("/status").WithPort("8030/tcp").WithStartupTimeout(time.Minute),
		}, Started: true})
		fixture.container, fixture.err = container, err
		if err != nil {
			if container != nil {
				if logs, logErr := container.Logs(ctx); logErr == nil {
					data, _ := io.ReadAll(io.LimitReader(logs, 16<<10))
					_ = logs.Close()
					fmt.Println(string(data))
				}
			}
			return
		}
		host, err := container.Host(ctx)
		if err != nil {
			fixture.err = err
			return
		}
		mapped, err := container.MappedPort(ctx, "8030/tcp")
		if err != nil {
			fixture.err = err
			return
		}
		fixture.endpoint = "http://" + net.JoinHostPort(host, mapped.Port())
	})
	fixture := &launchDarklyPool.fixture
	if fixture.err != nil {
		t.Fatalf("required LaunchDarkly Relay unavailable: %v", fixture.err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	eventually(t, func() bool {
		response, err := client.Get(fixture.endpoint + "/status")
		if err != nil {
			return false
		}
		defer response.Body.Close()
		var status struct{ Version, Status string }
		if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&status); err != nil {
			return false
		}
		if status.Version != launchDarklyRelayVersion {
			t.Fatalf("running Relay version %q differs from pin %s", status.Version, launchDarklyRelayVersion)
		}
		return response.StatusCode == http.StatusOK && status.Status == "healthy" && fixture.streams.Load() > 0
	})
	return fixture
}

// relayFlag uses native LaunchDarkly models so the container and SDK retain rule evaluation semantics.
func relayFlag(key string, value bool) *ldbuilders.FlagBuilder {
	variation := 0
	if value {
		variation = 1
	}
	return ldbuilders.NewFlagBuilder(key).On(true).Variations(ldvalue.Bool(false), ldvalue.Bool(true)).FallthroughVariation(variation).OffVariation(0)
}

// publish atomically advances complete native data and notifies every connected Relay upstream stream.
func (f *launchDarklyFixture) publish(flags ...ldmodel.FeatureFlag) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version++
	for _, flag := range flags {
		flag.Version = f.version
		f.flags[flag.Key] = flag
	}
	for listener := range f.listeners {
		select {
		case listener <- struct{}{}:
		default:
		}
	}
}

// snapshot returns immutable native wire data and a monotonic FDv2 selector for reconnects.
func (f *launchDarklyFixture) snapshot() (*ldservicesv2.ServerSDKData, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	flags := make([]ldmodel.FeatureFlag, 0, len(f.flags))
	for _, flag := range f.flags {
		flags = append(flags, flag)
	}
	return ldservicesv2.NewServerSDKData().Flags(flags...), f.version
}

// serve implements only the upstream protocol fixtures; application SDKs always connect to the real Relay.
func (f *launchDarklyFixture) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != evaluationKey {
		http.Error(w, "unauthorized test credential", http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/sdk/poll":
		data, version := f.snapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data.ToInitializerPayload(subsystems.NewSelector("integration", version)))
	case "/sdk/stream":
		f.serveStream(w, r)
	case "/bulk", "/diagnostic":
		f.acceptEvents(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveStream replays current data on reconnect and broadcasts complete FDv2 transfers for live updates.
func (f *launchDarklyFixture) serveStream(w http.ResponseWriter, r *http.Request) {
	changed := make(chan struct{}, 1)
	f.mu.Lock()
	f.listeners[changed] = true
	f.mu.Unlock()
	defer func() { f.mu.Lock(); delete(f.listeners, changed); f.mu.Unlock() }()
	f.streams.Add(1)
	w.Header().Set("Content-Type", "text/event-stream")
	for {
		data, version := f.snapshot()
		protocol := ldservicesv2.NewStreamingProtocol().WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "integration", Target: version, Code: subsystems.IntentTransferFull, Reason: "integration-fixture",
		}}).WithPutObjects(data.ToPutObjects()).WithTransferred("integration", version)
		for _, event := range protocol.SSEEvents() {
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Event(), event.Data()); err != nil {
				return
			}
		}
		w.(http.Flusher).Flush()
		select {
		case <-changed:
		case <-r.Context().Done():
			return
		case <-f.stop:
			return
		}
	}
}

// acceptEvents verifies analytics pass through the container to a bounded local collector.
func (f *launchDarklyFixture) acceptEvents(w http.ResponseWriter, r *http.Request) {
	var body io.Reader = io.LimitReader(r.Body, 1<<20)
	if r.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer reader.Close()
		body = io.LimitReader(reader, 1<<20)
	}
	if r.URL.Path == "/bulk" {
		var events []struct{ Kind, Key string }
		if err := json.NewDecoder(body).Decode(&events); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		for _, event := range events {
			if event.Kind == "feature" {
				f.events[event.Key]++
			}
		}
		f.mu.Unlock()
	} else {
		_, _ = io.Copy(io.Discard, body)
	}
	w.WriteHeader(http.StatusAccepted)
}

// sdkConfig directs every application SDK service endpoint to the vendor container.
func (f *launchDarklyFixture) sdkConfig() ldclient.Config {
	return ldclient.Config{
		ServiceEndpoints: ldcomponents.RelayProxyEndpoints(f.endpoint), Logging: ldcomponents.NoLogging(),
		Events: ldcomponents.NoEvents(), DataSource: ldcomponents.StreamingDataSource().InitialReconnectDelay(10 * time.Millisecond),
	}
}

// TestLaunchDarklyRelay verifies native off values, analytics, authentication, and real container interruption.
func TestLaunchDarklyRelay(t *testing.T) {
	service := launchDarklyRelay(t)
	key := namespace() + "relay"
	service.publish(relayFlag(key, false).TrackEvents(true).Build())
	config := service.sdkConfig()
	config.Events = ldcomponents.SendEvents().FlushInterval(time.Hour)
	driver, err := launchdarklyfeature.Open(context.Background(), launchdarklyfeature.Config{SDKKey: evaluationKey, SDKConfig: config})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Error(err)
		}
	})
	flags := feature.New(driver).For(feature.Target{Key: "tenant:acme"})
	eventually(t, func() bool { d := flags.Evaluate(context.Background(), key, true); return !d.Value && d.Err == nil })
	t.Run("configured-off-variation", func(t *testing.T) {
		service.publish(relayFlag(key, false).On(false).OffVariation(1).TrackEvents(true).Build())
		eventually(t, func() bool {
			d := flags.Evaluate(context.Background(), key, false)
			return d.Value && d.Err == nil && d.Reason == feature.ReasonDisabled && d.Variant == "1"
		})
	})
	t.Run("authentication", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodGet, service.endpoint+"/all", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "incorrect-public-test-key")
		response, err := (&http.Client{Timeout: time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("credential rejection status: got %d, want 401", response.StatusCode)
		}
		rejected, err := launchdarklyfeature.Open(context.Background(), launchdarklyfeature.Config{
			SDKKey: "incorrect-public-test-key", SDKConfig: service.sdkConfig(), InitTimeout: 250 * time.Millisecond,
		})
		if rejected != nil {
			t.Cleanup(func() { _ = rejected.Close() })
		}
		if rejected != nil || err != context.DeadlineExceeded {
			t.Fatalf("SDK authenticated with rejected key: driver=%v error=%v", rejected, err)
		}
	})
	t.Run("container-outage-and-recovery", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		docker, err := testcontainers.NewDockerClientWithOpts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer docker.Close()
		if err := docker.ContainerPause(ctx, service.container.GetContainerID()); err != nil {
			t.Fatal(err)
		}
		paused := true
		defer func() {
			if paused {
				cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := docker.ContainerUnpause(cleanup, service.container.GetContainerID()); err != nil {
					t.Error(err)
				}
			}
		}()
		service.publish(relayFlag(key, false).TrackEvents(true).Build())
		if d := flags.Evaluate(context.Background(), key, false); !d.Value || d.Err != nil {
			t.Fatal("SDK lost valid cached data during outage", d)
		}
		if err := docker.ContainerUnpause(ctx, service.container.GetContainerID()); err != nil {
			t.Fatal(err)
		}
		paused = false
		eventually(t, func() bool { d := flags.Evaluate(context.Background(), key, true); return !d.Value && d.Err == nil })
	})
	t.Run("analytics-on-close", func(t *testing.T) {
		service.mu.Lock()
		before := service.events[key]
		service.mu.Unlock()
		if before != 0 {
			t.Fatal("analytics were already delivered before the owned close")
		}
		if err := driver.Close(); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() bool { service.mu.Lock(); defer service.mu.Unlock(); return service.events[key] > 0 })
		if d := flags.Evaluate(context.Background(), key, true); !d.Value || d.Err != feature.ErrClosed {
			t.Fatal(d)
		}
	})
}
