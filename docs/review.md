# Library review

This review covers the native engine, portable facade, both SDK drivers, executable examples, repository tooling, and real-container integration harnesses. It preserves public API signatures, native JSON schema 1, rollout encoding, and each module's Go minimum. It does not authorize publication or framework integration.

## API and lifecycle

Two concrete lifecycle findings were reproduced and fixed:

1. Both drivers used an atomic flag as the entire Close guard. A concurrent caller could return before owned cleanup completed, and repeated calls lost a cleanup error. Close now uses one synchronized cleanup attempt, disables new evaluations before cleanup, waits for completion, and returns the same safe result to every caller. Successful borrowed and owned shutdown retain their behavior. The observable correction is that repeated calls after failed cleanup continue to return ErrUnavailable rather than nil.
2. OpenFeature could return a usable owned driver when its SDK initializer returned nil after the caller canceled. Initialization now checks cancellation before publishing the driver and cleans up the isolated API. The cancellation error takes precedence over upstream diagnostics.

New tests fail against the preceding implementation using Go source overlays, without modifying the working tree. They pass after the fixes. Each driver suite also passed ten repeated race runs. Tests coordinate the active cleanup with channels and check stable results for all callers and repeated calls.

Provider shutdown deadlines are cooperative. OpenFeature's legacy lifecycle interface has no context argument, so a five-second context cannot forcibly stop a legacy provider. The contract and driver guide now explain this limit. Required injected dependencies still fail visibly when miswired.

## Native correctness and resource ownership

Reviewed native publication, source serialization, cancellation boundaries, snapshot ownership, revision normalization, strict JSON scanning, aggregate bounds, stable rollout vectors, and freshness counters. Existing tests cover atomic replacement, invalid and missing files, cancellation, concurrent readers, rule order, and missing versus empty attributes. No native schema, revision, hashing, or API changes were needed.

Target cloning bounds depth, scalar types, collection lengths, and aggregate accounting. Cycles terminate at the nesting limit. Mutable maps and slices are copied; SDK adapters own another conversion copy before passing data into their SDK. Native string-only subjects remain distinct from typed targets. Provider evaluation, precision, context interpretation, and freshness remain native SDK responsibilities.

## Integration and coverage evidence

The shared contract uses the real LaunchDarkly Relay container for both direct and OpenFeature LaunchDarkly evaluation, and the real GO Feature Flag container for remote and in-process evaluation. Pinned images, observed running versions, isolated keys, partial-start cleanup, pause/resume recovery, authentication, and analytics forwarding were reviewed. Local upstream fixtures do not claim hosted account verification.

The unit command previously trusted the listed modules without checking for unlisted go.mod files. It now rejects inventory drift before measuring coverage. The inventory is also part of the evidence fingerprint, and a regression proves that changing it invalidates old evidence. An unused searchtest coverage exclusion copied from a sibling library was removed; unclassified packages must remain visible to the coverage gate.

Unit coverage remains an exact independent requirement. Integration coverage cannot fill unit gaps. The current [unit](unit-results.json) and [integration](integration-results.json) reports provide measured totals and source fingerprints.

The first reviewed candidate passed exact unit coverage, all 75 then-required container/SDK integration cases with race detection, race checks and vet across relevant modules, tidy parity, module inventory, executable examples, and generated-document parity. Minimum-version suites passed on Go 1.24.13, 1.25.5, and 1.26.0. Decoder fuzzing completed 70,612 executions without a failure. Fresh govulncheck v1.9.0 scans on Go 1.27.4 found no vulnerabilities in any runtime package. Subsequent review results are recorded below and in the current execution reports.

## Additional cancellation and interoperability passes

Three more concrete findings were reproduced before changing runtime code:

1. Refresh could choose a ready source-gate acquisition after its context was canceled. The old implementation entered the source 58 times in the 128-attempt checkpoint regression. Refresh now checks cancellation again after acquisition, before invoking the source. The test coordinates cancellation at the pre-acquisition observation and requires no source calls regardless of which ready select branch wins.
2. LaunchDarkly SDK construction errors replaced concurrent caller cancellation with ErrUnavailable. Cancellation now takes precedence after partial-client cleanup, including constructors that return no client. Both paths have direct regressions.
3. Native typed Resolve validated target data before checking cancellation or key grammar. With invalid targets it disagreed with native Evaluate and omitted revision and accounting information. These early failures now delegate to Evaluate before conversion, preserving native precedence, revisions, and counters. Tests compare complete decisions and accounting for the conflicting-input combinations.

The subsequent native-constructor check found a fourth startup defect: MakeCustomClient returns a nil native pointer on invalid SDK configuration. Converting that pointer directly to the managed-client interface made it appear non-nil, so cleanup could panic. The wrapper now preserves an absent constructor result before interface conversion. A real SDK test with a missing CA file reproduces the previous panic and now requires a safe ErrUnavailable result with no driver or path disclosure. This check concerns an optional constructor result, not a guard masking a required injected dependency.

The affected cancellation suites passed twenty repeated race runs. New real-Relay tests passed three repeated race runs for both direct and OpenFeature adapters. They exercise multi-kind targeting, boolean/numeric rules, configured true off-variations, and wire-level privacy. Privacy assertions require a redacted private email, preserved public attributes and native redaction metadata, no secret in any forwarded batch, and a successful unredacted control.

