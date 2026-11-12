// Command portable demonstrates the provider-neutral application API with a native evaluator.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
)

// main binds targeting once while retaining an explicit failure-explanation path.
func main() {
	ctx := context.Background()
	source, err := feature.NewMemorySource(feature.Document{Schema: 1, Flags: map[string]feature.Flag{
		"checkout-v2": {Rules: []feature.Rule{{Subjects: []string{"tenant:acme"}, Value: true}}},
	}})
	if err != nil {
		panic(err)
	}
	engine, err := feature.Open(ctx, source)
	if err != nil {
		panic(err)
	}
	// snippet:start portable
	features := feature.New(engine)
	checkout := features.For(feature.Target{
		Key:        "tenant:acme",
		Kind:       "organization",
		Attributes: map[string]any{"plan": "pro", "paid": true},
	})
	fmt.Println("Tenant checkout:", checkout.Enabled(ctx, "checkout-v2"))
	// Tenant checkout: true
	fmt.Println("Untargeted checkout:", features.Enabled(ctx, "checkout-v2"))
	// Untargeted checkout: false
	decision := checkout.Evaluate(ctx, "unknown-feature", true)
	fmt.Println("Explicit fallback:", decision.Value, decision.Reason)
	// Explicit fallback: true missing
	// snippet:end portable
}

// Output:
// Tenant checkout: true
// Untargeted checkout: false
// Explicit fallback: true missing
