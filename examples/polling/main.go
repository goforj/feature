// Command polling demonstrates an owned refresh goroutine with bounded shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/goforj/feature"
)

// main waits for a live update and joins its refresh goroutine before exiting.
func main() {
	ctx := context.Background()
	source, err := feature.NewMemorySource(feature.Document{Schema: 1, Flags: map[string]feature.Flag{}})
	if err != nil {
		panic(err)
	}
	engine, err := feature.Open(ctx, source)
	if err != nil {
		panic(err)
	}
	features := feature.New(engine)
	// snippet:start run
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- engine.Run(runCtx, 20*time.Millisecond) }()
	// snippet:end run

	// snippet:start replace
	if err := source.Replace(feature.Document{
		Schema: feature.SchemaVersion,
		Flags:  map[string]feature.Flag{"checkout-v2": {Default: true}},
	}); err != nil {
		panic(err)
	}
	// Existing clients adopt replacements on their next refresh.
	// snippet:end replace

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for !features.Enabled(ctx, "checkout-v2") {
		select {
		case <-deadline.C:
			panic("refresh did not converge")
		case <-tick.C:
		}
	}
	fmt.Println("Live checkout:", features.Enabled(ctx, "checkout-v2"))
	// Live checkout: true
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		panic(err)
	}
	fmt.Println("Refresh loop stopped")
	// Refresh loop stopped
}

// Output:
// Live checkout: true
// Refresh loop stopped
