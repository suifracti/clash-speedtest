# Third-round backend progress

Scope and order: [roadmap-round3.md](roadmap-round3.md). Tests use synthetic databases and local HTTP/TLS fixtures; they are not live node detection evidence. Production history is not migrated or repaired by this work.

## CI repair

- IPPure transport test now generates a local certificate with a controlled hostname and explicit trust roots. Covers trusted TLS, unknown authority and wrong hostname, while asserting the original node dialer, target identity and HTTP/2 are preserved. All fixture sockets terminate at the loopback test server.
- The original Linux failure was a hostname error from the local `example.com` fixture, not a real request to the public API. macOS returned a different certificate error, hiding the assertion problem locally.
- Added `go vet ./...` to the Linux CI job before race tests.
- Local checks: `go vet ./...` and `go test -race ./...` pass with `SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.5.sdk`. The default macOS 27 SDK is incompatible with the installed linker. [GitHub matrix for d093caa](https://github.com/suifracti/clash-speedtest/actions/runs/39401115759) passed all three jobs.

## B1.1 SQLite connection settings and inspection

- `foreign_keys(1)`, `busy_timeout(5000)` and `synchronous(NORMAL)` are applied through repeated DSN `_pragma` parameters on every connection. File URI encoding preserves paths with spaces and reserved characters. WAL activation remains after schema recognition, so unknown schemas are still refused before persistent changes.
- Read-only report: `go run ./cmd/history-orphans -db /path/to/history.db`. It opens an existing file with `mode=ro` and `query_only`, bypasses migrations, and emits aggregate declared foreign-key violations plus legacy monitor samples without a run. It has no repair/delete operation and emits no row contents.
- Validation: four simultaneously held connections all have the expected settings and all cascade child deletion. Synthetic orphan fixtures report the expected three violations without changing database bytes or creating artifacts; missing files are not created. `go vet ./...` and `go test -race ./...` passed with the same SDK setting above.
- Windows CI exposed a URI portability error in the first B1.1 commit: `file://C:/...` made the drive an authority. The shared encoder now emits `file:///C:/...` for both normal opens and read-only reports. A hostless-drive regression fails before this fix and passes after it; Windows CI now runs the connection/report tests explicitly. [GitHub matrix for 1dec197](https://github.com/suifracti/clash-speedtest/actions/runs/39403109236) passed all three jobs, including the Windows connection/report tests.

## B1.2 Atomic workbench cleanup

- Latency tests, batches, public-service attempts and download attempts are pruned in one transaction. Execution, affected-row and commit errors are returned; failed cleanups report zero committed deletions and roll back all tables.
- Old terminal attempts with `finished_at IS NULL` use `requested_at`. Live queued/running/cancelling work, saving/pending persistence and batches with active items are preserved. Counts describe deleted parent records; children cascade.
- Synthetic tests cover old interrupted records, recent/active/saving/late-finished records, cascades, cancellation, and a third-table delete error that must roll back earlier deletes. `go vet ./...` and `go test -race ./...` passed. No cleanup was run against user history.
- [GitHub matrix for fc16f63](https://github.com/suifracti/clash-speedtest/actions/runs/39404189012) passed all three jobs.

## B1.3 Daily usage collection lifecycle

- A claimed collection always attempts to persist a terminal state using a separate five-second cleanup context. Cancelled requests now leave `refresh.state="cancelled"` and a message, rather than `running`; cancellation errors are still returned.
- Up to four independent origins fetch concurrently outside `profileWriteMu`. Accounts on one origin remain serial so a first HTTP 429 activates cooldown before another request. Snapshot and cache/store/history apply remain locked; the existing rollback path is reused.
- Apply checks the source fingerprint and last-update time again, so a changed URL or newer completed refresh wins over an older in-flight response. Existing manual refresh entry points retain their behavior.
- Local HTTP fixtures prove cancellation finalization, free profile editing during fetch, provider overlap, stale-source/newer-counter protection and same-origin cooldown. Focused race tests, `go vet ./...` and full `go test -race ./...` passed. No real subscriptions or nodes were requested.

- [GitHub matrix for 9a29ffd](https://github.com/suifracti/clash-speedtest/actions/runs/39405127807) passed all three jobs. The following compact-summary UI change [6cf7517](https://github.com/suifracti/clash-speedtest/actions/runs/39406101066) also passed the full matrix.

## B2.1 Partial usage header compatibility

- Usage fields are parsed independently. Decimal and scientific notation are accepted and fractional bytes/seconds are truncated. The last valid duplicate wins; invalid repeats do not erase valid fields. Repeated lines of one header are parsed together; a more complete metadata header can supersede an incomplete standard header.
- Subscription DTO `usage.upload` and `usage.download` are now `number | null`. `null` means unknown; numeric zero remains known. Existing numeric profile JSON remains readable. `usage.raw_header` retains the selected original header for local diagnosis, with repeated lines separated by a newline. It is not copied to usage history or logs. Nonempty malformed headers remain diagnostic objects with unknown counters.
- History snapshots use `status="partial"` when both counters cannot be combined. Known individual counters and total remain available, but a partial snapshot does not create a daily increment. Existing explicitly identified notice-balance accounting remains available. The overview and account ledger show unknown instead of coercing null to zero or drawing a false meter.
- Validation: parser cases, repeated headers/metadata, temporary-store/history persistence and UI null regressions pass. Full `go vet ./...`, `go test -race ./...`, 357 frontend tests and the production frontend build passed. No real subscriptions, node probes or user-history writes were used.

## Next

B2.2 header-only alternate-UA fallback, then per-subscription UA and per-account usage-only refresh. B3–B6 and backend support for subscription diffs, recommendations and native notifications remain pending.
