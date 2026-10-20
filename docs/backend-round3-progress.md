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
- Windows CI exposed a URI portability error in the first B1.1 commit: `file://C:/...` made the drive an authority. The shared encoder now emits `file:///C:/...` for both normal opens and read-only reports. A hostless-drive regression fails before this fix and passes after it; Windows CI now runs the connection/report tests explicitly. The repair still requires its new Windows CI result.

## Next

B1.2 transactional cleanup and B1.3 cancellable usage refresh without network requests under the profile write lock. B2–B6 and backend support for subscription diffs, recommendations and native notifications remain pending.
