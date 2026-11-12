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

The reviewed candidate passed exact unit coverage, all 75 container/SDK integration cases with race detection, race checks and vet across relevant modules, tidy parity, module inventory, executable examples, and generated-document parity. Minimum-version suites passed on Go 1.24.13, 1.25.5, and 1.26.0. Decoder fuzzing completed 70,612 executions without a failure. Fresh govulncheck v1.9.0 scans on Go 1.27.4 found no vulnerabilities in any runtime package.

## Documentation and distribution

Corrected contract text that still described implemented SDK adapters as future work. Canonical programs remain the only executable example source, documentation generation runs twice for parity, and the native root still has no vendor dependencies. Native, LaunchDarkly, and OpenFeature minimums remain Go 1.24, 1.25, and 1.26 respectively.

There are no release tags or release scripts yet. The published-module inventory is root plus the two drivers; examples and integration are tooling modules. Future tags must independently publish each runtime module:

| Module | Tag shape |
| :--- | :--- |
| github.com/goforj/feature | vX.Y.Z |
| github.com/goforj/feature/driver/launchdarklyfeature | driver/launchdarklyfeature/vX.Y.Z |
| github.com/goforj/feature/driver/openfeaturefeature | driver/openfeaturefeature/vX.Y.Z |

A root tag does not publish either driver. Before a versioned release, select intended root requirements for each driver, preserve development replacements, and repeat independent and combined consumer tests with GOWORK=off and no replacements against actual published versions. Existing private Git consumer checks are recorded in [verification](verification.md). Public distribution, hosted LaunchDarkly account checks, Codecov configuration, and real GoForj App adoption remain separate pending work.
