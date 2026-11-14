# Remaining work

The standalone [production-readiness assessment](production-readiness.md) records the current library candidate gates separately from framework integration and publication.

The standalone library has a coordinated [public v0.1.0 release](https://github.com/goforj/feature/releases/tag/v0.1.0). Review, real-App adoption, and public installation passed as recorded in the [release record](release-readiness.md). The broader generated GoForj component remains separate.

The [provider study](provider-study.md) informed a provider-neutral client with native, direct LaunchDarkly, and OpenFeature drivers. The native engine, portable facade, SDK adapters, and cross-driver local integration suite are implemented. Hosted LaunchDarkly account verification remains pending and is distinct from the tested local SDK transports.

## Implemented

- [x] Boolean, string, integer, float, and object evaluation with explicit fallbacks and decision reasons.
- [x] Native OpenFeature provider with typed evaluation, lifecycle ownership, change events, and tracking integration.
- [x] Typed conditions, named variants, weighted splits, scheduled windows, and progressive native rollouts.
- [x] HTTP retrieval, custom source adapters, observers, analytics sinks, and webhook notifications.
- [x] Exact identity and attribute targeting, ordered rules, and stable percentage rollouts.
- [x] Immutable request/job views and atomic complete-document publication.
- [x] Memory, JSON file, HTTP, and custom sources with explicit refresh and a caller-owned polling lifecycle.
- [x] Last-valid snapshot retention, source error redaction, and document bounds.
- [x] Executable examples, generated documentation, Go maintenance tooling, and CI/Security workflows.
- [x] Exact full runtime-library statement coverage, race checks, minimum Go validation, and decoder fuzzing.

## Portable driver design and implementation before v1

- [x] Study OpenFeature, LaunchDarkly, GO Feature Flag, Unleash, and ConfigCat using official sources and pinned SDK probes. Retain [findings and reproducible evidence](provider-study.md).
- [x] Compare realistic consumer code for setup, identity, simple checks, fallbacks, details, test data, and shutdown. Canonical native and SDK examples execute under one output-checking runner.
- [x] Define a portable evaluation boundary that keeps vendor SDKs authoritative rather than converting their rules into native Documents.
- [x] Decide typed subject attributes, multi-identity mapping, snapshot capabilities, error/reason mapping, and readiness semantics before freezing their public representation.
- [x] Preserve existing native behavior or document concrete migration requirements for any API changes.
- [x] Implement direct LaunchDarkly and OpenFeature drivers in optional modules, with explicit borrowed versus owned SDK lifecycles.
- [x] Add a required shared driver conformance suite plus real SDK tests for updates, failures, context mapping, ownership, cancellation, and concurrent use. The [integration report](integration-results.json) records executed local service and SDK transport evidence.
- [x] Verify OpenFeature hooks/context precedence and native LaunchDarkly behavior; keep unsupported capabilities explicit.
- [x] Inventory all five modules, minimum Go requirements, dependency pins, independent tag shapes, and clean consumers. Preserve the root's Go 1.24 minimum for native-only users. Independently tagged v0.1.0 modules passed public installation checks.

## Library review before freezing v1

- [ ] Revisit OpenFeature cancellation cleanup ordering when its SDK exposes joinable initialization or shutdown. Until then, stateful providers must synchronize their lifecycle methods, or applications must manage SDK startup and borrow an initialized client.
- [ ] Revisit OpenFeature initialization event ordering when its SDK can preserve health events concurrent with registration. Until then, finish registration before starting native refresh and serialize later registrations with Refresh.

- [x] Review the consumer API separately from its implementation: construction, source ownership, fallbacks, cancellation, and request/job snapshot use.
- [x] Review correctness and resources: concurrent publication, failed refreshes, stale state, JSON validation, aggregate limits, and error redaction.
- [x] Review the native engine and external drivers against the provider-neutral product boundary, including whether the facade improves on using an SDK directly.
- [x] Review schema 1, rollout encoding, and revision normalization as compatibility contracts before publishing them.
- [x] Reproduce and fix concrete findings, add regression coverage, and rerun affected checks. The [verification record](verification.md) records findings, executed checks, consumer evidence, and limits.

The latest [multi-pass review](review.md) records reproduced findings and their resolution. Existing coverage and green CI do not stand in for this review.

## Real-App adoption

- [x] Inject the client into a real GoForj App service and use a meaningful feature decision.
- [x] Demonstrate a file-backed decision changing while the App PID remains unchanged.
- [x] Demonstrate several related decisions using one captured view during an update.
- [x] Exercise invalid input, file deletion, recovery, and refresh-loop shutdown in the running App.
- [x] Check the developer workflow, including whether `forj dev` rebuilds or restarts when the selected flag document changes.

The [reproducible App smoke](app-smoke.md) passed direct and native development lifecycles, including unchanged process IDs, captured-request consistency, invalid/deleted file retention, recovery, rerender preservation, and worker shutdown. It uses app-owned wiring and does not add a generated Feature component.

## GoForj integration

This work belongs in `goforj`, with public guidance in `goforj-docs`.

- [ ] Define and implement the optional Feature component and startup source configuration.
- [ ] Add generated providers and explicit constructor injection into App composition.
- [ ] Wire refresh into HTTP, worker, scheduler, and aggregate runtime lifecycle, with one loop per shared client.
- [ ] Keep short-lived commands load-once and ensure disabled components emit no Feature dependencies or wiring.
- [ ] Support additional Apps and preserve user-owned composition files through the normal upgrade path.
- [ ] Teach `forj dev` to distinguish runtime flag document changes from `.env` and source changes.
- [ ] Validate all affected modules and pins, fresh renders, upgrade behavior, and the largest supported composition. Run renders under `/tmp`.
- [ ] Add framework documentation and a generated-App test proving live updates without PID changes.

## Release and distribution

- [x] Resolve review and real-App findings and freeze the initial v0.1 API and document contract.
- [x] Confirm public distribution. GitHub visibility is public, the maintainer selected a public release, and normal proxy/checksum source consumers passed.
- [x] Verify independent native and SDK consumers plus their combined composition with `GOWORK=off`, normal public dependency checksums, and no local replacements. Repeated successfully against all actual public v0.1.0 module tags.
- [x] Keep Codecov uploads optional and enforce exact runtime unit coverage independently in CI. This release does not change external coverage-account configuration.
- [x] Prepare release notes, select v0.1.0, and verify CI/Security for each exact tagged candidate.
- [x] Publish root and both driver modules, verify independent and combined public installation, and create the coordinated GitHub release. Installation guidance and the generated version badge use v0.1.0.

## Later, separately scoped

- [ ] Read-only Lighthouse visibility into each running process's adopted revision, snapshot age, and refresh status.
- [ ] Additional dedicated remote connectors when applications require more than the implemented HTTP source and custom source boundary.
- [ ] Administration commands or UI, with explicit authorization, persistence, and audit behavior.

Statistical experiment analysis, browser SDKs, and additional dedicated vendor drivers remain deferred. Weighted allocation, custom tracking, and evaluation observability are implemented. Typed flag values, native OpenFeature provider support, typed targeting, named variants, scheduled/progressive rules, HTTP sources, and observability primitives are included in the current library build. Broader backend-specific administration and statistical analysis remain separate.
