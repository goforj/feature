// Command events demonstrates portable analytics, observations, and explicit native listener ownership.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
)

// main leaves event delivery and metric-label selection under application control.
func main() {
	ctx := context.Background()
	source, err := feature.NewMemorySource(feature.Document{Schema: 1, Flags: map[string]feature.Flag{"checkout": {Default: true}}})
	if err != nil {
		panic(err)
	}
	sink := feature.TrackerFunc(func(_ context.Context, name string, _ feature.Target, _ feature.Event) error {
		fmt.Println("Tracked:", name)
		return nil
	})
	observations := 0
	// snippet:start events
	engine, err := feature.Open(ctx, source, feature.WithEventSink(sink))
	if err != nil {
		panic(err)
	}
	flags := feature.New(engine, feature.WithObserver(feature.ObserverFunc(func(context.Context, feature.Observation) { observations++ })))
	fmt.Println("Tracking configured:", flags.SupportsTracking())
	// Tracking configured: true

	if err := flags.Track(ctx, "checkout", feature.Event{}); err != nil {
		panic(err)
	}
	// Tracked: checkout

	if err := flags.For(feature.Target{Key: "tenant:acme"}).Track(ctx, "purchase", feature.Event{Attributes: map[string]any{"currency": "USD"}}); err != nil {
		panic(err)
	}
	// Tracked: purchase

	fmt.Println("Observed operations:", observations)
	// Observed operations: 2

	subscription, err := engine.Subscribe()
	if err != nil {
		panic(err)
	}
	defer subscription.Close()
	<-subscription.Changes
	// snippet:end events
	if err := source.Replace(feature.Document{Schema: 1, Flags: map[string]feature.Flag{"checkout": {Default: false}}}); err != nil {
		panic(err)
	}
	if err := engine.Refresh(ctx); err != nil {
		panic(err)
	}
	change := <-subscription.Changes
	fmt.Println("Changed flags:", len(change.Keys))
	// Changed flags: 1
}

// Output:
// Tracking configured: true
// Tracked: checkout
// Tracked: purchase
// Observed operations: 2
// Changed flags: 1
