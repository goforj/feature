# Driver capabilities

The portable API uses one injected evaluator and owned request targeting. Enabled keeps the short boolean path; Evaluate, String, Int, Float, and Object expose explicit fallbacks and decisions. Unsupported optional capabilities return ErrUnsupported rather than silently succeeding.

| Capability | Native engine | Direct LaunchDarkly | OpenFeature adapter |
| :--- | :--- | :--- | :--- |
| Boolean values | Schema 1 and 2 | Official SDK | Configured provider |
| String, integer, float, object values | Schema 2 | Official SDK | Configured provider |
| Integer precision | Exact int64 | Exact integers through 2^53-1; reject fractions and larger values | Provider-specific; native provider preserves int64 |
| Structured values | Owned JSON maps and arrays, bounded to 64 KiB | Owned JSON maps and arrays, same portable bounds | Owned JSON maps and arrays, same portable bounds |
| Targeting and rollout rules | Native definitions | LaunchDarkly evaluates its rules | Provider evaluates its rules |
| Updates | Explicit Refresh or caller-owned Run | Native SDK update lifecycle | Native SDK/provider lifecycle |
| Immutable definition snapshots | Native View, including typed reads | No portable snapshot guarantee | No portable snapshot guarantee |
| OpenFeature provider | NewProvider exposes the native evaluator through the optional module | Can also be exposed through NewProvider | Consumes ordinary SDK clients and providers |
| SDK hooks and context merging | Through the native OpenFeature provider | Configured SDK hooks | Configured SDK hooks and context merging |
| Evaluation analytics | Observability additions are in progress | Configured native SDK; real Relay forwarding tested | Configured provider |
| Custom tracking and change notifications | In progress | Native SDK API available | Native SDK API and provider capabilities |

The real-service suite tests native, the native OpenFeature provider, direct LaunchDarkly, the community OpenFeature LaunchDarkly provider, and GO Feature Flag in remote and in-process modes. The versions in integration/versions.json define the executed scope. Supporting the OpenFeature API boundary does not establish verification of every provider or full specification conformance.

Hosted LaunchDarkly account verification, application adoption, versioned publication, and external account setup remain separate. Scheduled/progressive native rollouts, HTTP sources, tracking, and change-notification primitives are the next implementation phase of the current build, not completed claims in this matrix.

## Native model migration

This repository is unreleased and its API is fluid. Flag.Default and Rule.Value now accept supported values rather than bool alone. Existing keyed boolean definitions and boolean evaluation methods retain their behavior; code that reads these fields as bool must select the appropriate typed value. Use Encode to validate and persist Go documents so zero-value boolean definitions and numeric type annotations are normalized before JSON encoding.

Schema 1 remains strict and boolean-only. Schema 2 retains the same flags, default, rules, subjects, attributes, and rollout structure while adding type and off. Defaults, rule results, and off-values must agree on one type. Scalar null defaults are invalid; null is supported inside objects and arrays. Existing schema 1 revision encoding and rollout hashing remain unchanged.

NewProvider borrows its evaluator, performs no global registration, and starts no update loop. Bind it to an ordinary OpenFeature API. Provider shutdown disables that provider and leaves the borrowed engine available to its owner.
