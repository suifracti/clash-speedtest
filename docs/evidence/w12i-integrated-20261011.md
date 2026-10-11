# CST W12i temporary Git-object integration manifest (2026-10-11)

**Draft integration candidate only. Not main, a release, or an installed desktop application.**

## Exact source revisions (8 original Draft PRs)

- B1 W01 #29: `d65281ab4192f0f2772e5b295154f4ed5489b797`
- W02 #34: `0c871a0b1bd81cf684c6549ed3d5a7f5302cd422`
- W03 #36: `f12249c1bbef59e49285ffcbed4f1ac062c163c9`
- W04/05 #35: `24d6e66f1285f9e1aff782e8db5de5dfbfa2a35c`
- W06 #45: `f5d11a5483eaa207dcac819ca3e09ac5e340b565`
- W11a #42: `91afa7b38e980d874dbb3355ac091993e777e895`
- W09a #43: `9b248cf05b466019297672458889d9faafd94fa1`
- W08a #44: `45ccb409624b3c8c8d2088a0cfc1e3a8d313be53`

## Assembly and compatibility

The CI candidate uses B1 as its file tree base, then overlays the _relative-to-base_ source-PR file blobs: W02 Web safety/CI, W03 subscriptions, W04/W05 B3, W06 B4 (descends from W04/W05), W11a backup, W09a history and W08a Settings/Data UI.

Due to the current assistant container's lack of GitHub DNS connectivity, the initial integration tree is assembled through the GitHub Git objects API on a **fresh independent remote Git branch**, not by claiming access to the user's Mac or a local source checkout. The tree itself is verified and tested by the integration Draft PR's GitHub Actions; a conventional independent local worktree must be checked out by an execution agent if a local build is required. The eight original PR branches are untouched.

**Exactly two shared paths:**
1. `adapter/web/server.go` uses W02's security middleware/default localhost/password implementation and adds exactly W03's four subscription-refresh REST route registrations. The synthetic regression `adapter/web/integration_auth_refresh_test.go` verifies authenticated remote access, unauthorized/invalid-host/origin rejection, and the passwordless local route.
2. `core/speedtester/download_egress_darwin.go` uses W06's final blob, which includes the W04→W05 B3 parent changes. It is not overwritten by the older B3 blob.

This integration does not add a new SQLite schema, exercise production data, update VPN/TUN settings, or send external DNS, node, subscription, IPOK or paid-service requests.

## Required evidence before any broader acceptance

- CI must attach to **this exact integration commit** and run Go race, Vitest/build, Windows build and a genuine macOS app bundle assertion.
- The W11a pre-upgrade backup primitive is NOT yet automatically invoked before a real user's v11→v12 OpenDB. **Never open or upgrade a production v11 SQLite+WAL database using this candidate until a separate deployment-safe backup/rollback gate is approved and proven.**
- W06's local DNS RCODE/AAAA tests are not real IPv6 physical-interface or TUN routing verification. Real OS file dialogs and actual 1440×900/800px Wails windows are still unverified.
- All original 8 PRs and `main` must remain unchanged and unmerged. This branch exists solely for CI and review, under W12i #46 user authorization.
