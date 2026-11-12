// Command maintain generates documentation and verifies repository unit evidence.
package main

import (
	"fmt"
	"os"

	"github.com/goforj/feature/internal/devtools"
)

// main keeps repository maintenance in the same standard-library Go toolchain as the library.
func main() {
	root, err := os.Getwd()
	if err == nil {
		if len(os.Args) != 2 {
			err = fmt.Errorf("usage: maintain generate|check-generated|test-unit|record-unit")
		} else {
			switch os.Args[1] {
			case "modules-check":
				err = devtools.CheckModules(root)
			case "test-all":
				err = devtools.EachModule(root, "test")
			case "test-race":
				err = devtools.EachModule(root, "test-race")
			case "vet":
				err = devtools.EachModule(root, "vet")
			case "tidy":
				err = devtools.EachModule(root, "tidy")
			case "generate":
				err = devtools.Generate(root)
			case "check-generated":
				err = devtools.CheckGenerated(root)
			case "test-unit":
				err = devtools.TestUnit(root, false, os.Stdout)
			case "record-unit":
				err = devtools.TestUnit(root, true, os.Stdout)
			default:
				err = fmt.Errorf("unknown maintenance command: %s", os.Args[1])
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
