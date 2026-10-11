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

The snapshot and private manifest are synced; the completion manifest publishes via no-replacement hard link. Attempt/root/history directory syncs must also succeed before the callback returns. Failures attempt to delete only the new incomplete attempt; cleanup errors are preserved and reported. Where permitted, residual attempts are quarantined under `.failed-*`. If cleanup and quarantine are both denied, the original path can remain and is reported explicitly; a residual manifest alone never proves gate success. Earlier completed snapshots survive. Retrying v11 always takes a fresh snapshot; reopening v12 does not create another. There is no automatic pruning, production restoration, old-snapshot reuse or app-level recovery decision.

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


## Sol D follow-up: post-publication cleanup denial

Requested by [the continuation contract](https://github.com/suifracti/clash-speedtest/pull/50#issuecomment-6106171749), fixing [Sol D's independently reproduced Relevant](https://github.com/suifracti/clash-speedtest/pull/50#issuecomment-6106115134) on baseline head `114296fb985b90b59f89540757d680f85016e4c6`. Same PR50 branch; PR48 base remains `5094e54b045a359a83447d08a23dcb6d24be9b7d`. The prior unconditional cleanup description was too strong.

The original callback published `complete.json`, then a synthetic final attempt-sync failure combined with real OS chmod0500 prevented deferred unlink. The source was correctly protected but cleanup errors were discarded. The regression uses only synthetic v11+WAL data in t.TempDir and a non-root runner; directory Sync failure is injected, unlink permission denial is real.

The minimal fix changes only upgrade_backup.go, its tests and evidence. Deferred cleanup joins the cleanup error with the original gate error. If cleanup fails, it reserves a unique `.failed-*` name in the same backup parent and renames only the current failed attempt. The temporary empty name reservation belongs to this call; only that empty reservation is removed. This same-parent move avoids macOS denial when an unwritable attempt is moved into a different parent. No production chmod, previous-backup deletion, db.go or schema/startup changes were made.

**Completion boundary:** `complete.json` records a verified snapshot, not proof that the entire directory-durability gate returned success. `.failed-*` paths are failed attempts and excluded from ordinary attempt enumeration. If the root is also unwritable, quarantine may fail and the original attempt/manifest may remain: both failures and the residual path are returned. Permission restoration must not treat that manifest as authority. Whole OpenDB retry always creates a fresh snapshot and does not modify/reuse/delete prior failed artifacts. No automatic cleanup/restore of retained artifacts is introduced.

Fresh evidence on macOS27.0.1/arm64, Go1.25.14, uid501 (non-root):

- [Cleanup RED](w11b-v11-upgrade-gate/cleanup-red.txt): against unchanged baseline production, `TestV11UpgradeBackupCleanupPermissionFailure` fails 3/3, `cleanup permission failure was swallowed`; exit1. Source schema11 and DB/WAL hashes are checked before the failing assertion.
- [Cleanup GREEN](w11b-v11-upgrade-gate/cleanup-green.txt): both real-permission tests pass 3/3 each; exit0 (1.170s). Original case is quarantined and cleanup denial exposed. Root+attempt denial preserves both error causes and original residual path without changing permissions. Both tests verify source v11/DB/WAL hashes before retry; after test-only permission restoration, whole retry reaches12 with a fresh snapshot. Prior valid backup hash is preserved, and quarantined/retained failed artifacts are not removed or reused.
- [Fresh package log](w11b-v11-upgrade-gate/cleanup-packages.txt): full history/application/appdata tests exit0 (2.267s/26.135s/0.474s).
- [Fresh race log](w11b-v11-upgrade-gate/cleanup-race.txt): cleanup, original W11b gate, W11a online backup and v11-schema regressions exit0 (10.080s).
- Vet for the same three packages and diff checks exit0. No new app bundle or native UI claim. Prior build evidence remains historical, not a newly run build for this follow-up.
- Independent read-only reviewer reran the cleanup/dual-denial/pre-DDL cases and found no actionable P1/P2. This is a local review; it does not claim Sol D has reviewed the new head.

Commands, with GOPROXY=off GOSUMDB=off:

```sh
go test ./core/history -run '^TestV11UpgradeBackupCleanupPermissionFailure$' -count=3 -v # RED before production fix
go test ./core/history -run '^TestV11UpgradeBackupCleanup' -count=3 -v # GREEN
go test ./core/history ./application ./core/appdata -count=1
go test -race ./core/history -run 'TestV11UpgradeBackup|TestBackupDatabase|TestBackupToInterrupted|TestSchemaV11Upgrade' -count=1 -v
go vet ./core/history ./application ./core/appdata
git diff --check
```

These are synthetic error/permission experiments, not physical power-loss tests. Quarantine sync errors also remain observable; the helper cannot guarantee artifact removal or durable quarantine under arbitrary I/O/permission failure. Source DDL remains blocked on every failed gate. PR50 is stacked and has no Actions/checks observed at follow-up delivery; remote CI is not reported as passing. No real SQLite/WAL, dirty candidate, PR48/main/other-agent branch change, subscriptions/probes/VPN/TUN, merge or release.


## Windows Relevant follow-up (Sol B, 2026-10-11)

Scope: test/platform evidence only, following task [6106558257](https://github.com/suifracti/clash-speedtest/pull/50#issuecomment-6106558257) and Sol D [second review](https://github.com/suifracti/clash-speedtest/pull/50#issuecomment-6106493480). Production `db.go` and `upgrade_backup.go` remain byte-identical to verified head `897ebc09b8b474dddc06a4a2bbaab2ea8b69260d`. Prior macOS cleanup RED/GREEN evidence above remains applicable.

Exact test difference:
- Move the two real chmod cleanup regressions from `upgrade_backup_test.go` to `upgrade_backup_permissions_unix_test.go`, selecting darwin/linux/freebsd/netbsd/openbsd/dragonfly. Preserve their cleanup/quarantine/retry assertions. A non-root Unix capability probe uses its own `t.TempDir()` and requires actual `os.ErrPermission` on a write to a 0500 directory; unsupported runners fail visibly instead of silently skipping.
- Add `upgrade_backup_windows_test.go`, Windows-only. Using only synthetic v11 + committed WAL in `t.TempDir()`, it checks the normal writable root's 0777 mode and two failed OpenDB attempts: explanatory ErrUpgradeBackup, schema11, unchanged DB/WAL hashes, committed sample47/all three samples, absent v12 column, empty backup root. This regression documents current refusal, not successful Windows upgrade support.

Evidence: `w11b-platform-registration.txt` records the failing baseline and passing updated Windows test registration assertion from actual cross-compiled `_testmain.go`: Unix-only regressions were incorrectly registered before, and are absent after; the Windows-specific regression is now registered. This is a static build-selection RED/GREEN, NOT a Windows runtime RED/GREEN. The prior real macOS cleanup RED remains in `w11b-cleanup-red.txt`. `w11b-platform-unix.txt` is actual macOS non-root execution (both regressions, count3). `w11b-platform-packages.txt` and `w11b-platform-race.txt` are actual macOS package/race executions. Windows amd64 test binary cross-compilation succeeded; **Windows runtime is NOT verified** because no Windows host/runner is available in this task. The targeted Windows command prepared for an authorized runner is:

```sh
go test ./core/history -run '^TestV11UpgradeBackupWindowsUnsupportedPrivateRootBlocksDDL$' -count=1 -v
```

Do not report the full Windows history suite as green: shared successful-upgrade tests require a working platform backup implementation and currently encounter this functional gap. No Windows execution or physical durability claim is made. Existing CI does not run on this stacked PR base; its Windows job builds but does not execute these regressions.

### Separate Windows functionality gap

Installed Go 1.25.14 sources establish the static behavior: `os/types_windows.go` (`fileStat.mode`) maps normal writable directory attributes to 0666 + directory 0111 = 0777, or read-only to 0555. `os/file.go` documents that Windows Chmod only uses 0200 to control the read-only attribute; it neither establishes nor verifies owner-only ACLs. `syscall/syscall_windows.go` returns -1 from Geteuid. Therefore the unchanged production exact `root.Mode().Perm()==0700` check rejects every normal Windows v11 first upgrade, including directories whose ACLs might already be private. A safe refusal protects data but does **not** provide usable Windows first-upgrade support. Snapshot exact0600 validation is a further blocker after root support is added.

Minimal future implementation proposal, requiring separate authorization:
1. Add platform-specific private directory/file creation and verification. Keep Unix mode behavior; on Windows use an explicit owner-only DACL and verify ACL/owner, rejecting unsafe inherited access and reparse points. Cover both root and snapshot/manifest private checks; do not merely bypass 0700/0600 on Windows.
2. Qualify Windows file FlushFileBuffers and directory durability behavior on the intended filesystem. Define publication/no-replace and same-volume quarantine semantics; unsupported durability must remain an explanatory gate error. Preserve the joined original/gate/cleanup error tree and observable residual path; never chmod/ACL-repair user data during failure cleanup.
3. Run real Windows synthetic v11+WAL fixtures for successful verified backup and isolated rollback, committed WAL pages, ACL denial of deletion/rename/parent creation, final sync/publication failure, unchanged source schema/hash, stale complete.json rejection, and fresh retry after a new sample53. Restore permissions only on test fixtures. Cross-compilation cannot substitute for these tests.

Commands actually used on macOS (Go 1.25.14 darwin/arm64, uid501):

```sh
go test ./core/history -run 'TestV11UpgradeBackupCleanup' -count=3 -v
go test ./core/history ./application ./core/appdata -count=1
go test -race ./core/history -run 'Test(V11Upgrade|Backup)' -count=1
go vet ./core/history ./application ./core/appdata
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOPROXY=off GOSUMDB=off go test -c -work ./core/history -o <local-only-test-binary>
git diff --check
```

No production SQLite/WAL, PR48/main, dirty candidate, subscription/probe/VPN/TUN, workflow, merge or release change. Windows safety acceptance remains open pending real runtime evidence and separately authorized functionality work.
