// Package feature provides portable typed feature decisions and a native flag engine.
//
// New borrows an initialized Evaluator. Features provides short checks and typed
// request targeting through For. String, Int, Float, and Object retain explicit
// fallbacks and detailed results. ObjectAs decodes structured flags into application
// types. Features.Snapshot preserves facade policy while capturing native decisions.
// Native Client implements TypedEvaluator while retaining source, refresh, and status
// ownership. Application services normally use Features and Scope.
//
// Open loads a complete source synchronously. Applications own the refresh lifecycle:
// call Refresh explicitly or run Run with a cancellable context. Native decisions
// read local immutable state; SDK providers, observers, and sinks follow their
// configured I/O behavior. Snapshot captures a revision and evaluation time for
// consistent decisions within a job or request. Refresh failures retain the last
// valid document.
// Validate supplies explicit authoring diagnostics without changing runtime errors.
//
// Clients and views support concurrent use. Subjects are borrowed only during an
// evaluation and must not be mutated concurrently. Sources passed to Open are
// required collaborators, remain caller-owned, and are never closed by the client.
package feature
