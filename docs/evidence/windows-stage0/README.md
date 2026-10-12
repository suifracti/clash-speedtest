# Stage 0: BLOCKED — ordinary-user directory durability not established

Authorization: [PR50 Stage0 task](https://github.com/suifracti/clash-speedtest/pull/50#issuecomment-6107098266), explicitly forwarded by user.

Executed probe SHA: `20f922502159dd8373af16628a416e4468b43d83`.
[Real Windows run](https://github.com/suifracti/clash-speedtest/actions/runs/38125099083): collection completed successfully. This means observations were collected, not safety/durability GO. Workflow is added ONLY on dedicated probe branch; PR50/PR48/main implementation and existing workflows are unchanged. Standalone test imports no SQLite or CST code, mutates only t.TempDir synthetic data and does not perform volume flush or physical power loss.

## Platform and privilege: exploratory only

GitHub hosted `windows-2025`, Windows 10.0 build26100, amd64, Go1.25.14, x/sys v0.47.0 (module pin), local temporary C: subtree on NTFS with persistent ACL support. TokenElevation=1, effective Administrators membership=true, ordinary_token=false. This is an elevated/admin runner and **cannot replace non-admin local NTFS acceptance**. No token privileges were enabled, user accounts created or credentials collected. The probe did not run as a restricted-token or ordinary-user substitute.

`observations.jsonl` contains normalized parsed native API results (runner SID and synthetic TempDir prefix redacted). Every API failure retains its Win32 error. Original JSON test artifact is retained in the linked run for14 days. Its SHA256: `9726a4024e300a0e40c3ae28da4a32885d4c3e235c1f3c132a4550a18e172dbc`. This commit adds evidence only; native test source/workflow match the executed SHA. Local Windows cross-compilation was preparation only, not runtime evidence.

## Actual observations

| Operation | Native result on THIS elevated runner | Limit |
|---|---|---|
| Go permission reporting | Directory0777, file0666 | Confirms exact0700/0600 checks cannot prove Windows ACL privacy. |
| File open/flush | GENERIC_READ open succeeds, FlushFileBuffers fails5; WRITE and READ|WRITE opens/flushes succeed | Native file-flush API completion, not measured power-loss recovery. |
| WRITE_THROUGH file | CREATE_NEW/write/flush/close all succeed | Does not establish newly created ancestor directory naming persistence. |
| Fresh child+parent dirs | All48 opens succeed. Access0/READ flushes fail5 (24); WRITE/READ|WRITE flushes succeed (24) | No generic claim that Windows directory flush always fails. This runner accepts writable directory flush; non-admin behavior and durable directory-entry guarantee remain unproved. |
| Private ACL creation | protected root owner=current token user, DACL `(A;OICI;FA;;;LA)`; inherited file DACL `(A;ID;FA;;;LA)` | Root creation can carry owner-only private ACL. **Inherited file owner=BA, not current user** on this elevated runner; owner assumption requires explicit investigation. No second-user access test or complete ACL security audit. |
| hardlink/no-replace | First link succeeds, collision183, linked data equal, source-name removal succeeds | API visibility/non-replacement observed, not durable publication proof. |
| no DELETE share | unlink fails32 with held READ handle/share3; after close unlink succeeds | Close order matters independently of ACL; cleanup failure is observable. |
| nonempty dir | remove fails145 | Expected actionable failure; no production error suppression. |
| same-volume rename | MOVEFILE_WRITE_THROUGH succeeds; existing destination fails183 without replace | API success does not substitute for parent/new-entry durability proof. |
| junction | Create, no-follow handle open and reparse-attribute readback succeed; remove link succeeds | Capability probe only; not adversarial ancestor-swap or source-path security acceptance. |

Directory matrix: child and parent x access0/0x80000000/0x40000000/0xc0000000 x share0/3/7 x flags0x02200000 (BACKUP_SEMANTICS|OPEN_REPARSE_POINT) or0x82200000 (plus WRITE_THROUGH). Every open/flush/close is recorded individually. File access sharesREAD|WRITE|DELETE (7), flagsNORMAL (0x80); explicit write-through creation adds0x80000000.

## Evidence and contract boundaries

1. **API success: observed.** The supported writable handles returned success. Native failure codes5/32/145/183 are actual results, not injected failures. All this is scoped to the elevated hosted environment.
2. **File flush: documented API semantics + observed call completion.** [FlushFileBuffers](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers) requires GENERIC_WRITE and describes flushing buffered file data; volume flushing requires administrative rights. No volume flushing performed. Storage hardware/host caching and crash/power-loss behavior were not measured.
3. **New-directory-entry persistence: not established.** [CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew) documents directory handles and write-through flags. Our directory flush returns success with write access, which warrants follow-up; neither the reviewed documentation nor this observation establishes the specific parent/new-child/hard-link namespace durability contract under ordinary-user permissions. [MoveFileExW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw) describes write-through moves including copy/delete flushing; it is not evidence that the existing hard-link publication plus all parent entries survives sudden power loss. [CreateHardLinkW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createhardlinkw) documents same-volume file links, not a directory durability barrier.
4. **Power-loss recovery: NOT tested.** No physical power cut, VM crash, process-crash/restart or SQLite/WAL recovery exercise occurred. A readable linked file and test PASS cannot claim those guarantees.

## Decision

**BLOCKED for entering Windows successful-upgrade implementation.** Current evidence does not answer “ordinary-user reliable realization of the existing SQLite/WAL pre-upgrade backup contract” affirmatively. It also does not prove that Windows cannot implement that contract: writable directory flush API success is useful exploratory evidence, and should not be concealed or labeled unsupported solely because of the prior static concerns.

Missing gates: real standard-user NTFS observations; a defensible documented/qualified namespace durability barrier for newly-created attempts/root/manifest links; explicit privilege/owner/ACL/security boundary review. Power-loss claims would need separately authorized recovery evidence and storage assumptions. Neither elevated API success nor bypassing0700/0600 grants GO. Preserve existing fail-closed production gate.

Stop here: no compatibility layer, schema/startup change, new production flags, gate weakening, real database/SQLite/WAL access, subscription/probe/VPN/TUN, merge or release. Further execution requires the next user decision; this probe does not auto-start implementation.
