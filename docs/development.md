# Development

The root module has a Go 1.24 minimum and no external requirements. Optional LaunchDarkly and OpenFeature driver modules require Go 1.25 and 1.26 respectively. scripts/module-manifest.txt owns the complete inventory. Canonical executable examples use a separate Go 1.26 module to demonstrate both SDKs without adding dependencies or a higher Go requirement to native consumers. Documentation and coverage tooling use standard-library Go.

## Scope and implementation decision

The initial workflow is a local typed decision with exact targeting, stable percentages, complete immutable snapshots, and explicit live refresh. Native sources include memory, regular files, HTTP, and application-owned SourceFunc adapters. No framework packages or deployment services are required.

The native evaluator has no external runtime dependencies. The [provider study](provider-study.md) informed the portable facade and optional direct LaunchDarkly and OpenFeature drivers. The adapters enforce exact full runtime unit coverage and have a separate required service integration suite. No claim of hosted-provider interchangeability or OpenFeature specification conformance is made. See the [verification record](verification.md) for executed checks and remaining boundaries.

The implementation follows the GoForj feature-flags proposal, with Open replacing the proposed I/O-performing New constructor. Source constructors remain explicit and do not perform filesystem I/O. Generated framework integration, write APIs, and hosted account setup remain separate. SDK drivers and container-backed integration are in scope for the current library build.

## Commands

Run from the repository root:

```sh
make test
make test-unit
make record-unit # Refresh tracked evidence after relevant source changes.
make test-race
make vet
make test-tools
make modules-check
make tidy
make generate
make check-generated
```

Tests use real temporary files for native file-source behavior and the actual vendor SDKs for adapters. `make test-integration` additionally requires Docker and runs the [real-service and SDK transport suite](../integration). `make test-coverage` runs both gates with separate profiles. `make record-integration` records full-suite evidence; focused `INTEGRATION_RUN` executions cannot overwrite that report. Linux CI and local macOS runs execute the same suite; CI separately verifies each runtime module's minimum Go line.

The unit runner executes Go tests and all canonical programs, instruments example subprocesses against first-party runtime packages, and merges their coverage. It requires exact coverage of every runtime statement across the root and both SDK modules. Program scaffolding and Go build tooling are validated separately. Each run writes executed case counts, statement counts, toolchain identity, and source fingerprint under .artifacts. Unit and integration commands compare source fingerprints before and after execution and reject observed source changes instead of recording mixed revisions. Rerun verification after concurrent edits finish. Use make record-unit to update docs/unit-results.json after source changes. Statement totals can differ between Go versions; every run must independently achieve exact full coverage without rewriting evidence recorded on another toolchain. No integration run supplies missing unit coverage.

GoDoc owns API descriptions. examples/*/main.go owns snippets and checked expected output. docs/README.tmpl.md owns the surrounding presentation. tools/maintain checks API names, associated calls, snippet boundaries, and current unit evidence, then generates README.md and status badges. Regeneration twice must preserve the bytes. Never edit the generated README to repair a source issue.

Keep source-owned comments and expected output immediately adjacent to calls. One final Output comment per program supplies the executable runner's full output contract. During race runs, subprocess programs receive race instrumentation too.

## Publication status

The standalone [production-readiness assessment](production-readiness.md) tracks closure reviews, measured resource bounds, stress evidence, and remaining external gates. Native evaluation measurements and their reproducible harness are recorded in [performance](performance.md).

The repository is public. The coordinated v0.1.0 release uses three independently published runtime modules. Root and driver tags, selected commits, public checksums, and application evidence are recorded in [release readiness](release-readiness.md). Development replacements remain intentional and are rejected in external consumer verification.

Release maintenance:

```sh
make release-plan VERSION=v0.1.0
make verify-release VERSION=v0.1.0
make smoke-app VERSION=v0.1.0
make smoke-app VERSION=v0.1.0 SMOKE_ARGS=-dev
```

The planner does not create tags. Verify the root through the normal public Go proxy before publishing driver tags that require it. The public-consumer tool uses fresh module caches, normal checksum verification, GOWORK=off, and no replacements. It exercises each adapter's declared root requirement independently, plus the combined graph, on the supported Go lines. The [App smoke](app-smoke.md) requires a GoForj CLI and renders only under /tmp.

Patch releases preserve public signatures, persisted native document contracts, and rollout assignments. Before v1, a minor release may evolve APIs with documented migration guidance. Hosted-account verification and generated framework integration remain distinct from the released standalone library. Codecov uploads remain optional; exact unit statement coverage is required locally and in CI.
