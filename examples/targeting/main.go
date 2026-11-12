// Command targeting demonstrates exact attributes and stable percentage targeting.
package main

import (
	"context"
	"fmt"

	"github.com/goforj/feature"
)

// main enables a checkout experiment for a stable subset of pro tenants.
func main() {
	ctx := context.Background()
	// snippet:start decode
	document, err := feature.Decode([]byte(`{
  "schema": 1,
  "flags": {
   "checkout-v2": {
    "default": false,
    "rules": [{
     "attributes": {"plan": "pro"},
     "rollout": {"basis_points": 1000, "salt": "checkout-2026"},
     "value": true
    }]
   }
  }
 }`))
	if err != nil {
		panic(err)
	}
	// snippet:end decode
	source, err := feature.NewMemorySource(document)
	if err != nil {
		panic(err)
	}
	features, err := feature.Open(ctx, source)
	if err != nil {
		panic(err)
	}
	subject := feature.Subject{Key: "tenant:demo-2", Attributes: map[string]string{"plan": "pro"}}
	fmt.Println("Pro tenant:", features.Enabled(ctx, "checkout-v2", false, subject))
	// Pro tenant: true
	subject.Attributes["plan"] = "free"
	fmt.Println("Free tenant:", features.Enabled(ctx, "checkout-v2", false, subject))
	// Free tenant: false
}

// Output:
// Pro tenant: true
// Free tenant: false
