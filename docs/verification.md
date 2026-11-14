# Verification record

The latest [multi-pass review](review.md) records reproduced lifecycle fixes, coverage-gate hardening, and compatibility decisions. The [v0.1.0 release record](release-readiness.md) records completed public publication and real-App adoption; the source-consumer sections below preserve historical verification.

The standalone library includes native memory/file/HTTP evaluation, a portable typed facade, optional direct LaunchDarkly and OpenFeature SDK adapters, and a native OpenFeature provider. Typed targeting, variants, weighted splits, scheduled/progressive rollouts, and observable event delivery are implemented. Generated framework wiring and hosted account verification remain separate work.

## Executed checks

The runtime unit gate requires exact **100% statement coverage** across the root and both driver modules. It runs all thirteen canonical programs, checks their output, and collects subprocess coverage. Integration execution never supplies missing unit coverage. [Unit evidence](unit-results.json) records the current covered and total statement counts, source fingerprint, toolchain, and actual case count.

The [integration evidence](integration-results.json) records the current passing cases with race detection. Real, digest-pinned GO Feature Flag and official LaunchDarkly Relay containers serve the SDKs. Tests verify both running versions, a ten-case boolean contract across five configurations and an eight-case typed contract across six configurations, real container pause/resume, rejected credentials and initialization, typed attributes, malformed HTTP success, in-flight cancellation, updates, deletion and recreation, and shutdown. Successful false updates are checked with an opposing fallback and no evaluation error. File tests preserve coherent request snapshots during concurrent refresh and recover after malformed documents and deletion.

Both the direct LaunchDarkly driver and OpenFeature LaunchDarkly provider run their shared contract through the official Relay container. Its upstream consumes native FDv2 polling and streaming fixtures; analytics pass through the container to a local collector. Tests cover authentication, configured off-variations, owned-close forwarding, and actual Relay pause/resume. Direct SDK tests additionally verify malformed startup responses, cancellation, disconnected streams, and borrowed ownership. The hosted data plane is emulated locally, so these checks do not establish hosted LaunchDarkly account verification.

`make test-race vet tidy modules-check test-tools generate check-generated` passed. Tidy checks all five modules; vet includes integration-tagged code. Documentation generation checks two-pass byte parity. `govulncheck` v1.9.0 on Go 1.27.4 found no vulnerabilities in any of the three runtime packages. The scan does not certify every test dependency or container image.

## Production candidate Git consumers

The hardened native runtime is `v0.0.0-20261112220916-2fe3a029014f`. Both adapters at `v0.0.0-20261112221100-d1154b28f5d5` require that runtime. Examples and integration pins were advanced to the same hardened source while preserving development replacements.

Four fresh external consumers installed those actual Git versions with GOWORK=off, no local replacements, and normal checksum verification for public dependencies:

| Consumer | Executed Go | Selected root |
| :--- | :--- | :--- |
| Native only | 1.24.13 | `2fe3a029014f` |
| LaunchDarkly only | 1.25.5 | Adapter's declared `2fe3a029014f` requirement |
| OpenFeature only | 1.26.0 | Adapter's declared `2fe3a029014f` requirement |
| Native and both drivers | 1.27.4 | `2fe3a029014f` |

All consumers executed their native/SDK workflows with race detection. Each also rejected the original regex expansion counterexample and resolved the new pattern-limit API. Adapter-only consumers did not force a root upgrade, so the declared minimum itself provides the protection. Private Git authentication was process-local. These checks establish the current private installation model, not public-proxy availability or a versioned release.

The [production-readiness assessment](production-readiness.md) records the source, stress, review, performance, and operational acceptance gates. The earlier installation records below remain historical evidence for their stated commits.

## Expanded clean Git consumers

Four independent consumers installed the expanded library outside the checkout using actual Git pseudo-versions, with `GOWORK=off`, no local replacements, and race detection. Each exercised the APIs rather than only compiling an import.

