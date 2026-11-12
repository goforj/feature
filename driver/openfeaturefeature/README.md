# OpenFeature driver

This optional module adapts an OpenFeature SDK client to the portable feature API. It requires Go 1.26 or newer because OpenFeature v1.20 requires that version. The root library keeps its Go 1.24 minimum and zero external runtime dependencies.

New borrows an initialized SDK client. Open owns a separate isolated API, registers the supplied provider, waits for initialization, and cleans up failed startup. Close disables this adapter and shuts down only an owned API with a five-second cleanup deadline. The SDK's isolated factory is experimental and is contained behind this module.

Resolve calls the SDK client rather than calling a provider directly, preserving SDK hooks, context precedence, and configured provider behavior. Target.Key is authoritative over a reserved targetingKey attribute. Kind is an explicit kind attribute; structured attributes are forwarded without string coercion. Individual providers define how they interpret those fields and privacy metadata.

This adapter supports boolean evaluation. It is not a complete implementation of the OpenFeature specification, a guarantee of every provider's capabilities, or a promise that every evaluation is local. Providers can perform network I/O. Request targeting scopes do not freeze provider configuration.

Run from the repository checkout:

```sh
go -C driver/openfeaturefeature test -race ./...
go -C examples run ./openfeature
INTEGRATION_RUN='TestDriverConformance/openfeature|TestRelay|TestOpenFeature' make test-integration
```

Tests verify real SDK hooks, context merging, updates, lifecycle, and independent bindings alongside controlled error boundaries. Service-backed provider tests belong in the integration module. Hosted-account verification remains separate. This module is unreleased; its relative root replacement intentionally exercises sibling source during repository development.
