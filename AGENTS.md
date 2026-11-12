# Repository instructions

This repository owns the standalone feature flag library. Framework components, generated App wiring, commands, and Lighthouse integration belong in goforj and require their own scope.

- Preserve explicit source injection and caller-owned refresh lifecycle. Do not introduce a global client or mutate process environment variables.
- Evaluation must remain local and use complete immutable snapshots. Failed refreshes retain the last valid snapshot.
- Treat schema 1 and stable rollout encoding as compatibility contracts. Add regression coverage for every new validation branch and failure mode.
- Source errors must not reveal paths, targeting identities, attributes, or source contents.
- Required sources fail visibly when miswired. Do not hide nil dependencies behind fallback behavior.
- GoDoc owns API descriptions; examples/*/main.go owns executable snippets and expected output. Update those sources, then run make test-unit generate test-tools check-generated.
- Keep exact runtime library unit statement coverage at 100%. Preserve coverage and race instrumentation in executable example subprocesses.
- Run make test-race vet modules-check for relevant changes. Discover every go.mod before adding modules; the initial repository has only one.
- All Go functions and exported entities need name-leading doc comments. Avoid import aliases and em dashes.
- Check git status before edits and commits. Stage explicit task paths, inspect the staged diff, and use Conventional Commits with the maintainer's existing Git identity.
- External setup, pushes, tags, and release publication require authorization beyond a local implementation task.
