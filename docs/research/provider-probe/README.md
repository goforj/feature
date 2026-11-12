# Provider study probe

These are research fixtures, not shipped GoForj drivers or product examples. The `.txt` suffix keeps their external dependencies and higher Go requirement outside the root library module. The source and module files are the exact inputs for the recorded run.

Requires Go 1.26 or newer. Copy the fixtures into a fresh temporary module and run them with normal module checksum verification:

```sh
study_dir="$(mktemp -d)"
cp docs/research/provider-probe/go.mod.txt "$study_dir/go.mod"
cp docs/research/provider-probe/go.sum.txt "$study_dir/go.sum"
cp docs/research/provider-probe/provider_test.go.txt "$study_dir/provider_test.go"
(cd "$study_dir" && GOWORK=off go test -race -count=1 -v ./...)
```

The recorded run used Go 1.27.4 on macOS arm64, with a fresh module cache and no local replacements. An existing shared cache contained an empty go-cache package directory; isolating the study cache resolved that local setup issue. Dependency checksum verification remained enabled.

The five probes exercise the actual published SDKs:

- LaunchDarkly evaluation through an isolated OpenFeature API, including a live data update.
- Missing flags, type errors, cancellation, missing identity, and borrowed-client ownership.
- Concurrent evaluation through two independently configured APIs.
- A true disabled off-variation, multi-context mapping, and the pinned provider's tracking/event capability boundary.
- Boolean and numeric attribute targeting without string coercion, plus ConfigCat typed flag defaults and immutable snapshots.

LaunchDarkly uses its official local test data source and disables event delivery. ConfigCat uses its local snapshot API. No hosted account, SDK credential, vendor dashboard, or remote flag service was used. These results establish local SDK behavior, not hosted-service integration, polling/streaming recovery, or complete provider conformance.

See [results.txt](results.txt) for the actual run and [the provider study](../../provider-study.md) for the design implications.
