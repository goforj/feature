# Go feature flag provider study

Date: 2026-11-11. Status: research and proposed direction, not an implemented driver catalog or frozen public API.

## Recommendation

Make provider-neutral ergonomics and tested adapters the primary product. Keep the existing native memory/file engine as the local option. Add direct LaunchDarkly and OpenFeature drivers as the first external integration targets, after designing the evaluation boundary against both.

OpenFeature is a standard API and provider ecosystem. LaunchDarkly is a flag-management service with its own evaluation SDK. An OpenFeature adapter can consume multiple vendor providers, including a community-maintained LaunchDarkly provider. A direct LaunchDarkly adapter serves users who want its SDK and GoForj ergonomics without an additional OpenFeature stack.

The current Source interface loads complete GoForj Documents. It is a native-engine extension point, not a portable feature-evaluation contract. External services should keep their own rule evaluation, rollout assignments, and update mechanisms. Adding more Source implementations will not solve that architectural gap.

## What was examined

The study used official documentation, published SDK source, upstream examples/tests, and an executed local SDK probe. It did not assess pricing, service SLAs, hosted credentials, or benchmark performance.

| Library | Useful evidence | Implication for GoForj |
| --- | --- | --- |
| OpenFeature Go SDK | Typed values and details, hooks, transaction context, events, provider lifecycle, and independent API instances | Reuse the SDK through an optional adapter rather than reproducing its standard. |
| LaunchDarkly Go SDK | Local evaluation, explicit fallbacks, context kinds and multiple identities, detailed reasons, tracking, lifecycle, and local test data | A direct adapter should preserve native evaluation and lifecycle behavior. |
| GO Feature Flag | OpenFeature provider with local and remote evaluation modes | A generic adapter cannot promise that every evaluation is local or performs no I/O. |
| Unleash Go SDK/provider | Strategy/variant evaluation, metrics, context mapping, and documented differences between native SDKs and providers | Verify the exact provider version and supported capabilities rather than advertising full vendor parity. |
| ConfigCat Go SDK | Simple getters, declared typed flags, polling options, and explicit immutable snapshots | Study its ergonomics and retain genuine snapshots where a driver supports them. |

