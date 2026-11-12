# OpenFeature driver

This optional module adapts an OpenFeature SDK client to the portable feature API. It requires Go 1.26 or newer because OpenFeature v1.20 requires that version. The root library keeps its Go 1.24 minimum and zero external runtime dependencies.

New borrows an initialized SDK client. Open owns a separate isolated API, registers the supplied provider, waits for initialization, and cleans up failed or canceled startup. Close disables this adapter and shuts down only an owned API. Concurrent and repeated calls wait for one cleanup attempt and return the same classified result. Cleanup supplies a five-second context deadline; providers must cooperate with it, and legacy provider shutdown cannot be forcibly interrupted. Use a bounded initialization context. The SDK's isolated factory is experimental and is contained behind this module.

Resolve calls the SDK client rather than calling a provider directly, preserving SDK hooks, context precedence, and configured provider behavior. Target.Key is authoritative over a reserved targetingKey attribute. Kind is an explicit kind attribute; structured attributes are forwarded without string coercion. Individual providers define how they interpret those fields and privacy metadata.

An empty Target.Key allows identity to be inherited from the SDK's configured API, transaction, or client context. Providers decide whether an identity is required. Use Features.For when each request or job needs its own explicit identity and attributes.

This adapter supports boolean, string, integer, float, and structured evaluation. NewProvider exposes an injected native engine or other evaluator to ordinary OpenFeature SDK consumers, without owning its refresh or shutdown. See the [native provider program](../../examples/native-provider/main.go). Numeric interpretation remains provider-specific. It is not a complete implementation of the OpenFeature specification, a guarantee of every provider's capabilities, or a promise that every evaluation is local. Providers can perform network I/O. Request targeting scopes do not freeze provider configuration.

Run from the repository checkout:

```sh
go -C driver/openfeaturefeature test -race ./...
go -C examples run ./openfeature
INTEGRATION_RUN='TestDriverConformance/openfeature|TestRelay|TestOpenFeature' make test-integration
```

Tests verify real SDK hooks, context merging, updates, lifecycle, and independent bindings alongside controlled error boundaries. Service-backed provider tests belong in the integration module. Hosted-account verification remains separate. This module is unreleased; its relative root replacement intentionally exercises sibling source during repository development.

NewProvider forwards native revision changes and stale/ready transitions through the SDK event channel. It also forwards custom tracking to an explicitly configured native event sink. SupportsTracking reports known support; an owned adapter returns ErrUnsupported for a provider known to lack tracking. Borrowed clients can only establish SDK submission capability. OpenFeature Track has no error result, so observe native sink failures through WithObserver when needed. The [capability matrix](../../docs/capabilities.md) records pinned provider limitations, including custom tracking and numeric metrics.
