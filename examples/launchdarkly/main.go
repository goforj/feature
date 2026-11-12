// Command launchdarkly demonstrates borrowed and owned adapters with the real SDK's offline test data source.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/launchdarklyfeature"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
)

// main keeps provider configuration at startup while application checks use the portable API.
func main() {
	ctx := context.Background()
	data := ldtestdata.DataSource()
	data.Update(data.Flag("checkout-v2").BooleanFlag().VariationForUser("tenant:acme", true).FallthroughVariation(false))
	// This runnable example uses local data and fake credentials, with no hosted traffic.
	config := ldclient.Config{DataSource: data, Events: ldcomponents.NoEvents(), Logging: ldcomponents.NoLogging()}
	sdk, err := ldclient.MakeCustomClient("offline-example", config, time.Second)
	if err != nil {
		panic(err)
	}
	defer sdk.Close()
	// snippet:start launchdarkly-borrowed
	borrowed := launchdarklyfeature.New(sdk)
	defer borrowed.Close()
	flags := feature.New(borrowed).For(feature.Target{Key: "tenant:acme"})
	fmt.Println("Borrowed checkout:", flags.Enabled(ctx, "checkout-v2"))
	// Borrowed checkout: true
	// snippet:end launchdarkly-borrowed
	// snippet:start launchdarkly-owned
	owned, err := launchdarklyfeature.Open(ctx, launchdarklyfeature.Config{
		SDKKey: "offline-example", SDKConfig: config,
	})
	if err != nil {
		panic(err)
	}
	defer owned.Close()
	checkout := feature.New(owned).For(feature.Target{Key: "tenant:acme"})
	fmt.Println("Owned checkout:", checkout.Enabled(ctx, "checkout-v2"))
	// Owned checkout: true
	// snippet:end launchdarkly-owned
}

// Output:
// Borrowed checkout: true
// Owned checkout: true
