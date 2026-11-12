// Command nativeprovider exposes the native engine to an ordinary OpenFeature SDK client.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/openfeaturefeature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

// main leaves refresh and engine ownership with the application.
func main() {
	ctx := context.Background()
	source, err := feature.NewMemorySource(feature.Document{Schema: 2, Flags: map[string]feature.Flag{"theme": {Default: "blue"}}})
	if err != nil {
		panic(err)
	}
	engine, err := feature.Open(ctx, source)
	if err != nil {
		panic(err)
	}
	// snippet:start native-provider
	provider := openfeaturefeature.NewProvider(engine)
	api := isolated.NewAPI()
	defer api.Shutdown(ctx)
	if err := api.SetProviderAndWait(ctx, provider); err != nil {
		panic(err)
	}
	sdk := api.NewClient()
	theme, err := sdk.StringValue(ctx, "theme", "classic", openfeature.NewTargetlessEvaluationContext(nil))
	if err != nil {
		panic(err)
	}
	fmt.Println("OpenFeature theme:", theme)
	// OpenFeature theme: blue
	// snippet:end native-provider
}

// Output:
// OpenFeature theme: blue
