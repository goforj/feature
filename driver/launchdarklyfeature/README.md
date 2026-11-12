# LaunchDarkly driver

This optional module adapts the official LaunchDarkly Go server SDK to the portable feature API. It requires Go 1.25 or newer; native-only users of the root library still need Go 1.24.

Use New to borrow an initialized SDK client, or Open with Config to create an owned client. Open waits for SDK initialization with a five-second default timeout, respects caller cancellation, and cleans up partial startup. Close disables this adapter and closes only resources created by Open. Concurrent and repeated calls wait for one cleanup attempt and return the same classified result. Native SDK configuration owns streaming, polling, transport, privacy, events, and flush behavior. The adapter starts no competing update loop.

Target.Key is authoritative. Kind defaults to user. Typed attributes retain native SDK semantics; anonymous and privateAttributes use the vendor's explicit metadata conventions. A multi-kind target uses Kind multi and nested map[string]any identities with their own key fields. No identity kind is inferred from a key prefix. Native SDK numeric representation and targeting behavior remain authoritative.

Direct LaunchDarkly checks require a nonempty Target.Key, or valid child keys in a multi-kind target. Use Features.For to supply that identity before checking a flag. Untargeted Features.Enabled returns its false fallback because an empty LaunchDarkly identity is invalid; Evaluate exposes ErrInvalidSubject with the caller's chosen fallback.

Successful off-variations are preserved, including true values. Resolve returns a safe classified error, a reason, and a variation index when available; it does not fabricate a native document revision or frozen snapshot.

String, Int, Float, and Object provide typed decisions through the same scoped client. Integer evaluation rejects fractions and values outside the exact IEEE-754 integer range of minus (2^53-1) through 2^53-1. Object and array results are owned copies with portable value bounds.

Run from the repository checkout:

```sh
go -C driver/launchdarklyfeature test -race ./...
go -C examples run ./launchdarkly
INTEGRATION_RUN='TestDriverConformance/launchdarkly|TestLaunchDarkly' make test-integration
```

Unit tests exercise the actual SDK's official local data source as well as controlled failure boundaries. Required integration tests additionally run through the official LaunchDarkly Relay Docker image, checking targeting, updates, authentication, analytics forwarding, and container outage recovery. Its upstream data plane is a local fixture, so hosted-account verification remains separate. This module is unreleased; its relative root replacement intentionally exercises sibling source during repository development.

Track submits custom data or numeric metrics through the official SDK, retaining its hooks, privacy, batching, and delivery policy. SupportsTracking describes SDK submission capability, not delivery confirmation. Integration tests inspect custom events and metrics after they cross the real Relay container.
