//go:build !race

package examples_test

// exampleRace avoids requiring the race runtime during ordinary example execution.
const exampleRace = false
