# API simplicity audit

Audited the API at `a6726f7` through native setup, ordinary reads, targeting, snapshots, refresh, SDK ownership, custom drivers, and structured values. This is a consumer workflow and source review, not a claim that one API is universally most ergonomic.

The read path is compact and the ownership model is sound. The API is not yet as coherent or discoverable as it could be. The strongest opportunities concern competing frontends and preserving facade policy when capturing a native snapshot. They are design friction, not newly discovered evaluation defects.

## Findings

| Priority | Finding | Recommendation |
| :--- | :--- | :--- |
| High | Two public evaluation frontends expose different targeting and boolean signatures. | Establish Features and Scope as the canonical application API; present Client as the native owner of refresh, status, and snapshots. Keep existing methods compatible. |
| High | Capturing a native snapshot through a fresh Features wrapper requires reconstructing facade policy. | Design a policy-preserving snapshot path before freezing v1. Keep snapshot support explicitly capability-dependent. |
| Medium | Boolean convenience returns a value, while typed reads always return a Result. | Decide and document the value-versus-details convention before v1. Avoid adding several naming families merely to remove `.Value`. |
| Medium | Object exposes JSON trees, not application structs. | Make that boundary visible. Consider one generic decoding helper if a concrete application workflow justifies it. |
| Medium | Native definition errors provide little authoring feedback. | Separate safe runtime errors from a possible explicit definition-validation report. Keep values and source locations out of runtime diagnostics. |

### Competing frontends

These are both current, supported ways to read a native boolean:

```go
engine.Enabled(ctx, "checkout-v2", false, feature.Subject{Key: "tenant:acme"})
feature.New(engine).For(feature.Target{Key: "tenant:acme"}).Enabled(ctx, "checkout-v2")
```

The caller must learn Client versus Features, Subject versus Target, and two Enabled signatures. Features and Scope already support the desired short application path across drivers. The public constructor and method inventory still makes the older native boolean path easy to encounter first. Client implements ResolveString and the other driver-facing methods, whereas View also has String and other direct typed methods. This is mechanically valid but adds another naming distinction for a consumer exploring GoDoc.

Recommend one primary application story: initialize a native engine or SDK driver, wrap it once with New, then inject Features into services. Keep the source owner at application startup. Use Target consistently in application examples. Existing Subject and native methods can remain supported without being competing introductory APIs. This preserves source/API, configuration, persisted data, runtime behavior, and minimum Go versions.

The separate owner and facade are useful. Collapsing all providers into one client with fake portable Refresh, Status, or Snapshot methods would hide real backend differences. A one-function memory-only constructor would shorten the showcase but would not remove the need for source ownership in real applications.

### Snapshot policy is reconstructed manually

The documented recipe is:

```go
request := feature.New(engine.Snapshot()).For(target)
```

The captured decisions are correct, but this constructs a new facade. It does not inherit WithObserver or WithTracker from an existing Features instance. View is not a Tracker, so a fresh snapshot scope also does not discover the native engine's event sink. A consumer must explicitly supply the intended options again. Facade options and targeting are available separately from snapshot ownership; there is no public operation combining them.

A native snapshot workflow should preserve the observer, tracking override, and owned target deliberately. Before adding a helper, specify whether capturing an already captured View is idempotent, how unsupported SDK drivers report the capability, and which tracking sink is retained. Do not silently return a live SDK scope under a snapshot name. A new method could preserve compatibility; changing existing construction behavior would require a runtime compatibility review. This audit does not implement a new signature.

The preferred candidate for a consumer trial is a single Features.Snapshot method returning a new Features and an error. It would preserve the facade's observer and tracker, capture native definitions and time once, and return ErrUnsupported for drivers without genuine snapshots. Capturing an already captured native view would preserve that view. Target ownership would remain with the existing For operation. Proposed usage, not implemented API:

```go
snapshot, err := flags.Snapshot()
if err != nil {
	return err
}
request := snapshot.For(target)
theme := request.String(ctx, "theme", "classic").Value
```

### Value access and detailed access

The current pattern is:

```go
enabled := flags.Enabled(ctx, "checkout-v2")
theme := flags.String(ctx, "theme", "classic").Value
result := flags.String(ctx, "theme", "classic")
```

Enabled intentionally chooses a false fallback; Evaluate supplies an explicit boolean fallback and metadata. String, Int, Float, and Object always take a fallback and return a Result. This is compact, makes metadata readily available, and avoids duplicating every typed method. The cost is a different shape for ordinary boolean and typed reads.

