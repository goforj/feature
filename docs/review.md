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

## Further isolation and consumer guidance passes

Rechecked native publication, strict JSON and target bounds, error redaction, SDK startup and shutdown ownership, and the consumer guidance. No additional runtime defect was found. The root and both driver module suites passed ten shuffled repetitions with race detection using seed 584271. The entire container integration suite passed three shuffled repetitions in one package process, exercising reuse of the same services and namespace isolation across repeated runs. These repetitions remain separate from the single-run tracked integration report.

The independent unit gate again passed 176 cases, seven executable programs, and 565/565 runtime statements. Module inventory, maintenance-tool tests, generated-document parity, and vet passed. Consumer guides now state that direct LaunchDarkly requires an explicit identity, while OpenFeature can inherit configured SDK identity and leaves identity requirements to its provider. The verification overview now describes the current ten-case shared contract, including flag deletion and recreation. This pass changes documentation only.

## Expanded standalone library contract

The maintainer explicitly expanded this unreleased library's scope and allowed API changes needed for a coherent driver abstraction. Typed defaults and results now share one native flag/rule model, while schema 1 canonical bytes and rollout hashing remain frozen by compatibility regressions. Schema 2 adds typed conditions, JSON Pointer paths, named variants, weighted allocation, scheduled windows, progressive rollouts, and explicit off-values. Views fix definitions and evaluation time and implement the portable evaluator contract.

The native OpenFeature provider now forwards typed evaluations, source changes, stale/ready transitions, and custom tracking without owning the underlying engine or starting a second refresh loop. Repeated race tests exposed a startup coalescing case where the first actual update could replace the initial subscription message and then be skipped. Forwarding now distinguishes initial state from a real revision transition. Blocked event delivery is interruptible, subscriptions are bounded, and shutdown waits for owned forwarding.

Native HTTP retrieval, SourceFunc adapters, observers, custom sinks, change subscriptions, and HTTP analytics/webhooks provide extension points without adding root dependencies. HTTP request policy bounds responses and deadlines, copies headers, rejects redirects, and redacts endpoint diagnostics. Tracking identities require explicit export selection. Notifications cannot undo valid publication, and observer delivery failures remain inspectable. Native targeting uses exact numeric comparisons, including nested JSON equality, without narrowing int64 values.

Real-container tests prove LaunchDarkly custom data, metrics, and privacy after crossing the official Relay, and GO Feature Flag collector acceptance in both evaluation modes. The pinned community OpenFeature LaunchDarkly provider lacks Tracker, so known unsupported tracking is rejected. The GO Feature Flag provider's numeric tracking-value omission is documented rather than hidden behind a synthetic generic mapping. An end-to-end native HTTP workflow combines typed rules, calendar snapshots, allocation, custom analytics, refresh, invalid-document retention, and notification failure isolation.

This candidate passed 259 unit cases, all 12 canonical programs, 1729/1729 runtime statements, and 160 separate race-enabled integration cases. Native Go 1.24.13, LaunchDarkly Go 1.25.5, and OpenFeature Go 1.26.0 race suites passed. Module inventory, tidy parity, race checks, vet, maintenance tooling, and generated-document parity passed. Event/lifecycle cases passed ten repeated race runs. Expanded decoder fuzzing completed 3,037,900 executions without a failure. Current reports bind execution totals to source fingerprints; hosted account verification and release publication remain separate.

Linux CI exposed an integration-fixture readiness race: independently published LaunchDarkly flags were not necessarily all visible when the first string flag arrived. Typed fixture setup now waits for every published value, including the update baseline, before running conformance assertions. Evaluation assertions remain immediate after this bounded readiness check. This correction changes test synchronization, not driver behavior.

## Independent review of the expanded library

Four independent reviews examined consumer API and native models, SDK driver behavior, security and resource boundaries, and behavioral test strength. Findings were reproduced before fixes, and the numeric fixes received a second independent review.

JSON integer tokens outside int64 could silently become rounded floats. Declared-float conversion also bypassed integer precision checks. Validation now rejects lossy integer conversions, while Encode preserves floating-point syntax throughout nested values, variants, and conditions. Aggregate limits account for the actual representation. Regression tests preserve large finite float round trips, exact integer boundaries, nested values, and oversized-document rejection.

