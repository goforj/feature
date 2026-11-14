# Application adoption smoke

The release smoke renders a disposable GoForj HTTP App under /tmp and injects the library through app-owned Wire providers. It does not change the framework checkout, add a generated Feature component, start Docker services, or connect to Lighthouse. The App uses the rendered memory cache, in-process events, and local storage defaults.

The [fixture](../integration/testdata/goforj-smoke) keeps source lifecycle in the App's lifecycle registry and injects Features into a business service. A controller reads boolean, string, and structured decisions for a fake pro tenant. A held request captures its decisions before a source update and finishes after it. The runner checks consistent values and revisions, unchanged App PID, invalid JSON and deletion retention, recovery, and cancellation/join at shutdown. Rerender must preserve all app-owned fixture files, and module selection must contain no replacements.

Run from this repository with an installed GoForj CLI:

```sh
go run ./tools/appsmoke -version v0.1.0
go run ./tools/appsmoke -version v0.1.0 -dev
```

Use -forj to select a renderer executable and -keep to retain a successful project for inspection. Failed projects are retained automatically. Execution reports go under .artifacts. The dev scenario enables the conventional native dev.apps.app lifecycle, using its default Go and environment watch patterns. It verifies that config/flags.json updates do not restart the App.

The initial source smoke used renderer source d6ec51576d5f9dc6f74bc37f9907367a6280689c, built with Go 1.27.4. Its worktree's unrelated documentation and agent-guidance files were already modified; no renderer, template, runtime, or dependency source was changed for this check. The rendered App selected Go 1.27.4. That is the tested framework/application composition, not a higher minimum for native library consumers, which are verified independently on Go 1.24.13.

The rendered App's complete go test -race ./... suite also passed separately. Its generated HTTP tests used their default enabled-Lighthouse setting with the connection URL pointed at unused loopback port 1; runtime smoke processes disabled Lighthouse entirely.

Versioned execution evidence is recorded in [release readiness](release-readiness.md). Hosted vendor accounts and complete generated framework integration are outside this smoke.
