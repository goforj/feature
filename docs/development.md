# Development

The root module has a Go 1.24 minimum and no external requirements. Optional LaunchDarkly and OpenFeature driver modules require Go 1.25 and 1.26 respectively. scripts/module-manifest.txt owns the complete inventory. Canonical executable examples currently share the root module. Documentation and coverage tooling use the same standard-library Go toolchain as the library.

## Scope and implementation decision

The initial workflow is a local boolean decision with exact targeting, stable percentages, complete immutable snapshots, and explicit live refresh. Memory and regular-file JSON sources are the complete initial source catalog. No framework packages or deployment services are required.

The first implementation uses a native evaluator with no external runtime dependencies. The subsequent [provider study](provider-study.md) examines multiple SDKs, inspects their source, and executes pinned local SDK probes. It proposes a provider-neutral core plus optional direct LaunchDarkly and OpenFeature drivers. The initial adapters are implemented and their unit suites enforce exact full runtime coverage. Service integration validation is a separate gate. No claim of hosted-provider interchangeability or OpenFeature conformance is made.

The implementation follows the GoForj feature-flags proposal, with Open replacing the proposed I/O-performing New constructor. Source constructors remain explicit and do not perform filesystem I/O. Framework integration, write APIs, release publication, and hosted account setup remain separate. SDK drivers and container-backed integration are in scope for the current library build.

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

The unit runner executes Go tests and all canonical example programs, instruments example subprocesses against the root library package, and merges their coverage. It requires exact coverage of every statement in the runtime library package. Program scaffolding and Go build tooling are validated separately rather than represented as covered runtime code. Each run writes executed case counts, covered statement counts, toolchain identity, and source fingerprint under .artifacts. Use make record-unit to update docs/unit-results.json deliberately after source changes. Statement totals can differ between Go versions; every run must independently achieve exact full coverage without rewriting evidence recorded on another toolchain. No integration run supplies missing unit coverage.

GoDoc owns API descriptions. examples/*/main.go owns snippets and checked expected output. docs/README.tmpl.md owns the surrounding presentation. tools/maintain checks API names, associated calls, snippet boundaries, and current unit evidence, then generates README.md and status badges. Regeneration twice must preserve the bytes. Never edit the generated README to repair a source issue.

Keep source-owned comments and expected output immediately adjacent to calls. One final Output comment per program supplies the executable runner's full output contract. During race runs, subprocess programs receive race instrumentation too.

## Publication status

The repository is unreleased. Local validation does not establish public installation, remote CI, Codecov uploads, or GitHub App access. The local task prepares workflows and a strict Codecov policy; it does not configure external accounts, repository secrets, webhooks, or releases.

Before publishing, push the reviewed source when authorized, observe CI for that exact revision, verify a clean public-proxy consumer with GOWORK=off and normal checksums, and confirm external coverage setup if enabled. Until then the README's clone-and-run path is the intended entry point. No release tag or pseudo-version is invented.
