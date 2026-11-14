# Standalone production readiness

This assessment covers the dependency-free native library and the optional LaunchDarkly and OpenFeature modules, used within their documented contracts. All three modules are publicly released at v0.1.0, with independent installation evidence in the [release record](release-readiness.md). Generated framework integration, hosted LaunchDarkly account verification, and complete OpenFeature specification conformance remain outside this assessment.

## Acceptance gates

- No unresolved actionable correctness, security, resource, ownership, or lifecycle finding in supported usage.
- Fixes independently reproduced and reviewed, followed by two independent closure review rounds without new actionable findings.
- Exact 100% runtime unit statement coverage, separate real-container integration coverage, and passing tests across every relevant module.
- Repeated race-enabled outage, recovery, subscription, shutdown, and integration checks with no inconsistent decisions or leaked listener ownership.
- Bounded regex expansion before compilation, including a complete-document budget.
- Measured native scalar reads without a material regression and structured comparisons without repeated full-container copies per condition.
- Passing minimum-Go checks, tidy/module inventory, executable examples, generated-document parity, vulnerability checks, and CI for the final candidate.
- Independent Git consumers using published source commits, GOWORK=off, and no local replacements. Each adapter must select a root requirement containing the production fixes.

Coverage establishes executed statements, not completeness. Review, property checks, stress tests, and measurements supply separate evidence.

## Current assessment

The production audit and renewed boundary review found and resolved three issues:

1. A 1010-byte regular expression expanded into about one million instructions and retained about 43 MB for one condition. Multiple such conditions fit the previous document budget. The fix preflights estimated expansion before compilation, caps each pattern and the complete document, and reuses immutable validated programs.
2. Structured conditions repeatedly normalized the same attribute. A supported large configuration allocated about 71 MB per evaluation. The fix owns a lazy structured comparison cache for each evaluation and preserves scalar behavior.
3. A complete replacement of two disjoint maximum-size inventories produced 2048 changed keys, but the HTTP notifier accepted only 1024. The exporter now accepts the bounded union and a native-to-HTTP regression verifies all added and removed keys.

Closure review also caught Unicode table backing and anchored one-pass optimizer storage that an instruction-only limit missed. The final estimator covers both and rejects the original counterexamples before full compilation. Independent full-compiler storage oracles passed on Go 1.24.0 and 1.27.4, including 29,641 accepted current-Go and 29,659 accepted minimum-Go patterns, with no storage underestimates. A separate final oracle covered 11,695 accepted patterns.

Two fresh closure rounds after the resource fixes found no new actionable issue in the documented supported usage. Native/API and security reviewers covered the first round; independent cross-module and adversarial boundary reviewers covered the second. Generated evaluation/persistence properties, actual SDK probes, and repeated lifecycle stress supplied separate evidence.

The local candidate passed 447 unit cases, all 13 executable programs, exact 2088/2088 runtime statement coverage, and 172 separate race-enabled integration cases. The preceding resource-hardened candidate passed five shuffled race-enabled container repetitions with package-owned services. Renewed review added three complete shuffled repetitions before the notification fix, followed by a fresh full 160-case race-enabled integration pass after the fix and again after module pin updates. Minimum Go 1.24.13, 1.25.5, and 1.26.0 race checks passed, as did Linux/386 test-binary compilation, vet, tidy parity, module inventory, and generated-document parity. Govulncheck v1.9.0 on Go 1.27.4 found no runtime-package vulnerabilities.

Independent [performance measurements](performance.md) show the representative structured case improving from 121.502 us and 222,424 bytes to 17.713 us and 7,680 bytes. The largest supported fixture improved from 42.723 ms and 71,387,544 bytes to 5.935 ms and 70,528 bytes. Scalar allocation counts are unchanged. These are measured configurations on one host, not deployment-wide SLOs.

Source and independent-installation gates are closed. The audited runtime API originated at `3e7b49d5eec5`, with adapter requirements advanced at `b1e08a818877`. The root release v0.1.0 points to `e1ccf5b5271a`; driver releases point to `cf17c7d0dad1` and require root v0.1.0. Examples and integration pin those versions, with development replacements preserved. Four fresh public versioned consumers passed using Go 1.24.13, 1.25.5, 1.26.0, and 1.27.4, with GOWORK=off and no replacements. Each rejected the original regex expansion counterexample, delivered a complete 2048-key HTTP notification, and exercised the new snapshot, typed-object, and authoring-validation APIs, including adapter-only consumers selecting their declared minimum root. Exact versions and checksums are recorded in [release verification](release-readiness.md).

Renewed native review passed 31,680 independently composed decisions. Both adapter suites passed ten shuffled race-enabled repetitions. Two independent notification closure checks found no further actionable issue, including three shuffled repetitions of the full root race suite. The corrected root also passed its Go 1.24.13 minimum suite.

The compatible API simplification makes Features and Scope the documented application path. Features.Snapshot retains observer and tracking policy; ObjectAs decodes application structures with owned JSON fallbacks; Validate exposes explicit bounded authoring diagnostics while runtime errors remain redacted. The new regressions passed ten shuffled race-enabled repetitions. The full 172-case container suite and exact unit coverage passed again after updating module requirements. Existing native APIs, schema bytes, rollout encoding, and minimum Go versions are preserved.

This is acceptance of the standalone library for production use within the documented contracts, supported by passing CI/Security for the candidate. Any later source candidate must rerun the affected gates. Hosted-account verification and compatibility beyond the capability matrix are not claimed. Subsequent real-App adoption and public module publication are tracked separately in the [release record](release-readiness.md).

## Operational boundaries

The [contract](contract.md) and [capability matrix](capabilities.md) define supported behavior. In particular:

- Complete native documents publish atomically; failed loads retain the last valid snapshot. Applications own refresh goroutines and shutdown synchronization.
- Stateful OpenFeature providers must synchronize initialization and shutdown. The pinned SDK cannot join initialization after cancellation through its public API. Borrow an application-managed initialized SDK client when that requirement cannot be met.
- Finish native-provider SDK registration before starting refresh. Serialize later registrations with Refresh because SDK-owned initialization events can overwrite concurrent health events.
- Tracking submission does not guarantee durable delivery. Observers, notifiers, and configured sinks are synchronous; applications can supply queues when asynchronous or durable delivery is required.
- Source paths, endpoint URLs, transports, and callbacks are operator-controlled collaborators. File writes should use atomic replacement; filesystem calls cannot be forcibly interrupted.

Real-App adoption passed as recorded in the [application smoke](app-smoke.md). Framework wiring, account-specific verification, and external account setup remain separate. The [release record](release-readiness.md) tracks public versioned publication independently from this source assessment.
