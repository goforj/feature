# Behavior contract

`feature` is a standalone server-side typed feature evaluator. Clients are explicitly injected; no package-global client or process-environment mutation is involved.

## Ownership and lifecycle

`Open(ctx, source)` synchronously loads and validates a complete document. Failure returns an error and no usable client. The source is a required dependency borrowed from the application; the client never closes it. There is no zero-value Client or View contract.

The native engine starts no refresh goroutine. Injected sources and transports retain their own resource behavior. Call `Refresh` explicitly, or own a goroutine running `Run(ctx, interval)`. Cancel its context and wait for its return during shutdown. The minimum interval is 10 milliseconds. Only one Run loop may own a client at a time. Run waits until the first tick before refreshing; Open has already performed the initial load.

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

Refresh rechecks cancellation after acquiring its source gate, including when acquisition and cancellation are simultaneously ready. The native typed Resolve adapter applies the same cancellation and key-validation precedence as Evaluate before converting target attributes; these early failures retain native revision and counter behavior.

Snapshot captures a revision and evaluation time that remain fixed for its lifetime. Use one View for multiple decisions when a request or job needs consistency. Independent Client calls can legitimately see different revisions. Existing views may retain older document memory until callers release them.

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

The root native engine has no vendor dependencies. Optional LaunchDarkly and OpenFeature modules adapt actual SDKs through the portable boundary below, with [container integration tests](../integration). They do not change native snapshot, JSON, or rollout semantics. Administration APIs, framework integration, statistical experiment analysis, and full OpenFeature specification conformance remain outside the implemented contract. The [provider study](provider-study.md) records the architecture research.

## Portable evaluation

`New(evaluator)` borrows an initialized Evaluator and returns Features. `Enabled(ctx, key)` has a false fallback; `Evaluate(ctx, key, fallback)` retains an explicit fallback and returns Decision. A successful external decision keeps its value even when its reason is disabled. Unknown errors from custom evaluators are classified as ErrUnavailable rather than exposing provider text.

`For(Target)` owns typed targeting data for repeated checks without changing a shared client. Target is separate from the existing native Subject so the native string-map API remains source compatible. Target supports bounded booleans, Go numeric primitives, strings, time.Time, nested string-key maps, []any, and []string. Unsupported values, non-finite floats, excessive nesting, and contexts above the 64 KiB aggregate budget return ErrInvalidSubject on evaluation. A Scope is targeting data, not a frozen provider configuration.

Native Client implements Evaluator through Resolve. Its existing Open, Evaluate, Enabled, Snapshot, Refresh, Run, and Status methods retain their contracts. Legacy rule attributes match strings only; schema 2 Conditions preserve typed values and structured paths. Native identity hashing uses the explicit key and ignores the optional identity kind. Features does not expose fake generic snapshots or start a second update loop around a vendor SDK.

SDK adapters expose Close on their concrete drivers. New borrows its SDK client; closing that adapter disables only the adapter. Open owns its SDK resources and initialization cleanup. Concurrent Close calls wait for one cleanup attempt and return its same classified result, including repeated calls after cleanup failure. Calls already evaluating when closure begins follow the native SDK's lifecycle semantics; new evaluations return ErrClosed.

OpenFeature Open honors cancellation observed before publishing a successfully initialized driver. Cleanup supplies a five-second context deadline, but context-aware provider implementations must cooperate with it. Legacy provider shutdown has no context parameter, so the SDK cannot impose a hard timeout on such a provider. Applications must supply bounded initialization contexts and providers with appropriate shutdown behavior. LaunchDarkly shutdown uses the native SDK's configured flush and timeout policy.

LaunchDarkly also preserves cancellation when SDK construction fails, after disposing of any partial client. Its initialization budget does not forcibly interrupt synchronous native component factories; custom SDK factories must bound their own I/O.

## Typed values and schema 2

Schema 1 remains strict and boolean-only. Schema 2 uses the same flags inventory with boolean, string, integer, float, and structured defaults and rule results. A flag may declare type explicitly as boolean, string, integer, float, or object. Encode normalizes type annotations before persistence, preserving the distinction between integer 1 and floating-point 1.0. Raw JSON writers should declare numeric types explicitly.

