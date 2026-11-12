# Behavior contract

`feature` is a standalone server-side boolean feature evaluator. Clients are explicitly injected; no package-global client or process-environment mutation is involved.

## Ownership and lifecycle

`Open(ctx, source)` synchronously loads and validates a complete document. Failure returns an error and no usable client. The source is a required dependency borrowed from the application; the client never closes it. There is no zero-value Client or View contract.

Construction starts no goroutines. Call `Refresh` explicitly, or own a goroutine running `Run(ctx, interval)`. Cancel its context and wait for its return during shutdown. The minimum interval is 10 milliseconds. Only one Run loop may own a client at a time. Run waits until the first tick before refreshing; Open has already performed the initial load.

A Source must honor cancellation, keep returned data stable until Refresh returns, and must not reenter that client's Refresh. Filesystem calls cannot be interrupted while in the OS; cancellation is checked at their boundaries. A custom source that ignores its context can delay shutdown.

Clients, views, and MemorySource support concurrent use. Callers must not mutate a Subject's map during evaluation or mutate documents concurrently with calls that accept them. The library deep-copies all mutable document data before publication. MemorySource.Load also returns a detached document.

## JSON schema 1

```json
{
  "schema": 1,
  "flags": {
    "checkout-v2": {
      "default": false,
      "disabled": false,
      "rules": [
        {"subjects": ["tenant:internal"], "value": true},
        {
          "attributes": {"plan": "pro"},
          "rollout": {"basis_points": 1000, "salt": "checkout-2026"},
          "value": true
        }
      ]
    }
  }
}
```

The schema and flags inventory are required. Each flag requires an explicit boolean default; each rule requires an explicit boolean value. Disabled and rules are optional. A rollout requires an integer basis_points and nonempty salt. Unknown structural fields, duplicate keys at any level, JSON nulls, invalid UTF-8 bytes, multiple JSON values, and invalid field types are rejected. Structural names are case-sensitive.

An empty valid inventory (`{"schema":1,"flags":{}}`) deliberately removes all flags. Invalid documents, missing files, and source failures retain the last valid snapshot after startup; they never implicitly clear it. A disabled flag still requires a valid document and rules.

## Decisions

Evaluation validates cancellation, key grammar, and subject bounds first. Invalid requests return the caller fallback. For valid requests:

1. Missing flag: caller fallback, ReasonMissing, ErrNotFound.
2. Disabled flag: false, ReasonDisabled, no error.
3. Rules in document order: first match wins, ReasonRule.
4. No matching rule: the configured default, ReasonDefault.

All supplied conditions in a rule must match. Subjects are exact identity allowlists. Attributes require the key to exist and its string value to match exactly. Missing and explicitly empty attributes differ. Keys, identities, and attributes are case-sensitive. Rules require a nonempty identity list, nonempty attribute map, or rollout condition.

A rollout requires a nonempty identity even at 100 percent. Missing identity makes that rule ineligible, rather than failing evaluation. Empty subjects are valid for unconditional flags and attribute-only rules. Targeting has no expression parser, implicit conversions, nested conditions, flag dependencies, or framework authentication coupling.

Enabled returns only the boolean. Evaluate additionally returns reason, revision, and error. The caller controls the fallback for each decision; there is no implicit global fail-open or fail-closed setting. Views expose identical evaluation semantics.

## Stable rollout contract

For v1, hash these bytes with SHA-256:

1. UTF-8 `goforj-feature-rollout-v1`.
2. Flag key, salt, and subject key, each encoded as an unsigned 32-bit big-endian byte length followed by its UTF-8 bytes.

Interpret the first eight digest bytes as an unsigned big-endian integer and reduce modulo 10000. Match when the bucket is less than basis_points. No case folding or Unicode normalization occurs. The salt is not a secret.

Golden vectors in evaluate_test.go freeze this encoding. Increasing a rollout percentage preserves existing members if identity, flag key, salt, and preceding rules are unchanged. Changing the salt deliberately changes assignment. Buckets provide approximate proportions across a population, not exact quotas in small populations. Hash or encoding changes require explicit compatibility review.

## Atomicity and refresh

Refresh loads and validates away from readers, serializes source loads, and publishes one complete immutable document through a pointer swap. Cancellation while waiting for another refresh does not start an additional load. Cancellation races at publication follow normal operation-completion semantics: a completed refresh can succeed as cancellation arrives.

Snapshot captures a revision that remains fixed for its lifetime. Use one View for multiple decisions when a request or job needs consistency. Independent Client calls can legitimately see different revisions. Existing views may retain older document memory until callers release them.

Revision hashes the normalized validated document, not its timestamp or raw file bytes. JSON whitespace, object-key order, and subject-list order do not change revision. Rule ordering and other document changes can change it. Revisions identify definitions, not every possible semantic equivalence.

FileSource reads the configured path again on every refresh, so editor rename-and-replace saves work. Atomic replacement by the writer avoids publishing an unintended intermediate document that happens to be valid. Paths and symlink targets must be controlled by the application operator. Relative paths follow the process working directory; use absolute paths when it may change. FileSource accepts only regular files, following symlinks to regular files.

## Failure and freshness

Initial source failure prevents Open. Later source or validation failure retains the last valid snapshot indefinitely, while Status reports the last attempt, last success, and a safe error classification. Run retries on subsequent ticks. Unchanged valid refreshes update freshness without changing revision.

Source errors are redacted to ErrSource. Invalid documents use ErrInvalidDocument. Context cancellation and deadlines retain their context error. Error strings never include document contents, filesystem paths, subjects, or custom source error text.

Status evaluation and fallback counts are bounded aggregate counters for calls made through Client. View calls are not counted, avoiding hidden mutable ownership in a captured view. These counters are sampled independently and are not a transactional accounting system. Unknown keys do not create labels or per-key state.

There is no snapshot persistence across process restarts. A restart while its source is unavailable fails initial loading. Automatic expiration of stale snapshots is deliberately absent because reverting to fallbacks can itself change behavior.

## Bounds

| Surface | Limit |
| --- | ---: |
| Raw JSON bytes | 1 MiB |
| Compiled document accounting | 1 MiB conservative budget |
| Flags | 1024 |
| Rules per flag | 64 |
| Subjects per rule | 1024 |
| Attributes per rule or evaluation subject | 32 |
| Identity, salt, or attribute value | 1024 UTF-8 bytes |
| Flag or attribute key | 128 bytes |

Keys contain only ASCII letters, digits, period, underscore, and hyphen. They cannot be empty. Budget accounting reserves up to six encoded bytes per input string byte plus structural overhead, before allocating a deep copy or canonical JSON. Consequently some raw JSON documents smaller than 1 MiB are rejected by the conservative compiled budget. Passing one limit does not bypass the others. JSON scanning also bounds nesting.

## Process and product boundaries

Each process publishes atomically within itself. Multiple processes converge independently when they can read the same definitions; they may disagree during polling or outages. MemorySource is process-local. FileSource does not distribute a file to other hosts. There is no fleet-wide atomic switch or guaranteed update deadline during source failure.

Jobs evaluate at execution unless the application persists an earlier decision. Feature flags do not replace authorization or billing entitlements. A stale snapshot cannot guarantee immediate emergency shutdown. Server-side rules and identities should remain private; exposing selected decisions to a browser requires application policy.

The current native implementation includes no hosted provider adapter, OpenFeature compliance claim, administration API, framework integration, experimentation analytics, or multivariate values. The [provider study](provider-study.md) proposes the next architecture; it does not change the current runtime contract.
