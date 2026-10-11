# W11b: automatic v11 pre-upgrade SQLite gate

2026-10-11; macOS arm64; Go 1.25.14. Offline implementation by Sol B, stacked on integration [PR #48](https://github.com/suifracti/clash-speedtest/pull/48), locked base `5094e54b045a359a83447d08a23dcb6d24be9b7d`. Reuses [W11a PR #42](https://github.com/suifracti/clash-speedtest/pull/42), locked head `91afa7b38e980d874dbb3355ac091993e777e895`. Its history DB/backup code is identical at this base. The source integration branch was never checked out or modified.

## Trigger and resulting behavior

Before this change, opening an existing v11 DB immediately applied v12 DDL. Even `InspectMigration` can open the database early through `ValidateDatabase`; backing up only before Store creation misses that path.

`OpenDB` now supplies a mandatory pre-upgrade callback to the private schema authority. Once an existing v11 schema and budget ledger are recognized, the existing migration read transaction remains open while the callback takes and verifies a rollback snapshot. No v12 DDL runs until backup, verification, checksum, file sync, manifest publication and directory sync succeed. Backup errors are wrapped in `ErrUpgradeBackup`; nil callbacks cannot bypass the gate. Failure rolls back the migration transaction. No new schema tables or columns were added; existing v12 DDL is unchanged.

Only databases whose version is already 11 at entry take this gate. New databases and v12 reopen retain their existing behavior. No broader pre-upgrade backup claim is made for existing v1–v10 or unversioned databases.

The gate covers direct OpenDB, ValidateDatabase, NewStore and NewStoreWithLegacyDir, including the early canonical/legacy inspection path and native-to-web retry through the same authority. Cross-package startup code is unchanged; InspectMigration itself remains a migrating operation.

## Backup format and fail-closed boundary

Snapshots are under the **actual history directory passed to OpenDB**, including legacy or staging roots:

```
.schema-upgrade-backups/                 0700, regular directory; symlinks rejected
  v11-to-v12-<unique-attempt>/           0700
    history.db                         0600
    complete.json                      0600
```

Each attempt is fresh. W11a's online backup opens the source read-only, includes committed WAL pages and never migrates the source. The helper validates the snapshot using `mode=ro`, schema version exactly 11, existing v11 table/budget checks, absence of the v12 column, `integrity_check=ok` and no FK violations. It never calls OpenDB or ValidateDatabase on rollback. The manifest records format/from/to versions, relative filename, SHA-256 and UTC creation time, with no history records or subscription secrets.

The snapshot and private manifest are synced; the completion manifest publishes via no-replacement hard link. Attempt/root/history directory syncs must also succeed before the callback returns. Failures delete only the new incomplete attempt; earlier completed snapshots survive. Retrying v11 always takes a fresh snapshot; reopening v12 does not create another. There is no automatic pruning, production restoration, old-snapshot reuse or app-level recovery decision.

The read transaction also guards concurrency: if another WAL writer commits before or after backup, promotion of this transaction to a writer returns `SQLITE_BUSY_SNAPSHOT` (517). The caller must retry the whole OpenDB operation, which takes a fresh snapshot, rather than retry DDL within a stale transaction. Two concurrent schema authorities either upgrade only with completed backups or return safe errors. No lock/DDL retry bypass was added.

## Red → green evidence

All new SQLite/WAL operations use pure synthetic historical-v11 fixtures inside `t.TempDir()`. The fixture replays historical DDL only through v11, retains three samples and full identity, and holds a writer with WAL autocheckpoint disabled. Sample 3 is committed from 39 to 47 only in WAL. Existing W11a tests retain the naked-main-file negative control (39).

- [RED log](w11b-v11-upgrade-gate/red.txt): contract tests against unchanged PR48 production failed with zero pre-upgrade backups and backup failure not blocking upgrade; exit 1. This was a behavioral failure, not a compilation error.
- [GREEN log](w11b-v11-upgrade-gate/green.txt): complete v11 gate suite passed; exit 0. Copy, malformed SQLite, wrong version, v12 column, FK violation, snapshot sync, manifest write/sync/publication, directory sync, cancellation, non-directory, symlink and permission failures block DDL. The active writer remains open while failed-gate source DB/WAL SHA-256 equality is checked. Permission case ran non-root, no skip.
- Verified WAL-only snapshot SHA-256 for the sample=47 fixture: `a1d728006cb10ee985cbe5e72e28d9d5f0ee2e29f2b9701c5b0418e9372fccb1`. These hashes identify synthetic test contents only, not user data.
- Isolated rollback test restores a snapshot into another t.TempDir, verifies v11/integrity/identity/samples, upgrades only that separate candidate, then rechecks the original rollback remains valid v11. v12 reopen adds no backup.
- Concurrent commit tests (before and after backup) observe lock code 517 and source remains v11. Whole-operation retry upgrades to12 using a new snapshot containing sample=53, without overwriting the prior completed snapshot. Concurrent migrators also pass.
- [Package log](w11b-v11-upgrade-gate/packages.txt): full history/application/appdata tests pass (2.370s/25.142s/0.564s), exit0.
- [Race log](w11b-v11-upgrade-gate/race.txt): new gate + W11a backup + schema-v11 subset pass under race detector (8.647s), exit0.
- `go vet` and `go build` for the same three packages and `git diff --check` exit0.
- Independent read-only reviewer found no actionable P1/P2 findings and separately ran the synthetic gate suite successfully. Later changes only added manifest write/publication failure cases and corrected evidence logging; production code was unchanged.

Commands (GOPROXY=off and GOSUMDB=off on all Go checks):

```sh
go test ./core/history -run TestV11UpgradeBackup -count=1 -v
go test ./core/history ./application ./core/appdata -count=1
go test -race ./core/history -run 'TestV11UpgradeBackup|TestBackupDatabase|TestBackupToInterrupted|TestSchemaV11Upgrade' -count=1 -v
go vet ./core/history ./application ./core/appdata
go build ./core/history ./application ./core/appdata
git diff --check
```

## Exact scope and remaining acceptance

Production: `core/history/db.go` and new `core/history/upgrade_backup.go`. Tests: new `core/history/upgrade_backup_test.go`. Evidence: this document and the four linked logs. No application/adapter/main/gui/subscription/VPN/TUN files were changed. Existing schema DDL is unchanged. No original dirty candidate, real user DB/WAL, credentials, live probe, release or production rollback was touched.

This is macOS offline evidence. Other platforms/filesystems, a real old v11 executable, abrupt termination and power-loss recovery remain unvalidated. Unsupported permission semantics, hard links or directory Sync fail closed. The 30-second context is cooperative, not a hard real-time deadline. Snapshot consistency covers SQLite, not a simultaneous settings/profile bundle. Native/web UI and remote CI are separate acceptance layers. This Draft PR does not authorize production-data startup, merge or release.
