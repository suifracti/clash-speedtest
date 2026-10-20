# Third-round backend progress

Scope and order: [roadmap-round3.md](roadmap-round3.md). Tests use synthetic databases and local HTTP/TLS fixtures; they are not live node detection evidence. Production history is not migrated or repaired by this work.

## CI repair

- IPPure transport test now generates a local certificate with a controlled hostname and explicit trust roots. Covers trusted TLS, unknown authority and wrong hostname, while asserting the original node dialer, target identity and HTTP/2 are preserved. All fixture sockets terminate at the loopback test server.
- The original Linux failure was a hostname error from the local `example.com` fixture, not a real request to the public API. macOS returned a different certificate error, hiding the assertion problem locally.
- Added `go vet ./...` to the Linux CI job before race tests.
- Local checks: `go vet ./...` and `go test -race ./...` pass with `SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.5.sdk`. The default macOS 27 SDK is incompatible with the installed linker. GitHub matrix status must be checked after pushing this repair.

## Next

B1.1 per-connection SQLite pragmas and read-only orphan report, then B1.2 transactional cleanup and B1.3 cancellable usage refresh without network requests under the profile write lock. B2–B6 and backend support for subscription diffs, recommendations and native notifications remain pending.
