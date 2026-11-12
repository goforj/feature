package feature

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// roundTripperFunc controls transport failures while preserving the real request policy.
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip supplies the configured transport response.
func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// errorReader proves incomplete successful responses do not become valid documents.
type errorReader struct{ cancel context.CancelFunc }

// Read returns a transport failure after an optional coordinated cancellation.
func (r errorReader) Read([]byte) (int, error) {
	if r.cancel != nil {
		r.cancel()
	}
	return 0, errors.New("private body error")
}

// Close releases the disposable response body.
func (r errorReader) Close() error { return nil }

// TestHTTPSourceRealRequests verifies headers, strict bounds, last-valid retention, and request locality.
func TestHTTPSourceRealRequests(t *testing.T) {
	var mode atomic.Int32
	var requests atomic.Int32
	var redirectHits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirectHits.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "public-test-key" {
			http.Error(w, "bad auth", 401)
			return
		}
		switch mode.Load() {
		case 1:
			_, _ = io.WriteString(w, `{"broken":`)
		case 2:
			http.Error(w, "private server diagnostics", 500)
		case 3:
			_, _ = io.WriteString(w, strings.Repeat("x", MaxDocumentBytes+1))
		case 4:
			http.Redirect(w, r, destination.URL, http.StatusFound)
		default:
			_, _ = io.WriteString(w, `{"schema":2,"flags":{"theme":{"default":"blue"}}}`)
		}
	}))
	defer server.Close()
	headers := http.Header{"Authorization": []string{"public-test-key"}}
	source, err := NewHTTPSource(HTTPConfig{URL: server.URL, Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	headers.Set("Authorization", "changed")
	ctx := context.Background()
	engine, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	revision := engine.Status().Revision
	for i := 0; i < 10; i++ {
		if New(engine).String(ctx, "theme", "fallback").Value != "blue" {
			t.Fatal("native HTTP-backed evaluation failed")
		}
	}
	if requests.Load() != before {
		t.Fatal("evaluation performed source I/O")
	}
	for _, tc := range []struct {
		mode int32
		want error
	}{{1, ErrInvalidDocument}, {2, ErrSource}, {3, ErrInvalidDocument}, {4, ErrSource}} {
		mode.Store(tc.mode)
		if err := engine.Refresh(ctx); err != tc.want {
			t.Fatal(tc.mode, err)
		}
		if engine.Status().Revision != revision || New(engine).String(ctx, "theme", "f").Value != "blue" {
			t.Fatal("HTTP failure replaced valid snapshot")
		}
	}
	if redirectHits.Load() != 0 {
		t.Fatal("redirect forwarded source credentials")
	}
	mode.Store(0)
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := source.Load(canceled); err != context.Canceled {
		t.Fatal(err)
	}
}

// TestHTTPTransportBoundaries verifies safe configuration, cancellation, body failure, and timeout policy.
func TestHTTPTransportBoundaries(t *testing.T) {
	for _, config := range []HTTPConfig{{URL: "%"}, {URL: "file:///tmp/private"}, {URL: "https://"}, {URL: "https://user:password@example.invalid"}, {URL: "https://example.invalid/#fragment"}, {URL: "https://example.invalid", Timeout: -1}} {
		if _, err := NewHTTPSource(config); err != ErrInvalidConfig {
			t.Fatal(config.URL, err)
		}
	}
	ctx := context.Background()
	for _, tc := range []struct {
		transport roundTripperFunc
		want      error
	}{
		{func(*http.Request) (*http.Response, error) { return nil, errors.New("private transport") }, ErrSource},
		{func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }, context.DeadlineExceeded},
		{func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: errorReader{}}, nil
		}, ErrSource},
	} {
		source, err := NewHTTPSource(HTTPConfig{URL: "http://example.invalid", Transport: tc.transport, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := source.Load(ctx); err != tc.want {
			t.Fatal(err)
		}
	}
	child, cancel := context.WithCancel(ctx)
	source, _ := NewHTTPSource(HTTPConfig{URL: "http://example.invalid", Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { cancel(); return nil, errors.New("private transport") })})
	if _, err := source.Load(child); err != context.Canceled {
		t.Fatal(err)
	}
	child, cancel = context.WithCancel(ctx)
	source, _ = NewHTTPSource(HTTPConfig{URL: "http://example.invalid", Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: errorReader{cancel: cancel}}, nil
	})})
	if _, err := source.Load(child); err != context.Canceled {
		t.Fatal(err)
	}
	endpoint, _ := newHTTPEndpoint(HTTPConfig{URL: "http://example.invalid"})
	if _, err := endpoint.request(ctx, "bad method", nil); err != ErrSource {
		t.Fatal(err)
	}
}