| Consumer | Executed toolchain | Selected runtime modules |
| :--- | :--- | :--- |
| Native only | Go 1.24.13 | Root `v0.0.0-20261112111010-76d41d886154` |
| LaunchDarkly only | Go 1.25.5 | Driver `v0.0.0-20261112111340-ad4de13b8a00`, declared root `76d41d886154` |
| OpenFeature only | Go 1.26.0 | Driver `v0.0.0-20261112111340-ad4de13b8a00`, declared root `76d41d886154` |
| Native and both drivers | Go 1.27.4 | Both drivers `ad4de13b8a00`, root `76d41d886154` |

The native consumer checked typed values, encoding/decoding, conditions, named variants, captured time, refresh, snapshots, and custom tracking. The LaunchDarkly consumer used the actual SDK test-data source for all four additional value types and event submission. The OpenFeature consumer used the ordinary SDK with the native provider, including an integer beyond float64's exact range, tracking, and borrowed-engine ownership. The combined consumer exercised all three workflows together. Adapter-only consumers selected their declared root requirement without forcing an upgrade.

Selected module metadata contained no replacements. Public dependencies used normal checksum verification; the private first-party repository used authenticated Git and `GOPRIVATE=github.com/goforj/feature`. These checks establish installation under the current private Git access model, not public-proxy availability or a versioned release. The eventual release must repeat installation against its independently published module versions.

## Historical clean Git consumers

These checks apply to the earlier boolean candidate. Independent consumers installed reviewed source commit `8e7a76c0f17b6e6e0e8cb3042a19de2f231ae9fa` using actual Go pseudo-versions outside the checkout, with `GOWORK=off` and no replacements. Each consumer performed a successful boolean evaluation with race detection. Native and combined consumers checked validation precedence, revisions, and counters. LaunchDarkly and combined consumers exercised invalid TLS configuration and cancellation from an actual SDK component factory. OpenFeature and combined consumers verified that a real provider cleanup error survives repeated Close calls:

| Consumer | Executed toolchain | Selected native root |
| :--- | :--- | :--- |
| Native only | Go 1.24.13 | Candidate commit |
| LaunchDarkly only | Go 1.25.5 | Adapter's declared `331a16e2f707` requirement |
| OpenFeature only | Go 1.26.0 | Adapter's declared `331a16e2f707` requirement |
| Native and both drivers | Go 1.27.4 | Candidate commit |

The adapter-only checks did not force-upgrade the root, so they also validate each adapter's declared minimum root requirement. Selected module metadata contained no replacements. Public dependencies used normal checksum verification. At that historical check, the first-party repository was private: installation used Git authentication and `GOPRIVATE=github.com/goforj/feature`. This does not establish public-proxy availability or a versioned release.

## Review findings

- Compatibility: native Subject retains string attributes. Source, snapshot, schema 1, stable rollout encoding, and revision behavior remain intact. Typed targeting uses Target; schema 2 adds typed native conditions and value models. Flag.Default and Rule.Value now accept any supported type as part of the unreleased API expansion. Vendor dependencies and higher Go minimums stay in optional modules. The combined example catalog uses Go 1.26 without raising the native minimum.
- Provider semantics: Source.Load remains a complete native-document boundary. Adapters evaluate through actual SDKs, preserving rules, hooks, context merging, and configured off-variations. A targeting scope does not claim a frozen vendor revision. Provider rollout assignments and freshness are not portable guarantees.
- Ownership and resources: scopes clone bounded target data. SDK constructors explicitly borrow or own. Startup failures clean up owned resources. Integration fixtures own services once per package run, clean partial starts, and fail rather than skip missing Docker.
- Failure safety: structured errors preserve explicit fallbacks and safe classifications rather than upstream diagnostics. Refresh retains valid native snapshots. Outages and malformed responses are tested separately from missing flags.
- Documentation: SDK constructors have executable examples and generated API-index destinations. Generator tests reject unrelated constructors sharing the same name. Unit and integration reports remain separate and tied to tested source.

