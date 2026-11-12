// Command refresh demonstrates complete file replacement and captured request views.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goforj/feature"
)

// main adopts a new document while preserving an earlier request's view.
func main() {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "feature-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "flags.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"flags":{"checkout-v2":{"default":false}}}`), 0600); err != nil {
		panic(err)
	}
	// snippet:start file
	features, err := feature.Open(ctx, feature.NewFileSource(path))
	if err != nil {
		panic(err)
	}
	// snippet:end file

	// snippet:start snapshot
	view := features.Snapshot()
	fmt.Println("Captured checkout:", view.Enabled(ctx, "checkout-v2", false, feature.Subject{}))
	// Captured checkout: false
	// snippet:end snapshot

	// snippet:start refresh
	next := path + ".next"
	if err := os.WriteFile(next, []byte(`{"schema":1,"flags":{"checkout-v2":{"default":true}}}`), 0600); err != nil {
		panic(err)
	}
	if err := os.Rename(next, path); err != nil {
		panic(err)
	}
	if err := features.Refresh(ctx); err != nil {
		panic(err)
	}
	fmt.Println("Current checkout:", features.Enabled(ctx, "checkout-v2", false, feature.Subject{}))
	// Current checkout: true
	fmt.Println("Captured checkout:", view.Enabled(ctx, "checkout-v2", false, feature.Subject{}))
	// Captured checkout: false
	// snippet:end refresh

	// snippet:start status
	status := features.Status()
	fmt.Println("Refresh healthy:", status.LastError == nil && !status.LastSuccess.IsZero())
	// Refresh healthy: true
	// snippet:end status
}

// Output:
// Captured checkout: false
// Current checkout: true
// Captured checkout: false
// Refresh healthy: true
