// Command openfeature demonstrates isolated and borrowed OpenFeature SDK lifecycles without hosted credentials.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/openfeaturefeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
	"github.com/open-feature/go-sdk/openfeature/memprovider"
)

// main leaves provider choice outside application decisions and makes lifecycle ownership explicit.
func main() {
	ctx := context.Background()
	definitions := map[string]memprovider.InMemoryFlag{
		"checkout-v2": {Key: "checkout-v2", State: memprovider.Enabled, DefaultVariant: "on", Variants: map[string]any{"on": true}},
	}
	// Any compatible provider can replace this real SDK memory provider at startup.
	provider := memprovider.NewInMemoryProvider(definitions)
	// snippet:start openfeature-owned
	owned, err := openfeaturefeature.Open(ctx, provider)
	if err != nil {
		panic(err)
	}
	defer owned.Close()
	checkout := feature.New(owned).For(feature.Target{
		Key: "tenant:acme", Attributes: map[string]any{"plan": "pro", "seats": 12},
	})
	fmt.Println("Owned checkout:", checkout.Enabled(ctx, "checkout-v2"))
	// Owned checkout: true
	// snippet:end openfeature-owned
	api := isolated.NewAPI()
	defer api.Shutdown(ctx)
	if err := api.SetProviderAndWait(ctx, memprovider.NewInMemoryProvider(definitions)); err != nil {
		panic(err)
	}
	sdk := api.NewClient()
	// snippet:start openfeature-borrowed
	borrowed := openfeaturefeature.New(sdk)
	defer borrowed.Close()
	flags := feature.New(borrowed)
	fmt.Println("Borrowed checkout:", flags.Enabled(ctx, "checkout-v2"))
	// Borrowed checkout: true
	// snippet:end openfeature-borrowed
}

// Output:
// Owned checkout: true
// Borrowed checkout: true
