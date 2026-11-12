# Native evaluation measurements

These measurements compare commit `987f576f9dbd18d3c3a9b48e4c095e71cdb015b5` with the per-evaluation comparison cache. They establish a specific allocation fix, not a throughput or latency guarantee for every configuration or deployment.

The host was an Apple M4 Max running Darwin arm64 and Go 1.27.4. One uninstrumented benchmark process ran at a time with `-cpu=1`, three 500 ms repetitions, and warmed clients. The public evaluation call is timed; document construction, validation, and initial target ownership are outside the timed loop. Conditions use distinct product membership checks and fail the last check of every rule, forcing the full search without redundant operands. Structured fixtures assert the expected default decision before timing.

| Workload | Baseline median | Cache median | Bytes per check before / after |
| --- | ---: | ---: | ---: |
| Native boolean | 165.9 ns | 163.1 ns | 0 / 0 |
| Scoped boolean | 631.1 ns | 628.6 ns | 720 / 720 |
| Scoped integer | 649.0 ns | 639.9 ns | 736 / 736 |
| 8 rules, 8 conditions, 100 products | 121.502 us | 17.713 us | 222,424 / 7,680 |
| 64 rules, 32 conditions, 1024 products | 42.723 ms | 5.935 ms | 71,387,544 / 70,528 |

The structured fixtures encode to 4,478 and 133,582 document bytes; their targets encode to 1,550 and 15,410 bytes. The representative case uses 209 allocations instead of 6,633, and the largest fixture uses 2,057 instead of 2,102,277. Scalar allocation counts are unchanged. A single structured condition incurs two additional allocations and 464 bytes for the cache, with approximately 4.08 us versus 4.23 us in this run. Close scalar timing differences are measurement noise, not claimed speedups.

The cache normalizes each distinct structured attribute once for one evaluation. It is never shared across requests, targets, flags, or snapshots. Equality and membership still scan their configured values; the fix removes repeated deep copies rather than changing rule outcomes. Profiling attributed 99.87% of the original maximum fixture's allocated bytes to repeated condition normalization. After the fix, the expected equality scans dominate CPU use.

## Reproduction and raw evidence

The exact [external harness](bench/production-performance_test.go.txt), [fresh baseline output](bench/production-baseline.txt), and [cache output](bench/production-comparison-cache.txt) are retained. The baseline checkout was extracted with `git archive`; the cache implementation was measured from the working tree. Regex changes during the audit affected document validation outside the timed loop, not these matching paths.

Copy the harness into a disposable module as `performance_test.go`, require `github.com/goforj/feature v0.0.0`, and replace that module with the checkout being measured. Use Go 1.27.4 to reproduce the recorded toolchain:

```sh
go test -run '^$' -bench '^(BenchmarkReads|BenchmarkConditions)$' \
  -benchmem -benchtime=500ms -count=3 -cpu=1
```

The recorded harness SHA-256 is `74ef51051bc019d580eadc0257c67e59db0ec5d38f02a15f93d9a57b735c6221`. Baseline and cache output hashes are `5d48f87af3b5ec45ca63b62da579303c461d62a2891ec1625334813736d5a05e` and `c1e27a9a5a91d6604a8721ba60596065121d7a1543ca7a4d696ac4ed4490cbb9`.

These are sequential in-process measurements on one host. They exclude concurrent refresh and hosted SDK network latency. Container integration, race checks, source-failure recovery, and independent consumer installation are separate readiness evidence.