// TestHTTPTimeoutPhases preserves deadline classification before and after response headers.
func TestHTTPTimeoutPhases(t *testing.T) {
	for _, phase := range []string{"before_headers", "after_headers"} {
		t.Run(phase, func(t *testing.T) {
			var stalled atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if !stalled.Load() {
					_, _ = io.WriteString(w, `{"schema":2,"flags":{"theme":{"default":"blue"}}}`)
					return
				}
				if phase == "after_headers" {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			config := HTTPConfig{URL: server.URL, Timeout: 100 * time.Millisecond}
			source, err := NewHTTPSource(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			engine, err := Open(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			revision := engine.Status().Revision
			stalled.Store(true)
			if _, err := source.Load(ctx); err != context.DeadlineExceeded {
				t.Fatalf("source timeout: got %v, want deadline exceeded", err)
			}
			if err := engine.Refresh(ctx); err != ErrSource {
				t.Fatalf("refresh timeout: got %v, want source unavailable", err)
			}
			if engine.Status().Revision != revision || New(engine).String(ctx, "theme", "fallback").Value != "blue" {
				t.Fatal("HTTP timeout replaced valid snapshot")
			}
			exporter, err := NewHTTPExporter(ExporterConfig{HTTP: config})
			if err != nil {
				t.Fatal(err)
			}
			if err := exporter.Track(ctx, "checkout", Target{}, Event{}); err != context.DeadlineExceeded {
				t.Fatalf("exporter timeout: got %v, want deadline exceeded", err)
			}
			if exporter.LastError() != context.DeadlineExceeded {
				t.Fatal("exporter timeout classification was not retained")
			}
		})
	}
}

// TestHTTPExporterPayloads verifies explicit identity export and classified change metadata.
func TestHTTPExporterPayloads(t *testing.T) {
	ctx := context.Background()
	var payloads []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		payloads = append(payloads, string(data))
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invalid delivery request")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	exporter, err := NewHTTPExporter(ExporterConfig{HTTP: HTTPConfig{URL: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	metric := 12.5
	target := Target{Key: "private-identity", Attributes: map[string]any{"email": "private-email"}}
	if err := exporter.Track(ctx, "checkout", target, Event{Value: &metric, Attributes: map[string]any{"plan": "pro"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payloads[0], "private-") || !strings.Contains(payloads[0], "checkout") {
		t.Fatal("default analytics exported target data")
	}
	if err := exporter.Notify(ctx, Change{Revision: "rev", Keys: []string{"checkout"}, Err: errors.New("private source text")}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payloads[1], "private") || !strings.Contains(payloads[1], ErrUnavailable.Error()) {
		t.Fatal("notification exposed diagnostics")
	}
	identified, _ := NewHTTPExporter(ExporterConfig{HTTP: HTTPConfig{URL: server.URL}, IncludeTarget: true})
	if err := identified.Track(ctx, "checkout", target, Event{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payloads[2], `"key":"private-identity"`) {
		t.Fatal("explicit targeting export missing")
	}
	if err := identified.Track(ctx, "checkout", Target{Attributes: map[string]any{"time": time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}, Event{}); err != ErrInvalidValue {
		t.Fatal(err)
	}
	if _, err := NewHTTPExporter(ExporterConfig{}); err != ErrInvalidConfig {
		t.Fatal(err)
	}
	for _, call := range []func() error{
		func() error { return exporter.Track(ctx, "", Target{}, Event{}) },
		func() error {
			return exporter.Track(ctx, "x", Target{Attributes: map[string]any{"bad": func() {}}}, Event{})
		},
		func() error {
			return exporter.Track(ctx, "x", Target{}, Event{Attributes: map[string]any{"bad": func() {}}})
		},
		func() error { return exporter.Notify(ctx, Change{Revision: strings.Repeat("x", MaxStringBytes+1)}) },
		func() error { return exporter.Notify(ctx, Change{Keys: []string{"invalid/key"}}) },
	} {
		if err := call(); err == nil {
			t.Fatal("invalid export accepted")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if exporter.Track(canceled, "x", Target{}, Event{}) != context.Canceled || exporter.Notify(canceled, Change{}) != context.Canceled {
		t.Fatal("export ignored cancellation")
	}
	failed, _ := NewHTTPExporter(ExporterConfig{HTTP: HTTPConfig{URL: "http://example.invalid", Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private delivery error") })}})
	if err := failed.Track(ctx, "x", Target{}, Event{}); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := failed.Notify(canceled, Change{}); err != context.Canceled {
		t.Fatal(err)
	}
	timed, _ := NewHTTPExporter(ExporterConfig{HTTP: HTTPConfig{URL: "http://example.invalid", Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}})
	if err := timed.Track(ctx, "x", Target{}, Event{}); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	failed.Observe(ctx, Observation{Operation: "evaluate", Key: "checkout", Err: errors.New("private provider error")})
	if failed.LastError() != ErrUnavailable {
		t.Fatal("observer delivery failure was hidden")
	}
	exporter.Observe(ctx, Observation{Operation: "evaluate", Key: "checkout", Type: TypeBoolean, Reason: ReasonError, Err: errors.New("private provider error")})
	if exporter.LastError() != nil || strings.Contains(payloads[len(payloads)-1], "private") || !strings.Contains(payloads[len(payloads)-1], `"type":"observation"`) {
		t.Fatal("observation diagnostics leaked or were not delivered")
	}
	exporter.Observe(ctx, Observation{Key: ""})
	if exporter.LastError() != ErrInvalidValue {
		t.Fatal("invalid observation was not classified")
	}
}
