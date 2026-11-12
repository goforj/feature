# Remaining work

The standalone first implementation is built and pushed. The broader GoForj feature is still in progress. Green tests establish the implemented contract; they do not complete the review, adoption, integration, or release work below.

## Implemented

- [x] Boolean evaluation with explicit fallbacks and decision reasons.
- [x] Exact identity and attribute targeting, ordered rules, and stable percentage rollouts.
- [x] Immutable request/job views and atomic complete-document publication.
- [x] Memory and JSON file sources with explicit refresh and an owned polling lifecycle.
- [x] Last-valid snapshot retention, source error redaction, and document bounds.
- [x] Executable examples, generated documentation, Go maintenance tooling, and CI/Security workflows.
- [x] Exact full runtime-library statement coverage, race checks, minimum Go validation, and decoder fuzzing.

## Library review before freezing v1

- [ ] Review the consumer API separately from its implementation: construction, source ownership, fallbacks, cancellation, and request/job snapshot use.
- [ ] Review correctness and resources: concurrent publication, failed refreshes, stale state, JSON validation, aggregate limits, and error redaction.
- [ ] Confirm that the native evaluator's narrow scope is the intended product boundary, including what it offers compared with integrating an existing engine.
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

Multivariate flags, experiments, exposure analytics, browser SDKs, and OpenFeature adapters remain deferred rather than prerequisites for the initial boolean library release.
