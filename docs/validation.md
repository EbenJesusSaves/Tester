# Validation record

Validated on 2026-09-26 against this working tree, based on upstream
`b46e13569f5734a66ed903f75f51b87b78e80c1b`. The host was macOS arm64 / Apple M4,
Go 1.26.0 and PostgreSQL 17.6. No production service was deployed or mobile binary
installed.

## Executed checks

| Check | Result | Scope |
| --- | --- | --- |
| `go test ./...` with `TEST_DATABASE_URL` | Passed: 46 packages; 14 had no tests | Includes PostgreSQL-backed tests and upstream Go regression suite |
| `go test -race ./cmd/api ./internal/cache ./internal/update ./internal/assets ./internal/middleware ./internal/handlers ./internal/services` | Passed | Changed delivery, lifecycle, cache and handler paths under race detection |
| `go vet ./...` | Passed | Go static analysis |
| Mobile `npm run typecheck` | Passed | Actual Expo 57.0.25 / expo-updates 57.0.23 / React Native 0.86.3 types |
| Mobile `npm test` | Passed: 7 tests | Concurrent check deduplication, rollback staging, retry limits, pending download handling and reload failure |
| Python compilation, shell syntax and `docker compose … config --quiet` | Passed | Setup/bootstrap/smoke/E2E scripts and Compose syntax |
| Live API bootstrap against isolated PostgreSQL | Passed | Two apps, six branches/channels; repeat run retained IDs and certificates |
| `ota-smoke.py` against the live server | Passed | Twelve app/channel/platform combinations, readiness, missing/unknown app and anonymous management rejection |
| `ota-e2e.py` against the live server | Passed | Twelve real fixture publications, verified signed responses, downloaded asset hashes, wrong-app token and blob rejection |
| `git diff --check` | Passed | Patch whitespace |

The live API tests used the built Go executable, local asset storage, and the
in-memory cache. Their isolated PostgreSQL cluster listened on loopback port
54329; the API used loopback port 30329. Docker's daemon became unresponsive, so
the complete Compose runtime and shared Redis path were not validated end to end.
The Compose file parsed successfully. Helm template tests were skipped because
the Helm executable was unavailable. ClickHouse and Azure emulator integration
tests were skipped because their service URLs were not configured; those are
outside this deployment's default local-storage/no-device-telemetry profile.

The E2E script creates two disposable applications with independent signing keys
and publishes distinct synthetic content to dev, uat and prod on both platforms.
It verifies the exact multipart manifest bytes using OpenSSL RSA/SHA-256,
rejects a tampered manifest, compares returned update UUIDs, downloads each bundle
and asset, and verifies its SHA-256 and unique app/channel/platform marker. It then
checks that one application's publisher token cannot publish to another app and
that the other app cannot read the first app's content-addressed blob. The script
deletes only the disposable apps it created.

These are protocol fixture bundles, not runnable mobile apps. Signature verification
with OpenSSL establishes the wire signature is correct; native SDK behavior still
needs the real-device matrix in the [mobile guide](mobile-client.md).

## Measured hashing memory

Command:

```sh
go test ./internal/update -run '^$' -bench BenchmarkLegacyAssetHashing -benchmem -benchtime=500ms
```

The benchmark forces cold metadata cache reads against a warm local filesystem.
It includes cache reset/encoding overhead and measures legacy manifest asset
hashing, not HTTP throughput or device download latency.

| File size | Time / operation | Throughput | Allocated bytes / operation | Allocations |
| --- | ---: | ---: | ---: | ---: |
| 64 KiB | 0.136 ms | 481 MB/s | 36,771 | 51 |
| 1 MiB | 1.768 ms | 593 MB/s | 36,768 | 51 |
| 8 MiB | 15.391 ms | 545 MB/s | 36,805 | 51 |

Allocation remains approximately constant as asset size grows. This supports the
streaming-memory change; it does not establish a production request-rate limit.
The provided k6 workload and the [performance acceptance criteria](security-performance.md)
are ready for a representative staging environment. Fleet-scale load, CDN hit
rate, replica behavior, failure recovery and geographic latency have not been measured.

## Reproduce the protocol checks

Start a **disposable local-storage** server using the [operations guide](operations.md).
Use its matching bootstrap admin password file, then run:

```sh
python3 scripts/ota-bootstrap.py --admin-email ota-admin@example.test
python3 scripts/ota-smoke.py
python3 scripts/ota-e2e.py --admin-email ota-admin@example.test --disposable-server
```

The E2E script requires `openssl`, creates/deletes fixture apps, and refuses a
remote server. Use `--url` and `--password-file` for a different disposable local
instance. Never point fixture publishing at a real mobile application's app ID.

For Go database tests, set `TEST_DATABASE_URL` to an isolated test database with
permissions to create schemas/extensions, then run the Go commands above. Do not
use production: integration tests mutate and reset their test data.

For the mobile examples:

```sh
cd examples/mobile
npm ci --ignore-scripts
npm run typecheck
npm test
```

## Remaining release work

- Deploy the approved image behind HTTPS, provision shared storage/CDN/Redis and
  the production access policy, and rerun protocol tests in an isolated staging
  deployment with the same infrastructure settings.
- Run Android and iOS signed release builds through update, tamper, interruption,
  offline, native-runtime mismatch, safe restart and rollback scenarios.
- Run the recorded load/failure scenarios, restore a backup with the original
  master key, and approve measured latency/error/capacity limits.
- Validate licensed authorization capabilities if required, or use separate
  deployments when production publishing must be isolated from development staff.