Primary references: [OpenFeature Go SDK](https://openfeature.dev/docs/reference/sdks/server/go/), [LaunchDarkly SDK source](https://github.com/launchdarkly/go-server-sdk/blob/v7.22.0/ldclient.go), [GO Feature Flag Go provider](https://gofeatureflag.org/docs/sdk/server_providers/openfeature_go), [Unleash provider comparison](https://docs.getunleash.io/sdks/openfeature), and [ConfigCat Go reference](https://configcat.com/docs/sdk-reference/go/).

## Important findings

### OpenFeature already offers a short common path

The inspected SDK provides boolean convenience evaluation as well as values with errors and detailed results. A GoForj facade must add value through subject scoping, consistent setup, failure policy, and framework integration. Merely renaming Boolean to Enabled is not a sufficient product advantage.

Its isolated API supports dependency injection without changing the default global provider. The inspected v1.20.0 isolated package is explicitly experimental. Borrowing an initialized client provides a useful integration boundary; an owning constructor can encapsulate isolated API creation after its lifecycle contract is tested.

Sources: [client methods](https://github.com/open-feature/go-sdk/blob/v1.20.0/openfeature/client.go) and [isolated API factory](https://github.com/open-feature/go-sdk/blob/v1.20.0/openfeature/isolated/isolated.go).

### Vendor values must remain authoritative

The probe proved that a LaunchDarkly flag can be off while its configured off-variation evaluates to true. OpenFeature reports a disabled reason alongside that true value. Our native Disabled field forces false by design. A generic adapter must preserve the vendor value and reason rather than applying the native engine's policy.

The inspected community provider also supports multiple context kinds and has explicit ownership controls. Its v0.2.0 implementation has no OpenFeature Tracker or EventHandler implementation. Do not translate that into a claim that LaunchDarkly itself lacks tracking or events. Provider support and native SDK support are separate claims.

Sources: [pinned provider implementation](https://github.com/open-feature/go-sdk-contrib/blob/providers/launchdarkly/v0.2.0/providers/launchdarkly/pkg/provider.go), [community maintenance notice](https://github.com/open-feature/go-sdk-contrib/blob/providers/launchdarkly/v0.2.0/providers/launchdarkly/README.md), and the [executed probe](research/provider-probe/README.md).

### String-only subjects are too narrow

The probe targeted a tenant using a boolean paid attribute and numeric seat count. Replacing those values with strings changed the result. LaunchDarkly multi-context evaluation also needs structured identities. The current map[string]string Subject cannot represent these cases faithfully.

The revised context design should preserve supported attribute types, define ownership and bounds, and offer a clear mapping path for multiple identities and private attributes. Do not silently stringify values or infer vendor-specific identity kinds from a naming convention such as tenant:acme.

Reference: [OpenFeature evaluation-context contract](https://openfeature.dev/specification/sections/evaluation-context/).

### Snapshots and local evaluation are capabilities

Our native engine and ConfigCat expose genuine immutable snapshots. The OpenFeature evaluation interface does not provide a universal frozen rule snapshot. Capturing several values at different times or caching the first result per key does not establish one coherent configuration revision, and can change evaluation-event behavior.

Likewise, GO Feature Flag documents both local and remote evaluation modes. A generic provider can perform I/O. The portable contract must honor context and document the selected driver's behavior, rather than retaining a universal no-I/O promise.

Keep subject scoping distinct from snapshot consistency. Preserve native snapshots as an explicit capability. Do not fabricate a snapshot revision or freshness timestamp for a provider that cannot supply one.

Sources: [ConfigCat snapshot implementation](https://github.com/configcat/go-sdk/blob/v9.1.0/snapshot.go), [OpenFeature provider contract](https://openfeature.dev/specification/sections/providers/), and [GO Feature Flag evaluation modes](https://gofeatureflag.org/docs/sdk/server_providers/openfeature_go).

### Existing provider coverage is uneven

The inspected published Unleash provider is v0.1.1-alpha and depends on the v4 client. Current Unleash Go documentation describes a v6 SDK. Its broader comparison also lists native features unavailable through OpenFeature. Installing a provider is not proof of current vendor feature parity.

Sources: [published provider module](https://github.com/open-feature/go-sdk-contrib/blob/providers/unleash/v0.1.1-alpha/providers/unleash/go.mod), [current Go SDK documentation](https://docs.getunleash.io/sdks/go), and [provider comparison](https://docs.getunleash.io/sdks/openfeature).

## Proposed library boundary

The next design iteration should separate three responsibilities:

1. Portable client: simple evaluation, explicit detailed results, subject scoping, and bounded error classification.
2. Driver: provider-native evaluation, context conversion, readiness, and SDK ownership.
3. Native engine: Document, Source, refresh, immutable rules, and stable native rollout encoding.

Keep framework registries, environment settings, generated providers, and Lighthouse outside the standalone domain API.

The initial driver plan is:

| Driver family | Packaging | Initial promise |
| --- | --- | --- |
| Native memory/file | Included with the dependency-light root | Existing native rule, refresh, and snapshot semantics. |
| LaunchDarkly | Optional module, wrapping the actual vendor SDK | Native boolean evaluation and details with tested context and ownership mapping. |
| OpenFeature | Optional module, wrapping an initialized SDK client or explicitly owned API | Preserve SDK evaluation, hooks, context merging, and provider behavior. |

Package names and constructor signatures are not frozen. The OpenFeature driver is a route to additional providers, not a blanket claim that every provider is tested or feature-complete. Add dedicated GO Feature Flag, Unleash, or ConfigCat drivers when their setup or capabilities justify independent maintenance.

Start by proving one complete boolean workflow across all three families. Document broader typed evaluation explicitly before calling the facade a complete OpenFeature implementation. Consuming an OpenFeature client does not make our API an implementation of the OpenFeature specification.

Do not call FeatureProvider.BooleanEvaluation directly to bypass the SDK: that would bypass SDK behavior such as hooks and context merging. Do not export vendor targeting rules and re-evaluate them through our native engine.

## Ergonomic target

The common path should be a short check with a documented false fallback. Identity should be bindable once without mutating a shared client. This is an API sketch for comparison, not shipped code:

```go
features.Enabled(ctx, "checkout-v2")
features.For(subject).Enabled(ctx, "checkout-v2")
```

The explicit path must still expose chosen fallbacks, reasons, variants where available, and errors. A subject scope must not claim to freeze backend configuration. Compare this shape against retaining the current explicit fallback arguments and against declared typed flag handles before committing to it. ConfigCat's typed flag API is a useful existing precedent; more fluent calls are not automatically simpler.

Source: [ConfigCat typed flag implementation](https://github.com/configcat/go-sdk/blob/v9.1.0/flag.go).

Construction should be equally clear: pure wrappers borrow initialized clients; explicitly named Open operations may initialize SDK resources and return an error. Owning drivers close their resources, borrowed drivers leave them alone. No global provider registration, per-request SDK construction, or competing polling loop around a streaming SDK.

## Go version and dependency impact

Published modules inspected for the probe:

| Module | Version | Declared Go minimum |
| --- | --- | --- |
| OpenFeature Go SDK | v1.20.0 | 1.26.0 |
| Community LaunchDarkly provider | v0.2.0 | 1.26.0 |
| LaunchDarkly server SDK | v7.22.0 | 1.25.0 |
| ConfigCat Go SDK | v9.1.0 | 1.23 |

Keep optional SDKs in their own modules so native-only consumers retain the root's Go 1.24 minimum and zero external runtime dependencies. Applications selecting an optional driver must meet that driver's effective dependency requirements. Recheck the complete selected graph during implementation; do not assume a module's own directive is its entire transitive constraint.

References: [OpenFeature go.mod](https://github.com/open-feature/go-sdk/blob/v1.20.0/go.mod), [LaunchDarkly provider go.mod](https://github.com/open-feature/go-sdk-contrib/blob/providers/launchdarkly/v0.2.0/providers/launchdarkly/go.mod), [LaunchDarkly SDK go.mod](https://github.com/launchdarkly/go-server-sdk/blob/v7.22.0/go.mod), and [ConfigCat go.mod](https://github.com/configcat/go-sdk/blob/v9.1.0/go.mod).

## What highly tested means

Keep exact unit coverage for our runtime code, but add a required shared driver contract and native SDK verification. Test more than whether a fake returns true.

The shared contract should require explicit fallbacks for missing flags and failures; distinction between false and failure; supported typed context preservation; context ownership; cancellation/deadlines; safe classified errors; concurrent use; and separation between independently configured clients.

Driver-specific verification must cover native updates, initialization failures, shutdown/flush ownership, privacy mappings, variants and reasons, and unsupported capabilities. Test OpenFeature hooks and context precedence through the real SDK. Test direct LaunchDarkly through its actual evaluation engine and official local data source. Verify both the borrowed and owning constructors.

Use vendor-native rollout fixtures rather than requiring identical bucket membership across vendors. Native salt/hash stability remains a native-engine contract. Test remote providers with real disposable services where applicable; keep hosted-account verification separate and opt-in. Never claim a hosted integration from a local fixture.

Before claiming the most ergonomic API, write the same realistic application service using each candidate interface and compare construction, request identity, checking, explaining failures, test overrides, and shutdown. Prefer one supported path over a collection of specialized convenience variants.

## Executed evidence and limits

Five race-enabled probes passed on Go 1.27.4 using published modules and normal checksum verification, with no local replacements. They exercised LaunchDarkly directly and through OpenFeature, independent APIs, updates, errors, cancellation, ownership, off-variation behavior, multi-contexts, typed attributes, and ConfigCat snapshots.

The [retained fixtures and raw results](research/provider-probe/README.md) make this reproducible. LaunchDarkly used its official local test data source with event delivery disabled; ConfigCat used local snapshots. No hosted service or account was contacted. These probes do not certify drivers, lifecycle failures, event delivery, remote polling/streaming, or provider-wide conformance.

Source inspection also used these checkout revisions: OpenFeature 398b2b776fe965b5b6d5179140d70ec7b320ecac, go-sdk-contrib ce76eacacf16233ff10a00ce4d457287fb386651, and LaunchDarkly 733322c53634729eb8d267622b0242e0152370eb. Conclusions about the executed behavior use the published module versions above rather than assuming those branches are identical.

## Next step

Review and choose the portable API against native, direct LaunchDarkly, and OpenFeature usage examples. Then implement the evaluation boundary and drivers together with the shared contract. Revisit the existing Snapshot, Subject, Reason, Status, and lifecycle surfaces deliberately; do not silently change their meaning.

This is a pre-v1 architecture decision. The current native implementation remains usable and its runtime API is unchanged by this study. Any later source/API, behavior, configuration, data-format, or minimum-Go changes need their own compatibility assessment and migration guidance before implementation.