The full integration gate now also requires those context cases and verifies that support metadata agrees with image constants and actual selected SDK/provider module versions. Earlier metadata checks only proved the running service version against code constants; they could not detect a report-only version edit. Version-report drift now fails verification. Initialization documentation also makes the native SDK factory deadline limit explicit.

This follow-up passed all 87 required integration cases with race detection, exact 100% runtime unit statement coverage, all seven executable programs, race and vet checks, tidy and module inventory checks, and generated-document parity. Current statement counts are recorded in the unit evidence. Minimum-version suites and all three runtime vulnerability scans also passed. The public signatures, schema, rollout encoding, dependency versions, and module Go minimums are unchanged.

Fresh independent native, LaunchDarkly, OpenFeature, and combined Git consumers passed against runtime commit `8e7a76c0f17b6e6e0e8cb3042a19de2f231ae9fa` without local replacements. These checks additionally exercised native validation precedence and accounting, actual invalid SDK TLS configuration, caller cancellation during a real SDK event-processor factory error, and repeated native OpenFeature provider cleanup failures. The standalone driver consumers retained their declared older root requirement, while native and combined consumers selected the reviewed root. Toolchain details are in verification.md. CI and Security passed for that runtime commit.

## Further evidence and SDK boundary passes

A self-mutating disposable module reproduced a tooling defect: the unit command could test the original binary, compile its example after a source edit, and report success with the new source fingerprint. Unit and integration verification now capture fingerprints before execution and reject observed changes before recording success. Direct command-boundary regressions exercise actual Go subprocesses that change their source during execution. Focused integration runs apply the same check. This detects changed final inputs; it does not provide a filesystem snapshot or detect edits reverted before the final fingerprint.

Additional real-SDK regressions preserve OpenFeature API, transaction, client, and invocation context precedence, including inherited identities and reserved targeting attributes. A failed attempt to bind an already-owned provider must leave its original API working without shutting down that provider. LaunchDarkly offline construction must remain successful while evaluations return the caller's explicit fallback and the normalized unavailable error. These SDK checks passed ten repeated race runs without requiring runtime changes.

The coverage inventory was also checked with a disposable unimported runtime package: its untested statements remain visible in Go's coverage profile. No additional exclusion or runtime API was needed. This pass changes verification tooling, tests, and documentation; runtime behavior, public signatures, configuration, dependencies, and Go minimums are preserved.

This candidate passed 169 unit cases, all seven executable programs, 565/565 runtime statements, and all 87 separate integration cases with race detection. Module inventory, tidy parity, generated-document parity, race checks, and vet passed across the relevant modules. Native Go 1.24.13, LaunchDarkly Go 1.25.5, and OpenFeature Go 1.26.0 suites also passed with race detection.

## Further propagation and document compatibility passes

The shared integration contract previously observed a false live update through Enabled. Because Enabled uses a false fallback, a provider failure could satisfy that observation. Both update directions now require a successful Decision and use the opposite fallback. The shared contract also requires all five backend configurations to delete an existing flag, observe ErrNotFound with both explicit fallbacks, and recreate the flag as a successful false decision. The real LaunchDarkly Relay receives versioned native tombstones; GO Feature Flag retrieves a complete document with the flag removed. Each adapter continues to use its provider's own update lifecycle.

The integration evidence gate now requires deletion and recreation for every backend. Direct regressions remove each backend's deletion result from otherwise complete evidence and require rejection. The decoder fuzz property also checks that every accepted document can be encoded and decoded without changing its canonical revision, with additional valid rule and rollout seeds.

These passes found gaps in test assertions and behavioral coverage, without identifying another runtime defect. Public APIs, configuration, schema, revision and rollout algorithms, dependencies, and Go minimums are unchanged.

The candidate passed 176 unit cases, all seven executable programs, 565/565 runtime statements, and 92 separate integration cases with race detection. Module inventory, tidy parity, generated-document parity, race checks, and vet passed across the relevant modules. The native Go 1.24.13 race suite passed. Decoder fuzzing completed 9,075,206 executions in 46 seconds without a failure; this count includes rejected inputs as well as accepted-document round trips.

## Documentation and distribution

Corrected contract text that still described implemented SDK adapters as future work. Canonical programs remain the only executable example source, documentation generation runs twice for parity, and the native root still has no vendor dependencies. Native, LaunchDarkly, and OpenFeature minimums remain Go 1.24, 1.25, and 1.26 respectively.

There are no release tags or release scripts yet. The published-module inventory is root plus the two drivers; examples and integration are tooling modules. Future tags must independently publish each runtime module:

| Module | Tag shape |
| :--- | :--- |
| github.com/goforj/feature | vX.Y.Z |
| github.com/goforj/feature/driver/launchdarklyfeature | driver/launchdarklyfeature/vX.Y.Z |
| github.com/goforj/feature/driver/openfeaturefeature | driver/openfeaturefeature/vX.Y.Z |

A root tag does not publish either driver. Before a versioned release, select intended root requirements for each driver, preserve development replacements, and repeat independent and combined consumer tests with GOWORK=off and no replacements against actual published versions. Fresh private Git consumers passed against the reviewed runtime commit, including a real OpenFeature provider shutdown-error regression also retained in the unit suite. Exact versions and toolchains are recorded in [verification](verification.md). Public distribution, hosted LaunchDarkly account checks, Codecov configuration, and real GoForj App adoption remain separate pending work.
