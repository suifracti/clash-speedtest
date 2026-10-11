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


## Partial W08 integration and six-site UI (2026-10-11)

Per rewritten Issue #32 and user authorization, development proceeds independently of PR #58. Baseline `3f3f8e19907046a471088ef2b1a749d099ac2091` retains twelve packages. Source #56 `043eb093ce6b82528390f83dfd38d193ed94dda2` maps to `e01edbe`, `06a4f5b`, `43f6cbe`; #57 `199032e86db092471b748a31bfae736cf5833085` maps to `dab52d8`. Source files match locked source blobs exactly. PR #58 is not included in this increment; its alert-summary revision requires author delivery and Luna B review.

Integration UI additions: HomeView wiring, SiteQuality.vue, siteQualityView.ts, three focused comparison/render tests and one mounted Home regression. Existing qualityData/window/identity/cache are reused. Expand per-node evidence to read saved records on demand; selecting sorting/filtering reads the fixed full scope. No measurement is dispatched. The entire filtered set must meet same target, URL/method/exit/family/window and metric gates before ranking; otherwise preserve original order and show non-comparability reasons. P50/rate require >=5 successful observations, P95 >=20. Old RTT remains diagnostic; no composite score. Original matrix/history hover/fix/download/service and source theme CSS remain.

Fresh local frontend: 35 test files, 254 tests PASS, vue-tsc/Vite build PASS. New tests had three expected RED failures before minimal implementation. First adjacent DOM regression identified eager detailed-view history reads; fixed to on-demand and original four selection DOM tests now PASS. First build found a local name collision with existing siteReason; renamed import, build now PASS. No skip or timeout changes. Initial harness missing modules and one wrong script cwd were corrected; they are not product failures.

Synthetic actual Vue Home/SiteQuality browser rendering at 1440x900 and 800x900, light/dark: no document overflow or page errors. Only isolated bridge saved-history reads; no refresh/start/monitor calls, no external request. Screenshots are synthetic browser evidence, not Wails or real-data acceptance. Final exact SHA/tree and full cross-platform CI are reported separately in Issue #46. Windows successful v11 upgrade remains blocked; real App, network, data and native acceptance remain outside authorization.