Ordinary JSON serialization could give distinct integer and float conditions identical revision bytes. A reproduced case changed a decision without changing its revision or notifying subscribers. Schema 2 canonical encoding and per-flag change comparison now preserve numeric distinctions. Variant contradiction checks compare numbers exactly. Existing schema 1 canonical bytes and rollout vectors remain frozen. Float-containing schema 2 documents receive corrected revisions, which can cause one configuration-change notification after an upgrade.

LaunchDarkly string and numeric reads now prefer native typed SDK methods, so SDK type errors reach hooks and analytics before portable result handling. Existing borrowed clients exposing only JSON evaluation remain supported. Hook regressions use the actual SDK. Additional portable object, precision, and size checks still occur after native SDK evaluation and are documented as such.

A mutation changing a weighted split boundary from >= to > passed the earlier tests. New golden vectors reject that mutation at exact boundaries and cover unequal three-way allocation, zero-weight variants, and stable membership after refresh.

The OpenFeature SDK can shut down a provider before its asynchronous initialization returns after cancellation. Its public API offers no join handle. Detaching initialization would suppress caller cancellation and deadlines; wrapping providers would bypass native binding protections. This review preserves those SDK semantics, documents the stateful-provider synchronization requirement in Open's GoDoc and the contract, and records upstream follow-up. Applications can initialize and manage such a provider themselves and borrow its SDK client through New. This limitation remains unresolved for arbitrary providers lacking lifecycle synchronization.

Security review found no actionable issue inside the documented source and collaborator boundaries. Notifier reentrancy restrictions are now visible directly in GoDoc as well as the contract.

The final local gate passed 293 unit cases, all 12 programs, and 1808/1808 runtime statements. All 160 separate container/SDK integration cases passed with race detection. Race checks, vet, tidy parity, module inventory, and generated-document parity passed. Native Go 1.24.13 race tests passed. Decoder fuzzing completed 5,533,691 executions without a failure. The integration gate remains independent of exact unit coverage.

## Interaction review of publication and SDK health

Four additional independent reviews examined native model combinations, lifecycle and health forwarding, SDK value boundaries, and HTTP ownership/error handling. The native publication and provider event fixes were independently reviewed again after implementation. SDK driver numeric, off-variation, and borrowed-capability probes found no additional defect.

Calendar validation accepted local dates whose UTC normalization crossed JSON's supported year range. Serialization then failed silently, and a memory-source clone could drop the malformed flag. Validation now checks normalized years, encoding errors remain visible, and compilation checks the actual serialized size before accepting or hashing a document. A separate reproduced composition of calendar rules and long variant references exceeded 1 MiB despite passing the early size estimate. The final publication check now rejects it. Regressions cover both progressive endpoints independently, valid calendar boundaries, startup rejection, last-valid refresh retention, and bounded compositions.

HTTP client timeouts after response headers now preserve DeadlineExceeded, matching timeouts before headers. Real HTTP tests cover sources, exporters, and last-valid snapshot retention. Native Refresh still redacts source failures as ErrSource when its own context remains active.

The native OpenFeature provider could lose a configuration event when a published revision coalesced with a later source failure. It now forwards the revision before stale health. Known native source failures reject initialization, including additional domain bindings, rather than letting SDK initialization report ready. Existing bindings retain evaluation of the last valid snapshot. Recovery after rejected initialization is covered separately from ordinary stale recovery.

The SDK can overwrite a concurrent provider health event with its own later initialization result. This was reproduced with a real native engine and race detection. Its public provider interface offers no completion hook for ordering those events. Callers must finish SDK registration before native refresh starts and serialize later registrations with Refresh. This operational requirement is explicit in GoDoc and the contract; no timing retry or detached context masks the SDK boundary.

Public signatures, module layout, dependencies, Go minimums, and valid document encoding are unchanged. Configuration validation now rejects previously accepted unencodable dates and oversized composed documents. Native-provider initialization now returns an error for known unhealthy sources. These are validation and readiness corrections; the retained native snapshot remains available.

The final local gate passed 312 unit cases, all 12 programs, and 1823/1823 runtime statements. All 160 separate integration cases passed with race detection. Race checks, vet, tidy parity, module inventory, and generated-document parity passed. Native Go 1.24.13 race tests passed. Decoder fuzzing completed 5,753,089 executions without failure. Independent event/recovery checks passed 100 race-enabled repetitions, and the original external SDK probes passed after the fixes.

## Further numeric boundary review

Four further independent reviews examined model arithmetic, facade/adapter contracts, state and lifecycle behavior, and resource boundaries. The state/lifecycle pass found no additional supported-contract issue and repeated event/recovery race tests successfully. Numeric and contract fixes received independent re-review after implementation.

