# W10-1 Monitor recovery admission fixture evidence — 2026-10-11

Issue: [#47](https://github.com/suifracti/clash-speedtest/issues/47). Exact base: B1 / PR #29, `d65281ab4192f0f2772e5b295154f4ed5489b797`. Work branch: `codex/cst-w10-1-recovery`. Independent worktree: `clash-speedtest-w10-1-recovery`. Integration PR #48 was read only and was not modified.

## Actual CI evidence

[CI #123 attempt 1](https://github.com/suifracti/clash-speedtest/actions/runs/38068235315/attempts/1), head `0c871a0b1bd81cf684c6549ed3d5a7f5302cd422`, failed Linux `go test -v -race ./...` at `application/monitor_recovery_test.go:237`. The pause job was `running/restored`, while the fixture expected `restoring`. The test failed in 0.81s, before the fake occupier's five-second request timeout. The recovery test file at the CI head is identical to B1's file.

[Attempt 2](https://github.com/suifracti/clash-speedtest/actions/runs/38068235315/attempts/2) succeeded on the same head. This is an observed intermittent test result, not proof of an absence of production races. No CI rerun was dispatched by this task.

## Root cause and reliable counterexample

Recovery reads durable definitions in creation order, but `StartRecovered` launches an asynchronous round. Shared `AcquireRound` grants/staggers actual arrival, not definition order. `Runner.ExecuteRunWithAdmission` changes a job to `restored` after it acquires admission. The old fake transport blocked only the node labelled `occupier`; it did not block an earlier legitimately admitted task. Waiting until that named occupier entered the transport therefore did not prove that the four other tasks were still unadmitted.

The exact old target passed 20 local race repetitions. Changing only the fixture's definition creation order to `pause, stop, disable, delete, occupier`, retaining the original blocker and assertions, reproduced the CI symptom in **5/5 race repetitions**: pause was already `running/restored`. This is a reliable counterexample to the fixture's assumed first holder. It does not establish a production scheduler contract violation.

## Minimal repair and preserved contracts

Only `application/monitor_recovery_test.go` changes executable code; production recovery/jobs/scheduler/budget code and DTO/schema are unchanged.

- The fake transport selects and holds the actual first admitted node under a mutex. Its `started` channel is the synchronization boundary. Action roles are assigned after that signal, irrespective of definition/admission order.
- Before applying actions, the test proves the holder is restored, exactly one fake request has entered, the shared budget reports one active request, and each of four other restored jobs remains `restoring` with zero requests.
- Both occupier-first and occupier-last definition permutations run. All potential holders use the same 250ms synthetic interval, so later-cycle checks apply to whichever node actually holds the permit. This does not change the existing four-second event deadline or five-second fake request timeout.
- Existing Pause/Stop/Disable/Delete runtime and history assertions remain. Three persisted later occupier runs prove canceled recovery is not restarted on later ticks. Each canceled waiting job retains exactly its original saved run; durable paused/stopped/disabled intents and deleted definitions are explicitly checked.
- Added admitted-action cases prove: Pause saves paused intent while preserving the current admitted request; Stop drains that request and saves stopped intent; disabling future launch recovery preserves the running runtime/request and saves `resume_on_launch=false`; Delete drains/removes the definition while preserving history. Each case verifies its original saved run and durable intent after reopening the temporary store. These checks do not claim canceled in-flight sample/run finalization was successful or establish a full real-world lifecycle.

No test is skipped, removed or weakened to accept a restored waiting job. No added `Sleep`, increased timeout, retry-to-green loop, test-only production hook or production scheduling policy change.

## Bounded local verification

Environment: Go 1.25.14, Darwin/arm64. Every test command used `GOPROXY=off GOSUMDB=off`; all dependencies were already cached. All requests use injected fake transports; temporary profiles/history stores live under `t.TempDir()`.

| Stage | Command (following offline env prefix) | Actual result |
|---|---|---|
| Original unmodified B1 | `go test -race ./application -run '^TestMonitorRecoveryWaitCanBeCancelledByPauseStopDisableAndDelete$' -count=20 -timeout=90s` | PASS, 20 repetitions, 41.021s |
| Red reordered fixture, original blocker | `go test -race ./application -run '^TestMonitorRecoveryWaitCanBeCancelledByPauseStopDisableAndDelete/occupier_defined_last$' -count=5 -timeout=40s` | FAIL, 5/5 same pause running/restored assertion, 7.395s |
| Green corrected holder fixture | `go test -race ./application -run '^TestMonitorRecoveryWaitCanBeCancelledByPauseStopDisableAndDelete$' -count=10 -timeout=90s` | PASS, both permutations 10 times each (20 cases), 33.020s |
| Admitted action matrix | `go test -race ./application -run '^TestMonitorRecoveryAdmittedActionsPreserveDurableIntentAndHistory$' -count=3 -timeout=45s` | PASS, four actions 3 times each (12 cases), 5.552s |
| Recovery and persisted stop intent | `go test -race ./application -run '^TestMonitor(Recovery\|StopIntent)' -count=3 -timeout=90s` | PASS, 22.117s |
| Adjacent existing admission/scheduler tests | `go test -race ./core/monitor -run '^(TestBudgetAdmission\|TestMonitorBudgetIdentityAndCancelledWait\|TestScheduler)' -count=1 -timeout=45s` | PASS, 3.801s |
| Patch formatting | `gofmt` and `git diff --check` | PASS |

Independent read-only code review reported no actionable findings; the reviewer did not rerun tests or use production traffic. The commands' `-timeout` values bound whole test processes, not increased per-event/request timeout or a fix strategy. No whole-repository E2E run was requested or performed.

## Delivery boundary

Deliver as an independent B1-based stacked **Draft** PR and report the sole head and actual check state to Issue #47. Stacked checks that do not trigger are unverified, not passing CI. Local Darwin race tests are not a Linux runner reproduction or true long-monitor lifecycle acceptance. No real node/subscription/DNS/IPv6/TUN/IPOK/paid traffic, production SQLite/WAL, user setting writes, original Mac candidate changes, merge, rebase of main or release occurred.
