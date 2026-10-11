# CST W12i temporary Git-object integration manifest (2026-10-11)

**Draft integration candidate only. Not main, a release, or an installed desktop application.**

## Authorized four-PR increment (2026-10-11)

Baseline: `5094e54b045a359a83447d08a23dcb6d24be9b7d` in the existing isolated Mac worktree. The original eight packages below remain ancestors and are not reapplied.

| PR | Locked source head | Integrated commits |
|---|---|---|
| #53 GUI isolation (first) | `2d9ded0035f4d8d217b94a31d8a4d1b56e5b328d` | `69d0fdc`, `7b71850` |
| #49 Monitor recovery fixture | `63b1b8b37971bdf8d2603da7e535baca799e8e8b` | `f75fdcf` |
| #50 v11 verified backup gate | `2320e7cdf9f8c038a981b6cde2fe34c2392409f5` | `e759e94`, `e820e2b`, `b817c3e` |
| #54 Monitor UI | `acde520356d662eb86262d3c77903b772d153bdc` | `05bcb73`, `fd44fac`, `bcc7474` |

Each independent source commit was cherry-picked with provenance; every changed path matches the locked source HEAD blob exactly. No production conflict resolution was needed. Additional integration changes are this manifest and CI gates. #53 was applied before the first GUI lifecycle test, which passed using synthetic HOME/cwd/token/history only.

CI retains Linux full Go race/Vitest/build, Darwin strict DNS and native App build, and Windows exe build. It adds macOS synthetic history/application/appdata/GUI race and Windows runtime GUI lifecycle plus `TestV11UpgradeBackupWindowsUnsupportedPrivateRootBlocksDDL`. Windows rejection protects synthetic v11/WAL; **normal Windows v11-to-v12 upgrade remains unsupported**. No Windows successful-upgrade claim is made.

Fresh Mac validation: 216 Vitest tests, frontend build, targeted strict-DNS race, Go vet and build passed. Full synthetic package race results and exact final SHA/tree/remote CI are reported in Issue #46 after completion. An initial command used nonexistent `./appdata`; corrected path is `./core/appdata`, not a product defect. No real data or detection requests were used.

## Exact source revisions (8 original Draft PRs)

- B1 W01 #29: `d65281ab4192f0f2772e5b295154f4ed5489b797`
- W02 #34: `0c871a0b1bd81cf684c6549ed3d5a7f5302cd422`
- W03 #36: `f12249c1bbef59e49285ffcbed4f1ac062c163c9`
- W04/05 #35: `24d6e66f1285f9e1aff782e8db5de5dfbfa2a35c`
- W06 #45: `f5d11a5483eaa207dcac819ca3e09ac5e340b565`
- W11a #42: `91afa7b38e980d874dbb3355ac091993e777e895`
- W09a #43: `9b248cf05b466019297672458889d9faafd94fa1`
- W08a #44: `45ccb409624b3c8c8d2088a0cfc1e3a8d313be53`

## Original baseline assembly and compatibility (historical)

The CI candidate uses B1 as its file tree base, then overlays the _relative-to-base_ source-PR file blobs: W02 Web safety/CI, W03 subscriptions, W04/W05 B3, W06 B4 (descends from W04/W05), W11a backup, W09a history and W08a Settings/Data UI.

Due to the current assistant container's lack of GitHub DNS connectivity, the initial integration tree is assembled through the GitHub Git objects API on a **fresh independent remote Git branch**, not by claiming access to the user's Mac or a local source checkout. The tree itself is verified and tested by the integration Draft PR's GitHub Actions; a conventional independent local worktree must be checked out by an execution agent if a local build is required. The eight original PR branches are untouched.

**Exactly two shared paths:**
1. `adapter/web/server.go` uses W02's security middleware/default localhost/password implementation and adds exactly W03's four subscription-refresh REST route registrations. The synthetic regression `adapter/web/integration_auth_refresh_test.go` verifies authenticated remote access, unauthorized/invalid-host/origin rejection, and the passwordless local route.
2. `core/speedtester/download_egress_darwin.go` uses W06's final blob, which includes the W04→W05 B3 parent changes. It is not overwritten by the older B3 blob.

This integration does not add a new SQLite schema, exercise production data, update VPN/TUN settings, or send external DNS, node, subscription, IPOK or paid-service requests.

## Required evidence before any broader acceptance

- CI must attach to **this exact integration commit** and run Go race, Vitest/build, Windows build and a genuine macOS app bundle assertion.
- #50 now gates recognized v11 before v12 DDL with a verified WAL-aware backup. Synthetic Unix validation and Windows safe refusal do not authorize production-data startup, real upgrades, power-loss recovery or Windows successful upgrades.
- W06's local DNS RCODE/AAAA tests are not real IPv6 physical-interface or TUN routing verification. Real OS file dialogs and actual 1440×900/800px Wails windows are still unverified.
- All original 8 PRs and `main` must remain unchanged and unmerged. This branch exists solely for CI and review, under W12i #46 user authorization.
