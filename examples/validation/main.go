// Command validation demonstrates explicit authoring feedback and redacted runtime errors.
package main

import (
	"errors"
	"fmt"

	"github.com/goforj/feature"
)

// main locates a missing variant without putting definition details into ordinary error text.
func main() {
	// snippet:start validation
	document := feature.Document{
		Schema: feature.TypedSchemaVersion,
		Flags: map[string]feature.Flag{
			"theme": {DefaultVariant: "missing"},
		},
	}
	if err := feature.Validate(document); err != nil {
		var issue *feature.ValidationError
		if !errors.As(err, &issue) {
			panic(err)
		}
		fmt.Println("Definition path:", issue.Path)
		// Definition path: /flags/theme/default_variant

		fmt.Println("Problem:", issue.Problem)
		// Problem: default variant must reference the flag's variant catalog

		fmt.Println("Runtime classification:", err)
		// Runtime classification: feature: invalid document
	}
	// snippet:end validation
}

// Output:
// Definition path: /flags/theme/default_variant
// Problem: default variant must reference the flag's variant catalog
// Runtime classification: feature: invalid document
