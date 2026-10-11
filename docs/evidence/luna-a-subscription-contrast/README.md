# W08 subscription account contrast evidence

All screenshots are labeled `SYNTHETIC · OFFLINE`. The production frontend was served on localhost with a synthetic desktop bridge; browser routing blocked every non-local request. No backend, real subscription refresh, or monitor job was started.

## Visual captures

- `800-dark.png` and `800-light.png`: failed refresh state, usage, expiry, links, stages, retry control, and announcement count.
- `1440-dark.png` and `1440-light.png`: same account card at desktop width.
- `800-dark-running.png`: running refresh with disabled start control and cancel action.

## Computed colors

The reported ratios use browser-resolved CSS colors and the WCAG sRGB contrast formula. The previous dark card combined `--ink: #e4ebf5` with the undefined-token fallback background `#f5f7fa`, yielding **1.12:1**. The repaired account text uses `--ink` on `--surface-soft`: **12.66:1** in dark mode (`#1e2634`) and **12.49:1** in light mode (`#f9fafc`).

Refresh status text, stages, and error detail use `--ink` over the existing themed green, amber, or red status surfaces. Computed status-label ratios range from **10.08:1 to 11.21:1** in dark mode and **11.47:1 to 11.84:1** in light mode. Link colors, quota-track colors, and all resolved values are recorded in `computed-styles.json`.

## Interaction evidence

`behavior-checks.json` records synthetic failure → retry and running → cancel flows, including stage labels, disabled refresh opacity, hover color, and keyboard focus outlines. The existing Vitest integration test also covers account-scoped usage, expiry, refresh failure, links, and announcements.

This verifies the Vue frontend in a synthetic local browser. Wails native-window behavior and real backend data remain unverified.
