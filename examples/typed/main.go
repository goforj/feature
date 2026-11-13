// Command typed demonstrates all portable value types and native typed snapshots.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
)

// main keeps typed configuration behind the same injected feature client.
func main() {
	ctx := context.Background()
	document := feature.Document{Schema: feature.TypedSchemaVersion, Flags: map[string]feature.Flag{
		"theme":       {Default: "classic"},
		"page-size":   {Default: 25},
		"sample-rate": {Default: 0.25},
		"layout":      {Default: map[string]any{"columns": 2}},
		"menu":        {Default: []any{"home", "billing"}},
	}}
	encoded, err := feature.Encode(document)
	if err != nil {
		panic(err)
	}
	document, err = feature.Decode(encoded)
	if err != nil {
		panic(err)
	}
	source, err := feature.NewMemorySource(document)
	if err != nil {
		panic(err)
	}
	engine, err := feature.Open(ctx, source)
	if err != nil {
		panic(err)
	}
	flags := feature.New(engine)
	// snippet:start typed-values
	fmt.Println("Theme:", flags.String(ctx, "theme", "classic").Value)
	// Theme: classic

	fmt.Println("Page size:", flags.Int(ctx, "page-size", 10).Value)
	// Page size: 25

	fmt.Println("Sample rate:", flags.Float(ctx, "sample-rate", 0).Value)
	// Sample rate: 0.25

	layout := flags.Object(ctx, "layout", map[string]any{"columns": 1})
	fmt.Println("Columns:", layout.Value.(map[string]any)["columns"])
	// Columns: 2

	fmt.Println("Menu:", flags.Object(ctx, "menu", []any{}).Value)
	// Menu: [home billing]
	// snippet:end typed-values

	// snippet:start typed-decoding
	type layoutConfig struct {
		Columns int `json:"columns"`
	}
	layoutResult := feature.ObjectAs(ctx, flags, "layout", layoutConfig{Columns: 1})
	fmt.Println("Typed columns:", layoutResult.Value.Columns)
	// Typed columns: 2

	menu := feature.ObjectAs(ctx, flags.For(feature.Target{Key: "tenant:acme"}), "menu", []string{"home"})
	fmt.Println("Typed menu:", menu.Value)
	// Typed menu: [home billing]
	// snippet:end typed-decoding
}

// Output:
// Theme: classic
// Page size: 25
// Sample rate: 0.25
// Columns: 2
// Menu: [home billing]
// Typed columns: 2
// Typed menu: [home billing]
