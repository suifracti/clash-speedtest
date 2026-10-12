# Windows Stage 0 storage observations

Standalone Windows-only module, with no CST/SQLite imports. All mutations are
under `t.TempDir()`, including ACL and junction fixtures. No volume flush,
elevation, account creation, credential collection or physical power loss.

Run on actual Windows, preferably a standard non-admin local NTFS user:

```powershell
go test -json -count=1 -timeout=90s . > stage0-results.jsonl
```

TokenElevation/effective admin membership are recorded. Elevated/admin hosted
runners are exploratory only, not ordinary-user acceptance. ACL SID is redacted.
A test PASS means observation collection completed, NOT durability GO. Expected
API failures are recorded. Verdict stays BLOCKED until a separate review establishes
the existing directory-entry durability contract. No compatibility layer included.

Matrix: read/write file flush; WRITE_THROUGH file creation; parent/new-child handles
with 0/READ/WRITE/READ|WRITE x share0/3/7 x optional WRITE_THROUGH (always directory
BACKUP_SEMANTICS|OPEN_REPARSE_POINT); private root/child ACL-owner readback; hard-link
no-replace; DELETE share and close order; nonempty removal; same-volume write-through
rename/no-replace; junction create/no-follow/readback/removal.

API success, file flush, directory-entry durability and power-loss recovery are
separate evidence levels. Official interpretation:
- [FlushFileBuffers](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers): writable handle; volume flush needs admin.
- [CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew): directory/share/reparse/write-through rules.
- [MoveFileExW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw): not blanket parent-directory fsync proof.
- [CreateHardLinkW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createhardlinkw): NTFS/same-volume file links.
