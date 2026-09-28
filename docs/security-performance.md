# Security and performance acceptance

Security and delivery latency are release criteria. This document defines what is
protected, what operators must configure, and how to measure the result. It does
not convert upstream benchmark numbers into measured results for this deployment.

The dependency review found reachable vulnerabilities in the imported Go 1.26.0
standard library and gRPC 1.82.1. The minimum toolchain and Docker build stages now
use Go 1.26.8, and gRPC is upgraded to 1.83.1. The patch choices are based on the
[official Go releases](https://go.dev/dl/) and the
[gRPC HTTP/2 memory-exhaustion advisory](https://pkg.go.dev/vuln/GO-2026-6348).
The added CI workflow checks reachable Go vulnerabilities and validates the mobile
example against its lockfile. This is not an operating-system/container-image or
mobile supply-chain audit; scan those artifacts in the production release pipeline.

## Threat model and controls

| Threat | Control | Verification / residual boundary |
| --- | --- | --- |
| App A receives App B metadata or bytes from a cache | App-scoped repository/object keys, complete cache keys, HTTP Vary and no-store | Automated multi-app warm-cache regression matrix |
| Wrong channel or native runtime receives an update | Explicit app/channel mapping and runtime/platform match; no invalid-pin substitution | Two apps × three channels × two runtimes × two platforms in service tests |
| Bucket/CDN content is replaced | Native manifest signature plus signed asset SHA-256 hashes | Install a signed native build and test tampering before release |
| Attacker alters unsigned distribution hints | Hints never bypass the signed manifest/native verification | App/channel headers and stable client IDs are not authentication or secrets |
| Publishing token is stolen | App-scoped credentials, protected CI secrets, private control plane, token revocation | Core token access spans app branches; use licensed scoping or independent deployments for stricter publisher boundaries |
| Signing key or master key is stolen | Per-app keys sealed in DB; master key in secret manager; minimal runtime privileges | A compromised signing server can issue updates; isolate access and retain a key recovery plan |
| Password guessing | Upstream account/IP throttling plus ingress limits | Cache-backed throttling fails open on cache errors; the independent ingress control matters |
| Resource exhaustion | Header size/deadline, container memory/process limits, bounded hashing, external request/body limits | Large asset serving still uses memory in local mode; production should redirect to CDN |
| Secrets exposed through logs/builds | HTTP logs omit headers/queries/panic values; Docker excludes local credentials | Review provider and infrastructure logs too; do not enable verbose credential logging |
| Rollout/remap stale state | Shared Redis invalidation and documented TTLs | Offline clients and client polling add delay; measure rollback convergence |

TLS is mandatory outside localhost. Only delivery endpoints need public access.
Place `/api`, `/auth`, publisher write paths, OAuth/MCP administration and `/metrics`
behind the appropriate private ingress/access policy. If a dashboard must be
Internet-accessible, put the actual organizational identity and access policy in
front of it; a hidden URL is not a control. Configure request limits separately for
small auth/manifest requests and large uploads. Do not trust proxy headers until
the application can only be reached through the trusted proxy chain.

App UUIDs, public certificates and channel names may ship with the application.
Publishing tokens, private signing keys, JWT secrets, database credentials and the
database key-encryption key may not. Avoid user personal data in bundles, labels,
release metadata and client error reports.

## Performance design

The normal manifest path reads cached app identity, channel mapping and the latest
compatible release. Repeated checks for the current update return a small protocol
directive. Manifest signatures are cached by app, private-key fingerprint and exact
content hash, so key rotation cannot accidentally reuse an old signature entry.
The exact bytes of the manifest are signed.

Content-addressed storage deduplicates payloads within an app. A CDN makes bytes
independent of the Go service's CPU and memory. Direct local asset serving remains
useful for development but is not the fleet-scale topology. Legacy manifest asset
hashing streams reads and limits parallelism; it does not make arbitrary large
asset responses memory-free.

Manifest no-store does not disable the internal shared caches. Do not add a CDN
manifest cache as a shortcut: rollout/device/runtime selection makes an incorrect
cache key a distribution failure. Keep a deployment-specific Redis prefix and
stamp release versions into image builds. Keep PostgreSQL connection pool totals
below the database connection budget across all replicas and workers.

## Initial acceptance targets

These are proposed starting targets, not measurements or contractual guarantees.
Agree fleet size, geography and cost budget before capacity approval.

| Scenario | Initial target | Why |
| --- | --- | --- |
| Warm origin manifest checks | p95 < 100 ms, p99 < 250 ms, errors < 0.1% | Leaves budget for mobile-network latency |
| Cold caches and publish burst | No wrong-tenant response, no OOM, p99 < 1 s | A deploy or release must not make caches a correctness dependency |
| Sustained load | 30-minute steady run without growing memory/connection count | Catch leaks and pool exhaustion |
| Burst load | 3× expected peak for 5 minutes, recover without restart | Model coordinated app launches and foreground checks |
| Rollback/remap convergence | Measure origin TTL + mobile polling delay explicitly | Prevent promising instant recovery to offline devices |
| Integrity / authorization | Zero invalid-signature acceptance, zero cross-app credential success | Correctness has no acceptable error budget |

Use `scripts/ota-benchmark.js` with k6 against provisioned apps. Supply a runtime
with a real signed published release, warm caches, and measure current-update
checks separately from fresh downloads. Run a no-update workload separately; it
does not exercise asset composition or signature verification. Browser/mobile
TLS/network latency is excluded by an origin-only benchmark.

Record commit/image digest, tool versions, CPU/memory, replica count, storage/CDN,
dataset size, cache mode, signing status, concurrent clients, offered request rate,
achieved request rate, warm-up period, duration, p50/p95/p99, 4xx/5xx, CPU, RSS,
Redis hit rate and DB connections. Store results with the release. Upstream
`test/load/` provides additional fleet workload tools; do not compare unlike datasets.

## Production release checks

1. Create the app identity and download its public certificate; build signed iOS and
   Android release binaries with fixed app/channel/runtime configuration.
2. Publish a harmless visible change to dev and verify signature/hash checks and
   each client state transition; repeat for uat with its actual environment.
3. Check missing/unknown app, unknown channel, mismatched runtime/platform,
   wrong-app publishing token, invalid asset pin and anonymous management requests.
4. Tamper with a test asset and signed manifest in a disposable environment; the
   native client must retain the known-good bundle. Restore the original content.
5. Exercise offline launch, interrupted download, a restart during finalization,
   rollout expansion and rollback-to-known-good on both platforms.
6. Load test the intended production topology and dependency-failure behavior;
   verify monitoring, alerting, backup restore and emergency credential revocation.

The local validation record is [validation.md](validation.md). Device execution,
production infrastructure and performance approval require their own recorded evidence.
