# v0.1.0 release record

The [public v0.1.0 release](https://github.com/goforj/feature/releases/tag/v0.1.0) covers the standalone native library, optional LaunchDarkly and OpenFeature modules, and the native OpenFeature provider. Generated framework integration and hosted LaunchDarkly account verification remain separate.

## Published modules

| Module | Tag | Minimum Go | Publication |
| :--- | :--- | :--- | :--- |
| github.com/goforj/feature | v0.1.0 | 1.24 | Public proxy and checksum verified |
| github.com/goforj/feature/driver/launchdarklyfeature | driver/launchdarklyfeature/v0.1.0 | 1.25 | Public proxy and checksum verified |
| github.com/goforj/feature/driver/openfeaturefeature | driver/openfeaturefeature/v0.1.0 | 1.26 | Public proxy and checksum verified |

The examples and integration modules are tooling and receive no release tags. Both drivers require root v0.1.0. Intentional relative development replacements remain in the repository; none were used by public consumers. Public module ZIPs include their licenses. Existing tags must never be moved.

## Source and application gates

The [production assessment](production-readiness.md) and [API audit](api-simplicity-audit.md) record native contracts and compatible API simplifications. The maintainer selected public distribution; GitHub visibility is public. Four fresh source consumers passed through proxy.golang.org and sum.golang.org, with no credentials, private-module bypass, workspace replacement, or rewritten root requirements.

The [application smoke](app-smoke.md) passed through app-owned Wire providers and lifecycle hooks in a real rendered GoForj HTTP App. Repeated against published v0.1.0, [direct execution](releases/v0.1.0-app.json) retained App PID 1731, and [native forj dev](releases/v0.1.0-dev.json) retained App PID 4812 through valid flag updates, invalid JSON, file deletion, and recovery. A held request retained its original values and revision across an update. Rerender preserved app-owned code, JSON edits did not restart the App under conventional Go/environment watchers, and shutdown joined the feature refresh worker. The tested renderer was d6ec51576d5f9dc6f74bc37f9907367a6280689c with only unrelated documentation/agent-guidance worktree changes; the App used Go 1.27.4. The source-consumer rendered App's complete unit/race suite also passed with an isolated test environment. Library minimum-Go verification is independent of that application composition.

The release candidate passed 447 unit cases, 13 executable programs, exact 2088/2088 runtime unit statements, 172 separate race-enabled container integration cases, module race tests, vet, tidy/inventory, and generated parity. Tooling tests also verify that public consumers cannot inherit private/workspace bypasses and that changes to embedded verification probes invalidate source evidence.

## Publication evidence

The root's annotated remote tag peels to e1ccf5b5271a15266eb87eb1fb6591d763f806ec. Its exact-commit [CI](https://github.com/goforj/feature/actions/runs/43534304963) and [Security](https://github.com/goforj/feature/actions/runs/43534305511) passed before publication. Four fresh public source consumers passed for that candidate. The root tag was published and verified independently before updating either driver's requirement.

Both annotated driver tags peel to cf17c7d0dad156379e980401b6f4407cf17b5b2a. Its exact-commit [CI](https://github.com/goforj/feature/actions/runs/43534977416) and [Security](https://github.com/goforj/feature/actions/runs/43534977454) passed before publication, as did independent public driver-candidate consumers and the combined graph. Remote peeled targets were checked after each tag push.

The [versioned consumer report](releases/v0.1.0-consumers.json) records executed compilers, selected module versions, ZIP checksums, and go.mod checksums. Native, LaunchDarkly-only, OpenFeature-only, and combined consumers all passed with race detection, GOWORK=off, a fresh module cache, proxy.golang.org and sum.golang.org, and no local replacements or private-module bypass. Adapter-only consumers selected root v0.1.0 through their own declared requirements. The GitHub release was created from the existing verified root tag.

The release-triggered [public-consumer workflow](https://github.com/goforj/feature/actions/runs/43535338004) repeats the complete verification independently on Linux and records module checksums in its job summary. Later main-branch documentation commits do not change any published tag target.

## Reproduce

```sh
make release-plan VERSION=v0.1.0
make verify-release VERSION=v0.1.0
make smoke-app VERSION=v0.1.0
make smoke-app VERSION=v0.1.0 SMOKE_ARGS=-dev
```

The release notes are maintained in [releases/v0.1.0.md](releases/v0.1.0.md). This release introduces no source/API, configuration, persisted-data, operational-migration, or minimum-Go break from the audited candidate. The v0.1 API and documented native schema/rollout contracts form the initial released baseline.
