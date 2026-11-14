// Command settings demonstrates app-wide defaults and per-user behavior overrides.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
)

// main shares one engine across app-wide reads and owned user scopes.
func main() {
	// snippet:start settings
	ctx := context.Background()
	document := feature.Document{
		Schema: feature.TypedSchemaVersion,
		Flags: map[string]feature.Flag{
			"maintenance": {Default: false},
			"theme": {
				Default: "light",
				Rules: []feature.Rule{
					{Subjects: []string{"user:42"}, Value: "dark"},
				},
			},
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
	features := feature.New(engine)

	fmt.Println("App maintenance:", features.Enabled(ctx, "maintenance"))
	// App maintenance: false

	fmt.Println("App theme:", features.String(ctx, "theme", "light").Value)
	// App theme: light

	user := features.For(feature.Target{Key: "user:42"})
	fmt.Println("User 42 theme:", user.String(ctx, "theme", "light").Value)
	// User 42 theme: dark

	other := features.For(feature.Target{Key: "user:99"})
	fmt.Println("User 99 theme:", other.String(ctx, "theme", "light").Value)
	// User 99 theme: light
	// snippet:end settings
}

// Output:
// App maintenance: false
// App theme: light
// User 42 theme: dark
// User 99 theme: light
