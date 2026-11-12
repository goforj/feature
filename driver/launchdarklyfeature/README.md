# LaunchDarkly driver

This optional module adapts the official LaunchDarkly Go server SDK to the portable feature API. It requires Go 1.25 or newer; native-only users of the root library still need Go 1.24.

Use New to borrow an initialized SDK client, or Open with Config to create an owned client. Open waits for SDK initialization with a five-second default timeout, respects caller cancellation, and cleans up partial startup. Close disables this adapter and closes only resources created by Open. Native SDK configuration owns streaming, polling, transport, privacy, events, and flush behavior. The adapter starts no competing update loop.

Target.Key is authoritative. Kind defaults to user. Typed attributes retain native SDK semantics; anonymous and privateAttributes use the vendor's explicit metadata conventions. A multi-kind target uses Kind multi and nested map[string]any identities with their own key fields. No identity kind is inferred from a key prefix. Native SDK numeric representation and targeting behavior remain authoritative.

Successful off-variations are preserved, including true values. Resolve returns a safe classified error, a reason, and a variation index when available; it does not fabricate a native document revision or frozen snapshot.

Run from the repository checkout:

```sh
go -C driver/launchdarklyfeature test -race ./...
```

Unit tests exercise the actual SDK's official local data source as well as controlled failure boundaries. Local SDK and protocol verification are separate from hosted-account verification. This module is unreleased; its relative root replacement intentionally exercises sibling source during repository development.
