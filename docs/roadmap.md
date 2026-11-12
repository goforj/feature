# Remaining work

The standalone first implementation is built and pushed. The broader GoForj feature is still in progress. Green tests establish the implemented contract; they do not complete the review, adoption, integration, or release work below.

The [provider study](provider-study.md) proposes a broader completion target: a provider-neutral client with native, direct LaunchDarkly, and OpenFeature drivers. Only the native engine is implemented today.

## Implemented

- [x] Boolean evaluation with explicit fallbacks and decision reasons.
- [x] Exact identity and attribute targeting, ordered rules, and stable percentage rollouts.
- [x] Immutable request/job views and atomic complete-document publication.
- [x] Memory and JSON file sources with explicit refresh and an owned polling lifecycle.
- [x] Last-valid snapshot retention, source error redaction, and document bounds.
- [x] Executable examples, generated documentation, Go maintenance tooling, and CI/Security workflows.
- [x] Exact full runtime-library statement coverage, race checks, minimum Go validation, and decoder fuzzing.

## Portable driver design and implementation before v1

- [x] Study OpenFeature, LaunchDarkly, GO Feature Flag, Unleash, and ConfigCat using official sources and pinned SDK probes. Retain [findings and reproducible evidence](provider-study.md).
- [ ] Compare realistic consumer code for the candidate APIs, including setup, identity, simple checks, chosen fallbacks, details, test overrides, and shutdown.
- [ ] Define a portable evaluation boundary that keeps vendor SDKs authoritative rather than converting their rules into native Documents.
- [ ] Decide typed subject attributes, multi-identity mapping, snapshot capabilities, error/reason mapping, and readiness semantics before freezing their public representation.
- [ ] Preserve existing native behavior or document concrete migration requirements for any API changes.
- [ ] Implement direct LaunchDarkly and OpenFeature drivers in optional modules, with explicit borrowed versus owned SDK lifecycles.
- [ ] Add a required shared driver conformance suite plus real SDK tests for updates, failures, context mapping, ownership, cancellation, and concurrent use.
- [ ] Verify OpenFeature hooks/context precedence and native LaunchDarkly behavior; keep unsupported capabilities explicit.
- [ ] Inventory every new module, minimum Go requirement, dependency pin, release tag, and public consumer check independently. Preserve the root's Go 1.24 minimum for native-only users.

## Library review before freezing v1

- [ ] Review the consumer API separately from its implementation: construction, source ownership, fallbacks, cancellation, and request/job snapshot use.
- [ ] Review correctness and resources: concurrent publication, failed refreshes, stale state, JSON validation, aggregate limits, and error redaction.
- [ ] Review the native engine and external drivers against the provider-neutral product boundary, including whether the facade improves on using an SDK directly.
- [ ] Review schema 1, rollout encoding, and revision normalization as compatibility contracts before publishing them.
- [ ] Reproduce and fix concrete findings, add regression coverage, and rerun affected checks.

Completion means the review findings and their resolution are recorded. Existing coverage and green CI do not stand in for this review.

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
- [ ] Verify an external consumer can install the exact candidate with `GOWORK=off`, normal checksums, and no local replacements under the intended access model.
- [ ] Decide whether to enable Codecov, then configure and verify its uploads and checks if selected. Local exact coverage enforcement already exists.
- [ ] Prepare release notes, select an initial version, and verify CI/Security for the final candidate.
- [ ] Publish the authorized version and verify installation of the actual published module. Update the unreleased badge and installation guidance afterward.

## Later, separately scoped

- [ ] Read-only Lighthouse visibility into each running process's adopted revision, snapshot age, and refresh status.
- [ ] Shared remote document sources when an application needs cross-host distribution.
- [ ] Administration commands or UI, with explicit authorization, persistence, and audit behavior.

Experiments, exposure analytics, browser SDKs, and additional dedicated vendor drivers remain deferred. Typed flag values need an explicit scope decision; the initial cross-driver proof is boolean evaluation. The proposed OpenFeature adapter is now part of the pre-v1 driver work above.