A scalar getter family with a separate details family is a legitimate alternative, not an automatic improvement. Renaming String to return string would break existing callers using `.Value`, `.Err`, or other result fields. Adding StringValue and peers preserves compatibility but expands the public surface and leaves two similarly named operations per type. Prefer keeping the current Result API until a real usage trial demonstrates that the extra family improves the common path. Do not change return types as incidental cleanup.

### Structured values

Object accepts JSON maps and arrays. A struct fallback is rejected with ErrInvalidValue even when the configured object is valid. Native object integer leaves are int64; LaunchDarkly object numbers retain float64. Consequently application structs require explicit decoding rather than a direct type assertion. This is the documented value model, not a conversion bug.

A future generic decoding helper should preserve fallbacks and metadata, enforce the existing size and numeric rules, and report decoding failures explicitly. Go 1.24 supports generic functions, not methods with new type parameters. Do not raise the minimum Go version or introduce reflection-based struct support throughout every driver to make one call shorter.

### Authoring feedback

NewMemorySource, Decode, Encode, and Replace validate definitions early, but most definition failures collapse to ErrInvalidDocument. That keeps runtime errors safe while making it harder to locate a wrong variant, incompatible value, or exceeded budget in a larger document. A separate explicit validation report could improve authoring without putting source contents into runtime errors. This would be a new domain capability and needs its own bounded, privacy-aware contract; it is not an excuse to expose upstream errors.

## Keep

- For(Target) owns targeting once and keeps shared clients safe for concurrent use.
- Explicit fallbacks, classified errors, and Result metadata make failure behavior visible.
- New borrows resources; Open performs initialization and owns only the resources it creates.
- The application owns Run cancellation and joins it during shutdown.
- Native snapshots freeze definitions and evaluation time. Vendor SDK scopes retain vendor semantics.
- Optional driver modules isolate SDK dependencies and minimum Go versions.
- Context remains explicit per call. Copying the context-binding conventions of some siblings is not necessary to make feature reads short.
- Native definitions remain plain data. Avoid a second fluent rule language that duplicates Document, Rule, and Condition.

## Comparison and validation

Inspected the local cache facade/store boundary, search Backend/Index and typed SearchAs workflow, and storage context-binding interface. These siblings support the owner/facade separation, but differ on context and result conventions. Their APIs are evidence of tradeoffs, not a single GoForj rule to copy.

Inspected the repository-pinned OpenFeature Go SDK v1.20.0 and LaunchDarkly Go server SDK v7.22.0. Both distinguish value access from details access and retain explicit evaluation context. Features already removes repeated targeting arguments through For. This comparison concerns these pinned sources, not the latest vendor releases or a complete external-library audit.

An independent temporary consumer module passed ten race-enabled repetitions of three public-API probes: native and portable read shapes, snapshot facade policy reconstruction, and JSON-tree versus struct fallbacks. The probes confirm current behavior; they do not claim that the proposed API additions exist. The existing 12 executable programs remain the canonical supported workflows.

The audit preserves all public signatures and behavioral contracts. Its only source correction clarifies package GoDoc: native decisions read local state, while SDK providers and configured callbacks can perform I/O. Production correctness acceptance does not establish that every API choice is optimal. Resolve the application-frontdoor and snapshot-policy decisions before freezing the initial API; retain the current result model unless a consumer trial justifies expanding it.

## Implemented adjustments

The follow-up implements the compatible changes from this audit:

- Features and Scope are the documented application API. File, polling, targeting, rule, and quick-start examples keep native lifecycle operations on the engine. Existing native signatures and Subject remain available, with their API-index entries grouped as native compatibility.
- Features.Snapshot captures native definitions and time while retaining observer and selected tracking policy. Recapturing a native View preserves the capture. Unsupported SDK drivers return ErrUnsupported. The original target ownership stays with For.
- ObjectAs infers an application type from its fallback and decodes structures through the small ObjectReader boundary implemented by Features and Scope. It retains metadata, owns JSON-visible fallback data separately from provider input, and redacts conversion errors. Concrete fields use standard JSON conversion rules; interface fields retain json.Number. Custom JSON callbacks remain application-owned.
- Validate returns one bounded deterministic ValidationError for explicit authoring tools. Path and Problem are available for inspection while ordinary error text and runtime source operations stay redacted. The existing compiler remains authoritative for acceptance.
- Result-based typed reads and the existing boolean convenience methods retain their signatures. No second getter/details naming family was added.

Runnable examples, direct regression tests, and the shared real-service integration suite exercise the new workflows. Schema 1 bytes, native revisions, rollout assignments, optional module boundaries, and minimum Go versions remain unchanged.
