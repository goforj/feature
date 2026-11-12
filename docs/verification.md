# Verification record

The latest [multi-pass review](review.md) records reproduced lifecycle fixes, coverage-gate hardening, compatibility decisions, and remaining publication work.

The standalone library includes native memory/file evaluation, a portable boolean facade, and optional direct LaunchDarkly and OpenFeature SDK adapters. Framework wiring, typed flag values, hosted account verification, and release publication remain separate work.

## Executed checks

The runtime unit gate requires exact **100% statement coverage** across the root and both driver modules. It runs all seven canonical programs, checks their output, and collects subprocess coverage. Integration execution never supplies missing unit coverage. [Unit evidence](unit-results.json) records the current covered and total statement counts, source fingerprint, toolchain, and actual case count.

The [integration evidence](integration-results.json) records the current passing cases with race detection. Real, digest-pinned GO Feature Flag and official LaunchDarkly Relay containers serve the SDKs. Tests verify both running versions, a ten-case contract across five configurations, real container pause/resume, rejected credentials and initialization, typed attributes, malformed HTTP success, in-flight cancellation, updates, deletion and recreation, and shutdown. Successful false updates are checked with an opposing fallback and no evaluation error. File tests preserve coherent request snapshots during concurrent refresh and recover after malformed documents and deletion.

Both the direct LaunchDarkly driver and OpenFeature LaunchDarkly provider run their shared contract through the official Relay container. Its upstream consumes native FDv2 polling and streaming fixtures; analytics pass through the container to a local collector. Tests cover authentication, configured off-variations, owned-close forwarding, and actual Relay pause/resume. Direct SDK tests additionally verify malformed startup responses, cancellation, disconnected streams, and borrowed ownership. The hosted data plane is emulated locally, so these checks do not establish hosted LaunchDarkly account verification.

`make test-race vet tidy modules-check test-tools generate check-generated` passed. Tidy checks all five modules; vet includes integration-tagged code. Documentation generation checks two-pass byte parity. `govulncheck` v1.9.0 on Go 1.27.4 found no vulnerabilities in any of the three runtime packages. The scan does not certify every test dependency or container image.

## Clean Git consumers

Independent consumers installed reviewed source commit `8e7a76c0f17b6e6e0e8cb3042a19de2f231ae9fa` using actual Go pseudo-versions outside the checkout, with `GOWORK=off` and no replacements. Each consumer performed a successful boolean evaluation with race detection. Native and combined consumers checked validation precedence, revisions, and counters. LaunchDarkly and combined consumers exercised invalid TLS configuration and cancellation from an actual SDK component factory. OpenFeature and combined consumers verified that a real provider cleanup error survives repeated Close calls:

| Consumer | Executed toolchain | Selected native root |
| :--- | :--- | :--- |
| Native only | Go 1.24.13 | Candidate commit |
| LaunchDarkly only | Go 1.25.5 | Adapter's declared `331a16e2f707` requirement |
| OpenFeature only | Go 1.26.0 | Adapter's declared `331a16e2f707` requirement |
| Native and both drivers | Go 1.27.4 | Candidate commit |

The adapter-only checks did not force-upgrade the root, so they also validate each adapter's declared minimum root requirement. Selected module metadata contained no replacements. Public dependencies used normal checksum verification. The first-party repository is private: installation used Git authentication and `GOPRIVATE=github.com/goforj/feature`. This does not establish public-proxy availability or a versioned release.

## Review findings

- Compatibility: native Subject retains string attributes. Source, snapshot, schema 1, stable rollout encoding, and revision behavior remain intact. Typed SDK targeting uses the additive Target type. Vendor dependencies and higher Go minimums stay in optional modules. The combined example catalog uses Go 1.26 without raising the native minimum.
- Provider semantics: Source.Load remains a complete native-document boundary. Adapters evaluate through actual SDKs, preserving rules, hooks, context merging, and configured off-variations. A targeting scope does not claim a frozen vendor revision. Provider rollout assignments and freshness are not portable guarantees.
- Ownership and resources: scopes clone bounded target data. SDK constructors explicitly borrow or own. Startup failures clean up owned resources. Integration fixtures own services once per package run, clean partial starts, and fail rather than skip missing Docker.
- Failure safety: structured errors preserve explicit fallbacks and safe classifications rather than upstream diagnostics. Refresh retains valid native snapshots. Outages and malformed responses are tested separately from missing flags.
- Documentation: SDK constructors have executable examples and generated API-index destinations. Generator tests reject unrelated constructors sharing the same name. Unit and integration reports remain separate and tied to tested source.

Hosted LaunchDarkly verification, real GoForj App adoption, framework integration, external Codecov setup, public distribution, and releases remain pending in the [roadmap](roadmap.md).
