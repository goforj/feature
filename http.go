package feature

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPConfig configures bounded HTTP retrieval or event delivery.
// Transport is borrowed and defaults to http.DefaultTransport. Headers are owned
// copies. Timeout defaults to five seconds. Redirects are never followed.
type HTTPConfig struct {
	URL       string
	Headers   http.Header
	Transport http.RoundTripper
	Timeout   time.Duration
}

// httpEndpoint owns request policy without closing the borrowed transport.
type httpEndpoint struct {
	url     string
	headers http.Header
	client  *http.Client
}

// newHTTPEndpoint validates configuration without making a network request.
func newHTTPEndpoint(config HTTPConfig) (*httpEndpoint, error) {
	endpoint, err := url.Parse(config.URL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" || config.Timeout < 0 {
		return nil, ErrInvalidConfig
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	transport := config.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	headers := config.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	return &httpEndpoint{url: config.URL, headers: headers, client: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// request bounds response bytes and strips all transport, URL, and response diagnostics.
func (e *httpEndpoint) request(ctx context.Context, method string, body []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, e.url, bytes.NewReader(body))
	if err != nil {
		return nil, ErrSource
	}
	request.Header = e.headers.Clone()
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := e.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, ErrSource
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrSource
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxDocumentBytes+1))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, ErrSource
	}
	if len(data) > MaxDocumentBytes {
		return nil, ErrInvalidDocument
	}
	return data, nil
}

// HTTPSource retrieves complete native documents without changing evaluation locality.
type HTTPSource struct{ endpoint *httpEndpoint }

// NewHTTPSource captures explicit request policy without I/O or background workers.
func NewHTTPSource(config HTTPConfig) (*HTTPSource, error) {
	endpoint, err := newHTTPEndpoint(config)
	if err != nil {
		return nil, err
	}
	return &HTTPSource{endpoint: endpoint}, nil
}

// Load retrieves a bounded complete document and retains strict native decoding.
func (s *HTTPSource) Load(ctx context.Context) (Document, error) {
	data, err := s.endpoint.request(ctx, http.MethodGet, nil)
	if err != nil {
		return Document{}, err
	}
	return Decode(data)
}

// SourceFunc connects storage services and custom configuration models to the native engine.
type SourceFunc func(context.Context) (Document, error)

// Load delegates complete-document retrieval to the application-owned source.
func (f SourceFunc) Load(ctx context.Context) (Document, error) { return f(ctx) }

// HTTPExporter submits custom events and revision notifications to an explicit JSON endpoint.
// It starts no workers and includes targeting only when IncludeTarget was explicitly selected.
type HTTPExporter struct {
	endpoint      *httpEndpoint
	includeTarget bool
	mu            sync.Mutex
	lastError     error
}

// ExporterConfig controls HTTP delivery and optional identity export.
type ExporterConfig struct {
	HTTP          HTTPConfig
	IncludeTarget bool
}

// NewHTTPExporter captures bounded delivery policy for analytics and change webhooks.
func NewHTTPExporter(config ExporterConfig) (*HTTPExporter, error) {
	endpoint, err := newHTTPEndpoint(config.HTTP)
	if err != nil {
		return nil, err
	}
	return &HTTPExporter{endpoint: endpoint, includeTarget: config.IncludeTarget}, nil
}

// Track sends one owned custom event; success means the endpoint accepted the request.
func (e *HTTPExporter) Track(ctx context.Context, name string, target Target, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" || !validString(name) {
		return ErrInvalidKey
	}
	owned, err := target.Clone()
	if err != nil {
		return err
	}
	data, err := event.Clone()
	if err != nil {
		return err
	}
	payload := struct {
		Type   string  `json:"type"`
		Name   string  `json:"name"`
		Event  Event   `json:"event"`
		Target *Target `json:"target,omitempty"`
	}{Type: "track", Name: name, Event: data}
	if e.includeTarget {
		payload.Target = &owned
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ErrInvalidValue
	}
	return e.deliver(ctx, encoded)
}

// Notify sends revision metadata and safe failure classification without flag values.
func (e *HTTPExporter) Notify(ctx context.Context, change Change) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validString(change.Revision) || !validString(change.PreviousRevision) || len(change.Keys) > MaxFlags {
		return ErrInvalidValue
	}
	for _, key := range change.Keys {
		if !validKey(key) {
			return ErrInvalidKey
		}
	}
	payload := struct {
		Type   string `json:"type"`
		Change Change `json:"change"`
		Error  string `json:"error,omitempty"`
	}{Type: "change", Change: change}
	if change.Err != nil {
		payload.Error = safeError(change.Err).Error()
	}
	encoded, _ := json.Marshal(payload)
	return e.deliver(ctx, encoded)
}

// deliver translates endpoint failures into tracking errors without retaining response text.
func (e *HTTPExporter) deliver(ctx context.Context, data []byte) (result error) {
	defer func() { e.recordError(result) }()
	_, err := e.endpoint.request(ctx, http.MethodPost, data)
	if err != nil {
		if err == ErrSource || err == ErrInvalidDocument {
			return ErrUnavailable
		}
		return safeError(err)
	}
	return nil
}

// LastError reports the most recent delivery or observation validation result.
func (e *HTTPExporter) LastError() error { e.mu.Lock(); defer e.mu.Unlock(); return e.lastError }

// recordError keeps synchronous observer failures visible without changing a flag decision.
func (e *HTTPExporter) recordError(err error) { e.mu.Lock(); e.lastError = err; e.mu.Unlock() }

// Observe exports bounded operation metadata without values or targeting identities.
// Delivery is synchronous and bounded by HTTPConfig.Timeout; use an application
// queue or ObserverFunc when the evaluation path must not wait for network I/O.
func (e *HTTPExporter) Observe(ctx context.Context, event Observation) {
	if event.Key == "" || !validString(event.Key) || !validString(event.Operation) || !validString(string(event.Type)) || !validString(string(event.Reason)) || !validString(event.Revision) || !validString(event.Variant) || event.Duration < 0 {
		e.recordError(ErrInvalidValue)
		return
	}
	payload := struct {
		Type        string      `json:"type"`
		Observation Observation `json:"observation"`
		Error       string      `json:"error,omitempty"`
	}{Type: "observation", Observation: event}
	if event.Err != nil {
		payload.Error = safeError(event.Err).Error()
	}
	data, _ := json.Marshal(payload)
	_ = e.deliver(ctx, data)
}
