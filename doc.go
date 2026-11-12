// Package feature evaluates live boolean feature flags against immutable snapshots.
//
// Open loads a complete source synchronously. Applications own the refresh lifecycle:
// call Refresh explicitly or run Run with a cancellable context. Evaluations never
// perform I/O. Snapshot captures a revision for consistent decisions within a job
// or request. Refresh failures retain the last valid document.
//
// Clients and views support concurrent use. Subjects are borrowed only during an
// evaluation and must not be mutated concurrently. Sources passed to Open are
// required collaborators, remain caller-owned, and are never closed by the client.
package feature