At that historical check, hosted LaunchDarkly verification, real GoForj App adoption, framework integration, external Codecov setup, public distribution, and releases were pending. The current [roadmap](roadmap.md) and [release record](release-readiness.md) distinguish completed adoption/publication from remaining work.

## Renewed notification-boundary consumer verification

After the independent review found and fixed the complete-inventory notification limit, four fresh private Git consumers passed with GOWORK=off and no local replacements:

| Consumer | Executed toolchain | Selected root | Selected adapters |
| :--- | :--- | :--- | :--- |
| Native only | Go 1.24.13 | v0.0.0-20261112233624-815737cc74c4 | None |
| LaunchDarkly only | Go 1.25.5 | v0.0.0-20261112233624-815737cc74c4 | v0.0.0-20261112233735-6d068f9d7e3f |
| OpenFeature only | Go 1.26.0 | v0.0.0-20261112233624-815737cc74c4 | v0.0.0-20261112233735-6d068f9d7e3f |
| Native and both drivers | Go 1.27.4 | v0.0.0-20261112233624-815737cc74c4 | v0.0.0-20261112233735-6d068f9d7e3f |

Every consumer delivered a complete 2048-key notification to a real local HTTP endpoint, rejected a larger key list, and rejected the original regex expansion counterexample. Adapter-only consumers selected the corrected root through their declared requirements without forcing a root upgrade. Existing typed native, official LaunchDarkly SDK, and native OpenFeature SDK round-trip checks also passed with race detection. These are private source-commit installations, not versioned releases or hosted-account verification.

The corrected source passed 419 unit cases, all 12 executable programs, exact 1966/1966 runtime unit statement coverage, all 160 separate race-enabled integration cases, full module race tests, vet, tidy, module inventory, generated parity, and its root Go 1.24.13 race suite. Independent native probing passed 31,680 composed decisions, each adapter suite passed ten shuffled race repetitions, and notification closure review passed three shuffled full-root race repetitions. No actionable issue remained after the two independent notification closure checks.

## API simplicity consumer verification

The compatible API adjustment adds Features.Snapshot, ObjectAs, and Validate. Four fresh private Git consumers passed with GOWORK=off, race detection, and no local replacements:

| Consumer | Executed toolchain | Selected root | Selected adapters |
| :--- | :--- | :--- | :--- |
| Native only | Go 1.24.13 | v0.0.0-20261113224422-3e7b49d5eec5 | None |
| LaunchDarkly only | Go 1.25.5 | v0.0.0-20261113224422-3e7b49d5eec5 | v0.0.0-20261113224729-b1e08a818877 |
| OpenFeature only | Go 1.26.0 | v0.0.0-20261113224422-3e7b49d5eec5 | v0.0.0-20261113224729-b1e08a818877 |
| Native and both drivers | Go 1.27.4 | v0.0.0-20261113224422-3e7b49d5eec5 | v0.0.0-20261113224729-b1e08a818877 |

Adapter-only consumers selected their declared minimum root without forcing an upgrade. Each exercised owned application-struct decoding, decision revisions, live versus captured values, retained observer and tracker policy, and explicit validation paths with redacted error text. Existing regex-expansion and complete-inventory HTTP webhook checks also passed. Prior official SDK and typed engine checks remained in the consumers. These checks establish authenticated source-commit installation under the current private access model, not a versioned release or hosted-account verification.

The candidate passed 444 unit cases, all 13 executable programs, exact 2088/2088 runtime statements, all 172 separate race-enabled real-service integration cases, full module race checks, vet, tidy, module inventory, and generated parity. The new API regression suite passed ten shuffled race repetitions with seed 139277. The full root Go 1.24.13 race suite also passed. Unit and integration evidence was regenerated after module pin changes, with intentional development replacements preserved.