Defaults, rule results, and optional off values must have the same type. Disabled booleans default to false; disabled non-boolean flags require an explicit off value. Objects and arrays may contain null, booleans, strings, finite numbers, nested objects, and arrays. Standalone null flag values are rejected. Each owned value is bounded to 64 KiB and eight nested container levels, and container lengths are bounded to 1024. The complete compiled document remains bounded to 1 MiB.

Features and Scope expose String, Int, Float, and Object with explicit fallbacks and Result values. Wrong-type requests return ErrTypeMismatch. Drivers without the optional TypedEvaluator contract return ErrUnsupported. Validated fallback and successful object containers are copied. Native View has matching typed reads against one immutable revision. Native counters include typed reads through Client.

Native integers retain int64 precision. The direct LaunchDarkly adapter rejects integer values outside the exact IEEE-754 range or with a fractional component. Other SDK providers retain their numeric semantics. The [capability matrix](capabilities.md) records these differences.

The optional OpenFeature module also supplies NewProvider: ordinary OpenFeature clients can evaluate the injected engine through the standard provider contract. Provider shutdown disables its binding without closing its borrowed engine. No global registration or competing refresh loop is introduced.

## Native rules and variants

Schema 2 adds variants, default_variant, off_variant, and rule variant or split actions. Variant names reference one flag-local catalog and all values must agree on the flag type. A split has an explicit salt and ordered allocations totaling 10000 basis points. It requires a nonempty identity; missing identity leaves the rule ineligible. Allocation order, salt, flag key, and identity determine membership. Named actions normalize away redundant inline values. Contradictory inline values are rejected.

Conditions are ANDed within each rule. Operators are eq, ne, in, not_in, gt, ge, lt, le, contains, prefix, suffix, matches, exists, and missing. Comparisons require presence except missing; an explicit null is present. Numbers compare without narrowing int64 values, including inside JSON object and array equality. No string-to-number conversion occurs. Literal attribute names are supported, while a leading slash selects JSON Pointer traversal with ~0 and ~1 escaping. $key and $kind select explicit identity fields. Patterns are bounded RE2 expressions compiled before publication.

Rule start is inclusive and end is exclusive. A progressive rollout declares absolute start and end instants and integer from/to percentages in basis points. Percentages clamp before and after the interval and interpolate within it; increasing and decreasing rollouts are supported. A percentage still requires an identity at 100 percent. Calendar behavior is evaluated without a scheduling worker. Native View captures evaluation time together with its immutable definitions, and it implements TypedEvaluator for ordinary scoped use through New(view).

## Observations, custom events, and delivery

WithObserver attaches synchronous operation observations to the facade without exposing values or targeting data. Track submits bounded custom attributes and an optional finite metric through the selected Tracker. Native clients require WithEventSink; a facade can override its sink with WithTracker. SupportsTracking reports known configured support. Provider-specific limits are recorded in the capability matrix.

Subscribe owns one bounded native listener and starts no goroutine. Close unregisters it and is idempotent. Slow listeners receive the latest revision with Keys cleared for a full refresh. Notifications include publication changes and transitions into or out of source failure. Repeated unchanged health does not trigger duplicate notifications. The native OpenFeature provider maps updates and stale/ready transitions onto the SDK event channel and owns only its forwarding worker.

WithNotifier supplies an optional synchronous destination. LastNotifyError records notification failures separately from successful source publication. Callbacks must not reenter the same client's Refresh; they may inspect Status. HTTPExporter sends change notifications, custom analytics, and operation observations to an explicit endpoint. Target identities are omitted by default and require IncludeTarget to export. Observer failures are available through LastError. Delivery has no implicit retries or durable queue; applications can inject their own queue or sink.

HTTPSource and HTTPExporter borrow a transport, own copied headers, reject redirects, and default to a five-second timeout. Responses are bounded and errors never retain endpoint URLs, credentials, or response diagnostics. HTTPSource decodes complete native documents and preserves last-valid snapshots through the normal client lifecycle. SourceFunc remains available for other storage systems and configuration models.

Validated fallback and successful object containers are copied. A request rejected before fallback validation may return the caller's original fallback; that input is never stored in the engine. Optional observers and sinks can perform I/O, so native lookup locality does not imply a zero-I/O facade once these callbacks are configured.
