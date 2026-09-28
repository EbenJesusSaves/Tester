# Implementation decisions and reasons

The user requirements are multiple apps, `dev`/`uat`/`prod`, security and
performance, clear rationale, UML architecture, and actionable mobile guidance.
This ledger explains each material implementation step. It distinguishes reused
upstream behavior from changes made here.

| Decision / step | Reason | Tradeoff and evidence |
| --- | --- | --- |
| Clone and record the upstream commit | Preserve a reviewable baseline and existing protocol compatibility | Upstream contains commercial code with its own license; no enterprise controls were removed |
| Reuse xprem's protocol, publish CLI and dashboard | Avoid a second incompatible update protocol or admin surface | Improvements must preserve existing client contracts |
| Use PostgreSQL mode for multiple apps | Stateless flat-env mode supports one app; registry and mappings need durable storage | Requires database backup and migration operations |
| Provision per-app dev/uat/prod branches and channels idempotently | Give each client a predictable release lane without overwriting live promotions | Existing mappings remain intentional operator state; bootstrap is not a desired-state reconciler |
| Generate app-specific signing keys in xprem and export only public certificates | Devices verify who may deliver executable code for their app | The server master key and server access are high-value; key rotation requires a client trust plan |
| Reject missing app identity in the deployment | A default tenant hides mobile configuration mistakes | Legacy clients without the header require a new compatible configuration/build |
| Fail closed for invalid explicit legacy asset pins | Substituting a newer or incompatible bundle breaks manifest integrity and can cross release boundaries | Broken legacy URLs now get 404; tests cover app/runtime/platform/channel mismatches |
| Include platform, asset role, extension and origin in legacy metadata cache identity | Same-named iOS/Android assets and different deployments must not reuse metadata/URLs | New namespace causes one cold-cache cycle; isolation tests cover warm caches |
| Stream legacy asset hashing with bounded concurrency | Large assets and asset counts should not multiply memory and goroutines without bound | Legacy hashing still consumes I/O; CAS/CDN remains the preferred delivery path |
| Restrict delta patches to the exact launch bundle and compatible app/platform/runtime | A forwarded delta header must not turn an image or another bundle into patch bytes | Bundle diffing stays disabled in the default deployment |
| Use no-store for manifests, branch lists, redirects, failures and auth responses | Device-dependent decisions and session tokens must not be reused by intermediaries | API load is handled through internal caches rather than shared manifest caching |
| Vary immutable asset responses by selection and encoding headers | `/assets` identity is partly in headers; URL-only caching can serve another app's bytes | Configure CDN caching on app-scoped object URLs for better cache efficiency |
| Bound and validate manifest selectors and supported protocol versions | Reject malformed/oversized input before repository/cache work | Existing valid IDs, runtime strings and protocol versions 0/1 remain supported |
| Omit request headers, queries and panic values from HTTP logs | New credential headers and OAuth query values defeat partial redaction lists | Logs retain method/path/status/duration and panic stacks; fewer allocations and log writes |
| Drain HTTP before closing dependencies | Restarts should allow active finalization and downloads to finish | Requests exceeding 30 seconds during termination can be interrupted and must retry |
| Bind the listener before storage initialization | A port conflict should not run migrations before failing | Liveness answers during migrations; readiness remains distinct |
| Limit request headers to 32 KiB and retain the header-read timeout | Bound unauthenticated memory/time spent accepting headers | Unusually large cookie/header sets will be rejected |
| Use generated local secrets and a hardened private Compose network | Make local setup reproducible without committing credentials or exposing databases | Local Compose HTTP is for loopback; production requires TLS ingress and managed secrets |
| Exclude secrets and local artifacts from Docker context | Ignoring secrets in Git alone does not stop them entering container build contexts | Changes to local secret files no longer affect images |
| Share Redis across replicas and offload immutable bytes to CDN/object storage | Reduce database reads, signing work, API egress and per-instance inconsistency | Shared infrastructure must be monitored; cache loss increases origin load |
| Keep mobile checks foreground-only, throttled and jittered | Bound battery/network costs and spread fleet-wide bursts | Foreground polling is eventual; there is no promise of immediate background updates |
| Download first, then prompt at a safe point | Users should not lose work when a new bundle becomes available | The product decides when restarting is acceptable |
| Supply reproducible tests and load tooling | Security/performance claims need evidence tied to this implementation | Synthetic/local results do not establish production fleet capacity |
| Raise the Go patch level and update gRPC after vulnerability scanning | The imported baseline had reachable standard-library and HTTP/2 vulnerability findings | Go remains on the 1.26 release line; the regression suite and scan must pass after updates |
| Add CI checks for reachable vulnerabilities and mobile examples | New dependency disclosures and example API drift should fail validation early | Vulnerability data is current at scan time; production image and infrastructure scanning remain separate |

See [validation](validation.md) for executed checks and remaining environment/device
work, and [security and performance](security-performance.md) for acceptance criteria.
