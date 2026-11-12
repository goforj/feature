//go:build integration

package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/openfeaturefeature"
	"github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
)

// openRelay owns a real SDK provider while allowing native authentication and transport policy.
func openRelay(t *testing.T, endpoint, key string, mode gofeatureflag.EvaluationType) *openfeaturefeature.Driver {
	t.Helper()
	provider, err := gofeatureflag.NewProvider(gofeatureflag.ProviderOptions{
		Endpoint: endpoint, APIKey: key, EvaluationType: mode,
		FlagChangePollingInterval: 100 * time.Millisecond, DataCollectorDisabled: true,
		DisableCache: true, HTTPClient: &http.Client{Timeout: time.Second},
	})
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
	return driver
}

// TestRelayAuthentication verifies rejection by the actual service without exposing server prose.
func TestRelayAuthentication(t *testing.T) {
	service := relay(t)
	driver := openRelay(t, service.endpoint, "incorrect-public-test-key", gofeatureflag.EvaluationTypeRemote)
	decision := feature.New(driver).For(feature.Target{Key: "tenant:acme"}).Evaluate(context.Background(), "bootstrap", true)
	if !decision.Value || decision.Err != feature.ErrUnavailable {
		t.Fatal(decision)
	}
}

// TestRelayTransportFailures keeps the real service behind a controllable network boundary.
func TestRelayTransportFailures(t *testing.T) {
	service := relay(t)
	key := namespace() + "network"
	service.update(t, map[string]any{key: remoteFlag(true, "")})
	backend, err := url.Parse(service.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(backend)
	var mode atomic.Int32
	var rejected atomic.Int64
	entered := make(chan struct{}, 1)
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case 1:
			rejected.Add(1)
			http.Error(w, "private upstream diagnostics", http.StatusServiceUnavailable)
		case 2:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"private":"malformed evaluation"`))
		case 3:
			if !strings.Contains(r.URL.Path, "/evaluate/") {
				proxy.ServeHTTP(w, r)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case entered <- struct{}{}:
			default:
			}
			<-r.Context().Done()
		default:
			proxy.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(front.Close)
	remote := openRelay(t, front.URL, evaluationKey, gofeatureflag.EvaluationTypeRemote)
	scope := feature.New(remote).For(feature.Target{Key: "tenant:acme"})
	eventually(t, func() bool { return scope.Enabled(context.Background(), key) })
	inprocess := openRelay(t, front.URL, evaluationKey, gofeatureflag.EvaluationTypeInProcess)
	local := feature.New(inprocess).For(feature.Target{Key: "tenant:acme"})
	if !local.Enabled(context.Background(), key) {
		t.Fatal("in-process SDK did not initialize its configuration")
	}
	t.Run("outage-and-last-valid", func(t *testing.T) {
		mode.Store(1)
		defer mode.Store(0)
		before := rejected.Load()
		d := scope.Evaluate(context.Background(), key, false)
		if d.Value || d.Err != feature.ErrUnavailable || strings.Contains(d.Err.Error(), "private") {
			t.Fatal(d)
		}
		// A second rejection proves the real in-process poll reached the failed boundary.
		eventually(t, func() bool { return rejected.Load() >= before+2 })
		if d := local.Evaluate(context.Background(), key, false); !d.Value || d.Err != nil {
			t.Fatal("last valid SDK configuration lost", d)
		}
	})
	t.Run("malformed-success", func(t *testing.T) {
		mode.Store(2)
		defer mode.Store(0)
		d := scope.Evaluate(context.Background(), key, true)
		if !d.Value || d.Err != feature.ErrUnavailable {
			t.Fatal(d)
		}
	})
	t.Run("cancel-during-response", func(t *testing.T) {
		mode.Store(3)
		defer mode.Store(0)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan feature.Decision, 1)
		go func() { done <- scope.Evaluate(ctx, key, true) }()
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("SDK never reached the transport")
		}
		cancel()
		select {
		case d := <-done:
			if !d.Value || d.Err != context.Canceled {
				t.Fatal(d)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("evaluation ignored cancellation")
		}
	})
	t.Run("recover-and-update", func(t *testing.T) {
		service.update(t, map[string]any{key: remoteFlag(false, "")})
		eventually(t, func() bool {
			a := scope.Evaluate(context.Background(), key, true)
			b := local.Evaluate(context.Background(), key, true)
			return !a.Value && a.Err == nil && !b.Value && b.Err == nil
		})
	})
	t.Run("owned-close", func(t *testing.T) {
		if err := inprocess.Close(); err != nil {
			t.Fatal(err)
		}
		if err := inprocess.Close(); err != nil {
			t.Fatal(err)
		}
		if d := local.Evaluate(context.Background(), key, true); !d.Value || d.Err != feature.ErrClosed {
			t.Fatal(d)
		}
	})
}

// TestRelayTypedTargeting proves booleans and numbers retain their types across SDK and wire formats.
func TestRelayTypedTargeting(t *testing.T) {
	service := relay(t)
	key := namespace() + "typed"
	service.update(t, map[string]any{key: remoteFlag(false, `paid eq true and seats ge 10`)})
	for _, mode := range []gofeatureflag.EvaluationType{gofeatureflag.EvaluationTypeRemote, gofeatureflag.EvaluationTypeInProcess} {
		t.Run(string(mode), func(t *testing.T) {
			driver := openRelay(t, service.endpoint, evaluationKey, mode)
			flags := feature.New(driver)
			match := flags.For(feature.Target{Key: "tenant:acme", Attributes: map[string]any{"paid": true, "seats": 12}})
			eventually(t, func() bool { return match.Enabled(context.Background(), key) })
			miss := flags.For(feature.Target{Key: "tenant:acme", Attributes: map[string]any{"paid": false, "seats": 12}})
			if miss.Enabled(context.Background(), key) {
				t.Fatal("typed rule accepted a nonmatching target")
			}
		})
	}
}
