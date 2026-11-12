# Remaining work

The standalone first implementation is built and pushed. The broader GoForj feature is still in progress. Green tests establish the implemented contract; they do not complete the review, adoption, integration, or release work below.

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
- [x] Inventory all five modules, minimum Go requirements, dependency pins, independent tag shapes, and clean private Git consumers. Preserve the root's Go 1.24 minimum for native-only users. Published-version and public-distribution checks remain release gates.

## Library review before freezing v1

- [x] Review the consumer API separately from its implementation: construction, source ownership, fallbacks, cancellation, and request/job snapshot use.
- [x] Review correctness and resources: concurrent publication, failed refreshes, stale state, JSON validation, aggregate limits, and error redaction.
- [x] Review the native engine and external drivers against the provider-neutral product boundary, including whether the facade improves on using an SDK directly.
- [x] Review schema 1, rollout encoding, and revision normalization as compatibility contracts before publishing them.
- [x] Reproduce and fix concrete findings, add regression coverage, and rerun affected checks. The [verification record](verification.md) records findings, executed checks, consumer evidence, and limits.

The latest [multi-pass review](review.md) records reproduced findings and their resolution. Existing coverage and green CI do not stand in for this review.

## Real-App adoption

- [ ] Inject the client into a real GoForj App service and use a meaningful feature decision.
- [ ] Demonstrate a file-backed decision changing while the App PID remains unchanged.
- [ ] Demonstrate several related decisions using one captured view during an update.
- [ ] Exercise invalid input, file deletion, recovery, and refresh-loop shutdown in the running App.
- [ ] Check the developer workflow, including whether `forj dev` rebuilds or restarts when the selected flag document changes.

Completion means a reproducible App example proves the behavior and identifies any API or lifecycle changes needed before framework integration.

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

- [ ] Resolve review and real-App findings, then freeze the initial public API and document contract.
- [ ] Confirm the intended distribution and access model. The repository is currently private; do not assume public-proxy availability.
- [x] Verify independent native and SDK consumers plus their combined composition with `GOWORK=off`, normal public dependency checksums, and no local replacements under the current private Git access model. Repeat for the eventual release and public distribution model if selected.
- [ ] Decide whether to enable Codecov, then configure and verify its uploads and checks if selected. Local exact coverage enforcement already exists.
- [ ] Prepare release notes, select an initial version, and verify CI/Security for the final candidate.
- [ ] Publish the authorized version and verify installation of the actual published module. Update the unreleased badge and installation guidance afterward.

## Later, separately scoped

- [ ] Read-only Lighthouse visibility into each running process's adopted revision, snapshot age, and refresh status.
- [ ] Additional dedicated remote connectors when applications require more than the implemented HTTP source and custom source boundary.
- [ ] Administration commands or UI, with explicit authorization, persistence, and audit behavior.

Statistical experiment analysis, browser SDKs, and additional dedicated vendor drivers remain deferred. Weighted allocation, custom tracking, and evaluation observability are implemented. Typed flag values, native OpenFeature provider support, typed targeting, named variants, scheduled/progressive rules, HTTP sources, and observability primitives are included in the current library build. Broader backend-specific administration and statistical analysis remain separate.
