// Command http demonstrates native HTTP configuration and explicit webhook delivery using a local endpoint.
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/goforj/feature"
)

// main uses a disposable endpoint so the complete program runs without a hosted account.
func main() {
	ctx := context.Background()
	var accepted atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"schema":2,"flags":{"theme":{"default":"blue"}}}`))
			return
		}
		accepted.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	// snippet:start http-sources
	source, err := feature.NewHTTPSource(feature.HTTPConfig{URL: server.URL})
	if err != nil {
		panic(err)
	}
	exporter, err := feature.NewHTTPExporter(feature.ExporterConfig{HTTP: feature.HTTPConfig{URL: server.URL}})
	if err != nil {
		panic(err)
	}
	engine, err := feature.Open(ctx, source, feature.WithNotifier(exporter), feature.WithEventSink(exporter))
	if err != nil {
		panic(err)
	}
	flags := feature.New(engine, feature.WithObserver(exporter))
	fmt.Println("Remote theme:", flags.String(ctx, "theme", "classic").Value)
	// Remote theme: blue

	if err := flags.Track(ctx, "checkout", feature.Event{}); err != nil {
		panic(err)
	}
	if err := exporter.LastError(); err != nil {
		panic(err)
	}
	fmt.Println("Accepted events:", accepted.Load())
	// Accepted events: 4
	// snippet:end http-sources
}

// Output:
// Remote theme: blue
// Accepted events: 4
