# W11a: first-upgrade SQLite/WAL backup gate

Scope: issue #40, stacked on B1 PR #29 at `d65281ab4192f0f2772e5b295154f4ed5489b797`. All fixtures are synthetic and live under `t.TempDir()`. No user database, installation, restoration, network probe, or runtime startup is needed.

`history.BackupDatabase(ctx, sourceDir, targetDBPath)` now opens the existing source with SQLite `mode=ro`, bypassing `OpenDB` and its schema migrations. The online backup includes committed WAL pages. `DB.BackupTo` still backs up an already-open database at its current schema; application ZIP export is therefore **not** proof of a pre-upgrade backup if startup has already upgraded the database.

Snapshots are built in private files in the target filesystem (0600), then published with a hard link that cannot replace an existing destination. A concurrent destination wins safely; failures and context cancellation remove the private snapshot, not an existing backup. Filesystems without hard-link support fail closed. Cancellation is checked between 128-page backup steps; this is not a hard real-time deadline or a power-loss durability guarantee. Abrupt process termination before publication can leave private `.history-backup-*.tmp` artifacts; those are not completed backups. ZIP export writes its central directory only on success. A destination writer must propagate write errors; an export error means discard the partial stream.

## Synthetic evidence

- Baseline targeted schema-upgrade and ZIP restoration tests passed.
- New pre-upgrade regression failed on B1: `backup modified source DB/WAL`.
- New cancelled ZIP regression failed on B1: `failed export has a valid ZIP completion directory`.
- The fixture replays historical migrations only through v11, retains three raw latency samples and full attempt identity, and holds a writer with WAL autocheckpoint disabled. A committed WAL-only update changes sample 3 from 39 to 47.
- Negative control: copying only the main DB reads 39. The online backup reads 47.
- Backup leaves source DB and WAL SHA-256 unchanged while the writer remains open; rollback snapshot remains schema 11 without the v12 column. The v11 schema validator, `PRAGMA integrity_check` and legacy SQL reads succeed.
- A separate snapshot is upgraded by `OpenDB` to schema 12; identity, original sample fields and the committed update survive. The rollback snapshot is checked again and stays v11.
- Existing target, cancellation, denied destination permissions (non-root), missing source, concurrent publishers and an interrupted connection wait fail closed. Subscription/settings ZIP contents and lock/tmp/WAL/SHM/webview exclusions remain covered by the existing synthetic restoration test.

Validation on 2026-10-11: Go 1.25.14, macOS arm64. Targeted tests (all three packages), history race subset, vet and build passed with exit 0. Permission case ran as a non-root user; no platform claims beyond this host. Read-only code review found no P1/P2 defects. Commands:

```sh
go test ./core/history ./application ./core/appdata -run 'TestBackupDatabase|TestBackupToInterrupted|TestDataBackup|TestSchemaV11Upgrade|Test.*Migrat' -count=1 -v
go test -race ./core/history -run 'TestBackupDatabase|TestBackupToInterrupted' -count=1
go vet ./core/history ./application ./core/appdata
go build ./core/history ./application ./core/appdata
```

Not demonstrated: an actual old v11 executable, other platforms/filesystems, power-loss recovery, or real user data. The legacy validator/SQL checks establish schema compatibility, not execution of an old binary. This PR adds the reusable gate; it does not wire an automatic first-upgrade backup into startup or approve installation.

## Offline first-upgrade acceptance checklist

Before any future authorized upgrade:

- Record the intended data root privately, application build/version, and source schema through a read-only connection. Do not log tokens, subscription URLs, keys or user history in public evidence.
- Stop writers for the complete data-root bundle, or establish an explicit consistency boundary for subscription/settings files. The SQLite online snapshot alone does not synchronize independently changing JSON files.
- **Before any `OpenDB`, `NewStore`, startup or schema-validating helper that can migrate**, create the consistent snapshot using `BackupDatabase` at a new destination. Check its error; never treat a bare DB copy or a live ZIP export after startup as the original v11 backup.
- Save subscription/settings files using the established ZIP layout/exclusions or a private offline bundle. Publish completion only after every write and ZIP close succeeds. Record archive/snapshot SHA-256 and verify after transfer; keep contents and hashes private for real data.
- Read the backup without migration: check schema, `PRAGMA integrity_check`, identity, record/sample counts and known committed values. Keep that verified original immutable.
- Restore only into an isolated directory. Upgrade a separate candidate copy and compare preserved data. Test the matching old v11 binary against another unupgraded v11 copy before authorizing rollback.
- A v11 program must never be pointed at the upgraded v12 database. Rollback selects the verified unupgraded copy plus matching files; it does not rewrite schema metadata to 11.
- Any backup, checksum, restore or write failure blocks upgrade. No real upgrade/rollback is authorized by this offline PR.