A one-hour progressive rollout from zero to 10000 basis points computed two basis points rather than three at exactly 1080 milliseconds. An identity in bucket two switched one nanosecond late. Interpolation now uses overflow-safe integer multiplication and division, preserving endpoint clamps and signed truncation. Regressions cover the public evaluation path, increasing and decreasing thresholds, unchanged percentages, nanosecond intervals, and the maximum supported duration. The original reproduction now passes with its revision unchanged.

Large unsigned attributes passed Target validation but silently made native numeric conditions ineligible because flag-value normalization rejected them. Condition input normalization now retains full-range uint64 targeting values, including nested arrays and objects, and exact comparison supports both operand orders. Public flag values and configured comparison literals retain their existing signed range. The original MaxInt64-plus-one and MaxUint64 probes now match their numeric rules without changing source encoding or revision hashing.

A pre-existing uint constant in the value-bound tests prevented 32-bit test compilation. The test now selects the platform's maximum unsigned value without an overflowing constant conversion. Linux/386 library and test-binary cross-compilation passed; no 32-bit runtime execution is claimed.

Public signatures, persisted document formats, stable rollout buckets, dependencies, and Go minimums are unchanged. Runtime corrections affect unsigned-target eligibility and exact progressive transition instants. No additional security or ownership defect was confirmed.

The final local gate passed 351 unit cases, all 12 programs, and 1834/1834 runtime statements. All 160 separate integration cases passed with race detection. Race checks, vet, tidy parity, module inventory, and generated-document parity passed. Native Go 1.24.13 race tests passed. Independent progressive, schema 1, and split checks passed 20 repetitions, and both original numeric probes passed after the fixes.

## Property and evaluator-ownership review

Four additional independent reviews examined generated definitions, evaluation properties, real SDK behavior, and input/security boundaries. Persistence probes generated 40,000 accepted schema 2 documents with typed zeroes, variants, default/off actions, splits, schedules, conditions, and nested JSON. Every accepted document preserved canonical bytes through encoding, decoding, and memory-source loading. Separate structural mutations preserved stable round trips for all accepted inputs. Neither persistence nor boundary review found a new defect.

Evaluation review reproduced two issues. A supported time.Time leaf made an entire mixed target array ineligible for contains, even when another element matched. The same normalization prevented negative comparisons on time-containing values. Target comparison normalization now preserves those leaves without adding time literals to persisted flag definitions or coercing dates to strings or numbers. Regressions exercise Client, View, and scoped facade paths, mixed arrays, nested objects, negative comparisons, and nonnumeric operations.

The facade also passed its only preserved object fallback copy to custom evaluators. A mutation followed by an error or invalid result changed the fallback returned to the caller. Object evaluation now supplies a separate argument copy while retaining an independent failure value. Tests cover map and array roots, nested mutations, scoped and unscoped calls, errors, invalid successful results, and two-way ownership after return. Scalar paths are unchanged.

Both fixes passed independent re-review and their original external reproductions. A real isolated OpenFeature SDK probe passed 20 race-enabled repetitions for mixed time contexts and a hook that mutates and rejects object fallback evaluation. Public signatures, configured value domains, persisted formats, revisions, dependencies, and Go minimums are unchanged.

The final local gate passed 378 unit cases, all 12 programs, and 1841/1841 runtime statements. All 160 separate integration cases passed with race detection. Race checks, vet, tidy parity, module inventory, generated-document parity, and the native Go 1.24.13 race suite passed. Independent targeted regressions also passed 10 race-enabled repetitions.

## Production hardening and closure

The maintainer requested continued review until the standalone library met concrete production-candidate gates. The [readiness assessment](production-readiness.md) separates source acceptance from framework adoption, hosted verification, and publication.

Performance review measured repeated structured normalization in each condition: a valid 64-rule, 32-condition configuration allocated 71 MB per read. Matching now uses an evaluation-local structured-value cache, including cached validation failures, without changing scalar normalization. Independent matched-fixture measurements show approximately sevenfold lower latency and 99.90% fewer allocated bytes at the largest tested composition. Typical scalar allocation counts are unchanged. The [method, harness, and raw measurements](performance.md) are retained.

