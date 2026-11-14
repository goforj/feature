# v0.1.0 release record

The initial public release covers the standalone native library, optional LaunchDarkly and OpenFeature modules, and the native OpenFeature provider. Generated framework integration and hosted LaunchDarkly account verification remain separate.

## Module plan

| Module | Tag | Minimum Go | Publication |
| :--- | :--- | :--- | :--- |
| github.com/goforj/feature | v0.1.0 | 1.24 | Pending candidate CI |
| github.com/goforj/feature/driver/launchdarklyfeature | driver/launchdarklyfeature/v0.1.0 | 1.25 | Publish after root verification and requirement update |
| github.com/goforj/feature/driver/openfeaturefeature | driver/openfeaturefeature/v0.1.0 | 1.26 | Publish after root verification and requirement update |

The examples and integration modules are tooling and receive no release tags. Root publication, driver publication, and a coordinated GitHub release are separate operations. Existing tags must never be moved.

## Source and application gates

The [production assessment](production-readiness.md) and [API audit](api-simplicity-audit.md) record native contracts and compatible API simplifications. The maintainer selected public distribution; GitHub visibility is public. Four fresh source consumers passed through proxy.golang.org and sum.golang.org, with no credentials, private-module bypass, workspace replacement, or rewritten root requirements.

The [application smoke](app-smoke.md) passed through app-owned Wire providers and lifecycle hooks in a real rendered GoForj HTTP App. Direct execution retained App PID 66290, and native forj dev retained App PID 27326 through valid flag updates, invalid JSON, file deletion, and recovery. A held request retained its original values and revision across an update. Rerender preserved app-owned code, JSON edits did not restart the App under conventional Go/environment watchers, and shutdown joined the feature refresh worker. The tested renderer was d6ec51576d5f9dc6f74bc37f9907367a6280689c with only unrelated documentation/agent-guidance worktree changes; the App used Go 1.27.4. The rendered App's complete unit/race suite also passed with an isolated test environment. Library minimum-Go verification is independent of that application composition.

The release candidate passed 447 unit cases, 13 executable programs, exact 2088/2088 runtime unit statements, 172 separate race-enabled container integration cases, module race tests, vet, tidy/inventory, and generated parity. Tooling tests also verify that public consumers cannot inherit private/workspace bypasses and that changes to embedded verification probes invalidate source evidence.

## Publication gates

Before tags: commit the candidate, rerun required source checks and generated parity, observe CI/Security for that exact commit, verify public source consumers, and preflight all three tag names.

Publish and verify root v0.1.0 first. Then update both drivers to require the published root, preserve relative development replacements, and observe CI/Security for the driver candidate before publishing its tags. Finally run the complete public versioned consumer matrix in a new cache, inspect remote peeled tag targets, and create the coordinated GitHub release from the existing root tag. Versioned publication evidence will replace this pending plan after those checks pass.

## Reproduce

```sh
make release-plan VERSION=v0.1.0
make verify-release VERSION=v0.1.0
make smoke-app VERSION=v0.1.0
make smoke-app VERSION=v0.1.0 SMOKE_ARGS=-dev
```

The release notes are maintained in [releases/v0.1.0.md](releases/v0.1.0.md). This release introduces no source/API, configuration, persisted-data, operational-migration, or minimum-Go break from the audited candidate. The v0.1 API and documented native schema/rollout contracts form the initial released baseline.
