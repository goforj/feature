# Driver capabilities

The application API uses one injected evaluator and owned request targeting. Enabled keeps the short boolean path. Evaluate, String, Int, Float, and Object expose explicit fallbacks and decisions. Native Client and View, the direct LaunchDarkly adapter, and the OpenFeature adapter implement the typed evaluation boundary.

| Capability | Native engine | Direct LaunchDarkly | OpenFeature adapter |
| :--- | :--- | :--- | :--- |
| Boolean values | Schema 1 and 2 | Official SDK | Configured provider |
| String, integer, float, object values | Schema 2 | Official SDK | Configured provider |
| Integer precision | Exact int64 | Exact integers through 2^53-1; reject fractions and larger values | Provider-specific; native provider preserves int64 |
| Structured values | Owned JSON maps and arrays, bounded to 64 KiB | Same portable bounds; numbers retain SDK float64 semantics | Same portable bounds; provider numeric semantics |
| Typed targeting | Equality, membership, numeric comparison, strings, RE2, presence, and JSON Pointer paths | LaunchDarkly rules | Provider rules |
| Named variants and weighted allocation | Explicit ordered allocations totaling 10000 basis points | LaunchDarkly variations | Provider variations |
| Scheduled and progressive rollouts | Rule windows and time-based percentage interpolation | Backend policy | Provider/backend policy |
| Sources and updates | Memory, file, HTTP, SourceFunc; explicit Refresh or caller-owned Run | SDK update lifecycle | SDK/provider lifecycle |
| Immutable snapshots | View captures definitions and evaluation time; usable as an Evaluator | No portable snapshot guarantee | No portable snapshot guarantee |
| Custom events and metrics | Explicit Tracker sink, TrackerFunc, or HTTPExporter | Native SDK custom events and metrics | Provider-dependent; see below |
| Evaluation observability | Optional Observer and HTTPExporter | Facade observer and configured SDK hooks | Facade observer and configured SDK hooks |
| Change notifications | Bounded Subscribe, Notifier callbacks, HTTP webhooks | Native SDK API | Provider SDK API; native provider forwards changes and health |
| OpenFeature provider | NewProvider exposes an injected evaluator through the optional module | Can also be exposed through NewProvider | Consumes ordinary SDK clients and providers |

The real-service suite tests native, the native OpenFeature provider, direct LaunchDarkly, the community OpenFeature LaunchDarkly provider, and GO Feature Flag in remote and in-process modes. The versions in integration/versions.json define the executed scope. Supporting the OpenFeature API boundary does not certify every provider or establish complete specification conformance.

## Tracking differences

LaunchDarkly uses native string and numeric evaluation methods, so their SDK type errors reach hooks and analytics. Additional portable integer precision, object-shape, and size checks happen afterward. SDK hooks and analytics reflect the native result even when a later portable check returns the caller fallback. Borrowed clients exposing only JSON evaluation retain that path and its native JSON semantics.

SupportsTracking reports configured support where it is known. Native clients need an event sink. Owned OpenFeature adapters reject a provider known to lack Tracker with ErrUnsupported. A borrowed SDK client cannot expose every provider capability, so it can report SDK submission support without proving remote delivery.

The pinned community OpenFeature LaunchDarkly provider v0.2.0 does not implement Tracker. Its adapter therefore rejects custom tracking, although automatic LaunchDarkly evaluation analytics still use the underlying SDK. The direct LaunchDarkly driver supports custom events and metrics, and integration tests inspect their actual payloads after crossing the official Relay container.

GO Feature Flag v1.1.3 supports custom tracking and its real collector accepts the tested events in both evaluation modes. That provider forwards TrackingEventDetails attributes but does not put its separate numeric Value into its tracking payload. Use explicit event attributes such as amount when the GO Feature Flag backend needs that data. The generic adapter passes OpenFeature's numeric value through the standard SDK API and does not invent a provider-specific mapping. These are pinned-provider limits, not limitations of the root Event model.

OpenFeature Track returns no error. Successful adapter submission therefore does not establish provider processing or delivery. The native OpenFeature provider can expose its sink failures through a configured Observer. HTTPExporter.Track returns an error when its endpoint rejects delivery. No success promises durable storage, retries, experiment analysis, or a statistical outcome.

## Native models and extensions

Schema 1 remains strict and boolean-only. Schema 2 supports typed defaults and off-values, named variants, ordered AND conditions, weighted splits, rule windows, and progressive rollouts. Multiple ordered rules can express alternatives. Backend adapters keep their own rules authoritative; native rollout membership is not interchangeable with a vendor's bucketing algorithm.

SourceFunc adapts storage clients and custom configuration models to complete native documents. For example, an application can read through GoForj storage's WithContext and Get methods, then Decode the bytes. Native retrievers do not need to duplicate S3, Azure, database, or Kubernetes SDKs in the dependency-free root. OpenFeature providers retain their own configuration retrievers and formats.

ObserverFunc, TrackerFunc, and NotifierFunc connect application tracing, metrics, audit, queues, or exporters. HTTPExporter implements all three relevant delivery roles: Observer, Tracker, and Notifier. It sends bounded JSON to an explicit endpoint and omits targeting by default. Identity export requires IncludeTarget. Observer metadata includes no flag values or targeting attributes; callers still own the label policy for flag keys.

Observers, custom sinks, and notifiers are synchronous. HTTP delivery is bounded by the configured timeout. Use an application queue when event delivery must be asynchronous or durable. HTTPExporter.LastError exposes observer delivery failures. Notifier failures are separately visible in Status.LastNotifyError and do not undo valid publication.

Subscriptions have a one-message queue and coalesce slow consumers to the latest revision. Keys is nil after coalescing, meaning a consumer should refresh its whole inventory. Native provider forwarding is bounded, preserves SDK health transitions, and stops at provider shutdown without closing the borrowed engine.

A coalesced source failure can also carry a new valid revision. The native provider forwards its configuration event before stale health so SDK cache invalidation sees the publication. Known native source failures reject SDK initialization, including additional domain bindings, while existing bindings can evaluate the retained snapshot. Complete SDK registration before native refresh begins, and serialize later registration with refresh: SDK-owned initialization events can overwrite concurrent health events. Source Status remains authoritative for native freshness.

## API and persisted data

OpenFeature startup cancellation follows the pinned SDK's lifecycle behavior: shutdown can overlap provider initialization. Stateful providers must synchronize those methods. For a provider without that guarantee, manage its initialization and shutdown yourself and use the borrowed-client constructor. The adapter preserves provider binding checks and caller cancellation rather than wrapping providers or detaching their initialization contexts.

The v0.1 release freezes the initial application API and documented native contracts. Flag.Default and Rule.Value accept supported values rather than bool alone. Existing keyed boolean definitions and boolean evaluation methods retain their behavior; code reading these fields as bool must select the appropriate typed value. Encode validates and persists normalized Go definitions, including boolean zero values and numeric type annotations. Named-variant actions normalize away redundant inline values so loading, changing a variant name, and re-encoding remains predictable.

Existing schema 1 canonical bytes, revision hashing, and rollout encoding are covered by compatibility regressions. Schema 2 is opt-in. Snapshot fixes both definitions and evaluation time, so a request retains consistent calendar decisions even as wall time advances. New direct Client calls use the current clock.

Hosted LaunchDarkly account verification, real GoForj App adoption, framework wiring, versioned publication, and external account setup remain separate work.