Security review reproduced regex compilation amplification outside the previous text-byte bounds. Follow-up closure caught Unicode backing capacity and anchored optimizer copies. Full-document preflight now checks conservative instruction and rune/dispatch storage estimates before compilation. Accepted immutable programs are reused and discarded when their condition operator or text changes. Rejected startup documents fail safely; rejected refreshes retain the last valid snapshot. These are necessary configuration-validation restrictions, documented with migration guidance in the contract. Valid accepted encoding and revision contracts are unchanged.

Independent estimator properties covered the actual regex compiler, not only its syntax program, on minimum and current Go versions. The original expansion, Unicode-table, hidden-capacity, and anchored-repeat counterexamples reject. Two fresh closure rounds found no additional actionable issue within the documented supported usage. Separate properties covered nested numeric/time targeting, typed values, composed definitions, source round trips, snapshots, and exact rollouts.

The candidate passed 418 unit cases, 12 programs, and 1966/1966 runtime statements. All 160 separate integration cases passed with race detection, plus five shuffled full-suite repetitions after runtime fixes. Independent multi-domain SDK/outage/subscription stress passed 30 repetitions, and minimum-version, module, documentation, and vulnerability checks passed. Both adapters now require hardened root commit 2fe3a029014f. Independent native-only, each-adapter-only, and combined consumers passed with no local replacements and verified the regex rejection behavior. The [acceptance assessment](production-readiness.md) records the supported production boundary.

## Distribution inventory

Corrected contract text that still described implemented SDK adapters as future work. Canonical programs remain the only executable example source, documentation generation runs twice for parity, and the native root still has no vendor dependencies. Native, LaunchDarkly, and OpenFeature minimums remain Go 1.24, 1.25, and 1.26 respectively.

At the original prepublication review there were no release tags or scripts. The current planner and publication state are recorded in [release readiness](release-readiness.md). The runtime-module inventory remains root plus the two drivers; examples and integration are tooling modules. Tags independently publish each runtime module:

| Module | Tag shape |
| :--- | :--- |
| github.com/goforj/feature | vX.Y.Z |
| github.com/goforj/feature/driver/launchdarklyfeature | driver/launchdarklyfeature/vX.Y.Z |
| github.com/goforj/feature/driver/openfeaturefeature | driver/openfeaturefeature/vX.Y.Z |

A root tag does not publish either driver. Before a versioned release, select intended root requirements for each driver, preserve development replacements, and repeat independent and combined consumer tests with GOWORK=off and no replacements against actual published versions. Fresh private Git consumers passed against the reviewed runtime commit, including a real OpenFeature provider shutdown-error regression also retained in the unit suite. Exact versions and toolchains are recorded in [verification](verification.md). Those original pending gates are tracked in the current [roadmap](roadmap.md) and [release record](release-readiness.md); subsequent public-source and real-App checks are recorded there.

## Renewed production boundary review

A further independent review covered native decision composition, SDK lifecycle and forwarding, HTTP resources, and the verification pipeline. It found a notification-boundary mismatch: a complete replacement of 1024 flags with 1024 different flags produces 2048 changed keys, while HTTPExporter.Notify accepted at most 1024. Publication succeeded, but the native HTTP notifier recorded ErrInvalidValue and omitted the webhook.

The exporter now accepts the bounded union of two valid inventories, up to 2*MaxFlags. The regression failed against the preceding implementation, then passed ten race-enabled repetitions. It exercises native publication through a real local HTTP endpoint, verifies every added and removed key and both revisions, and rejects larger lists before delivery. An independent external public-API reproduction confirmed delivery and a cleared LastNotifyError after the fix. This corrects runtime notification behavior without changing API signatures, configuration, persisted documents, or minimum Go versions.

The fresh driver review found no actionable issue under the documented SDK contracts. Both adapter modules passed ten shuffled race-enabled repetitions. The unchanged starting candidate also passed three complete shuffled race-enabled container integration repetitions with seed 972611. Subsequent candidate checks and closure review are recorded in the current execution evidence and readiness assessment.

The final corrected candidate passed 419 unit cases, all 12 executable programs, exact 1966/1966 runtime statements, all 160 separate race-enabled integration cases, full module race tests, vet, tidy, module inventory, and generated parity. It also passed the root Go 1.24.13 race suite. An independent native property probe passed 31,680 composed decisions. Two independent notification closure checks found no further actionable issue; one also ran three shuffled repetitions of the full root race suite. Fresh private Git consumers verified the corrected root and adapter requirements without local replacements, including actual HTTP delivery at the 2048-key boundary. Exact versions appear in [verification](verification.md).
