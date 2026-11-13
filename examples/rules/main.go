// Command rules demonstrates typed conditions, named variants, and time-consistent rollout views.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/goforj/feature"
)

// main binds evaluation time without introducing another refresh or scheduling worker.
func main() {
	ctx := context.Background()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	now := start.Add(-time.Second)
	document := feature.Document{Schema: feature.TypedSchemaVersion, Flags: map[string]feature.Flag{
		"theme": {
			Variants:       map[string]any{"control": "blue", "treatment": "green"},
			DefaultVariant: "control",
			Rules: []feature.Rule{{
				Conditions: []feature.Condition{
					{Attribute: "paid", Operator: feature.OpEqual, Value: true},
					{Attribute: "seats", Operator: feature.OpGreaterOrEqual, Value: 10},
				},
				Variant: "treatment",
			}},
		},
		"experiment": {
			Variants:       map[string]any{"control": "blue", "treatment": "green"},
			DefaultVariant: "control",
			Rules: []feature.Rule{{Split: &feature.Split{
				Salt: "theme-experiment-v1",
				Allocations: []feature.Allocation{
					{Variant: "control", BasisPoints: 7500},
					{Variant: "treatment", BasisPoints: 2500},
				},
			}}},
		},
		"launch": {
			Default: false,
			Rules:   []feature.Rule{{Start: &start, End: &end, Value: true}},
		},
		"rollout": {
			Default: false,
			Rules: []feature.Rule{{
				Rollout: &feature.Rollout{
					Salt:        "launch-v1",
					Progressive: &feature.Progressive{Start: start, End: end, From: 0, To: 10000},
				},
				Value: true,
			}},
		},
	}}
	source, err := feature.NewMemorySource(document)
	if err != nil {
		panic(err)
	}
	// snippet:start native-rules
	engine, err := feature.Open(ctx, source, feature.WithClock(func() time.Time { return now }))
	if err != nil {
		panic(err)
	}
	target := feature.Target{Key: "tenant:acme", Attributes: map[string]any{"paid": true, "seats": 12}}
	flags := feature.New(engine).For(target)
	fmt.Println("Paid tenant theme:", flags.String(ctx, "theme", "classic").Value)
	// Paid tenant theme: green

	experiment := flags.String(ctx, "experiment", "blue")
	fmt.Println("Weighted experiment:", experiment.Value, experiment.Variant)
	// Weighted experiment: blue control

	request := feature.New(engine.Snapshot()).For(target)
	now = start
	fmt.Println("Scheduled launch:", flags.Enabled(ctx, "launch"))
	// Scheduled launch: true

	now = end
	fmt.Println("Rollout at completion:", flags.Enabled(ctx, "rollout"))
	// Rollout at completion: true

	fmt.Println("Captured request launch:", request.Enabled(ctx, "launch"))
	// Captured request launch: false
	// snippet:end native-rules
}

// Output:
// Paid tenant theme: green
// Weighted experiment: blue control
// Scheduled launch: true
// Rollout at completion: true
// Captured request launch: false
