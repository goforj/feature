// Command quickstart demonstrates typed targeting and live updates with request consistency.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
)

// main changes a live setting while a captured request retains its original decision.
func main() {
	// snippet:start construction
	ctx := context.Background()
	document := feature.Document{
		Schema: feature.TypedSchemaVersion,
		Flags: map[string]feature.Flag{
			"checkout-v2": {
				Default: false,
				Rules: []feature.Rule{
					{Attributes: map[string]string{"plan": "pro"}, Value: true},
				},
			},
			"theme":       {Default: "midnight"},
			"page-size":   {Default: 50},
			"sample-rate": {Default: 0.25},
			"layout":      {Default: map[string]any{"columns": 3}},
		},
	}
	source, err := feature.NewMemorySource(document)
	if err != nil {
		panic(err)
	}
	engine, err := feature.Open(ctx, source)
	if err != nil {
		panic(err)
	}
	// snippet:end construction

	// snippet:start quickstart-values
	target := feature.Target{
		Key:        "tenant:acme",
		Attributes: map[string]any{"plan": "pro"},
	}
	flags := feature.New(engine).For(target)
	fmt.Println("New checkout:", flags.Enabled(ctx, "checkout-v2"))
	// New checkout: true

	fmt.Println("Theme:", flags.String(ctx, "theme", "classic").Value)
	// Theme: midnight

	fmt.Println("Page size:", flags.Int(ctx, "page-size", 25).Value)
	// Page size: 50

	fmt.Println("Sample rate:", flags.Float(ctx, "sample-rate", 0).Value)
	// Sample rate: 0.25

	layout := flags.Object(ctx, "layout", map[string]any{"columns": 1})
	fmt.Println("Columns:", layout.Value.(map[string]any)["columns"])
	// Columns: 3
	// snippet:end quickstart-values

	// snippet:start quickstart-refresh
	request := feature.New(engine.Snapshot()).For(target)
	document.Flags["theme"] = feature.Flag{Default: "sunrise"}
	if err := source.Replace(document); err != nil {
		panic(err)
	}
	if err := engine.Refresh(ctx); err != nil {
		panic(err)
	}
	fmt.Println("Live theme:", flags.String(ctx, "theme", "classic").Value)
	// Live theme: sunrise

	fmt.Println("Captured request theme:", request.String(ctx, "theme", "classic").Value)
	// Captured request theme: midnight
	// snippet:end quickstart-refresh

	// snippet:start enabled
	subject := feature.Subject{Key: "tenant:acme", Attributes: map[string]string{"plan": "pro"}}
	fmt.Println("Native checkout:", engine.Enabled(ctx, "checkout-v2", false, subject))
	// Native checkout: true
	// snippet:end enabled

	// snippet:start evaluate
	decision := engine.Evaluate(ctx, "unknown-feature", false, subject)
	fmt.Println("Unknown feature:", decision.Value, decision.Reason)
	// Unknown feature: false missing
	// snippet:end evaluate
}

// Output:
// New checkout: true
// Theme: midnight
// Page size: 50
// Sample rate: 0.25
// Columns: 3
// Live theme: sunrise
// Captured request theme: midnight
// Native checkout: true
// Unknown feature: false missing
