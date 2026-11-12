// Command quickstart demonstrates injected feature evaluation and explicit fallbacks.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
)

// main evaluates a checkout rollout for a real application identity.
func main() {
	ctx := context.Background()
	// snippet:start construction
	source, err := feature.NewMemorySource(feature.Document{
		Schema: feature.SchemaVersion,
		Flags: map[string]feature.Flag{
			"checkout-v2": {
				Default: false,
				Rules: []feature.Rule{
					{Subjects: []string{"tenant:acme"}, Value: true},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	features, err := feature.Open(ctx, source)
	if err != nil {
		panic(err)
	}
	// snippet:end construction

	// snippet:start enabled
	subject := feature.Subject{Key: "tenant:acme"}
	fmt.Println("New checkout:", features.Enabled(ctx, "checkout-v2", false, subject))
	// New checkout: true
	// snippet:end enabled

	// snippet:start evaluate
	decision := features.Evaluate(ctx, "unknown-feature", false, subject)
	fmt.Println("Unknown feature:", decision.Value, decision.Reason)
	// Unknown feature: false missing
	// snippet:end evaluate
}

// Output:
// New checkout: true
// Unknown feature: false missing
