/**
 * Time axis ticks and timestamp formatting for the monitor timeline.
 *
 * Everything is derived from an explicit UTC offset in minutes rather than an IANA timezone
 * name, which keeps the behaviour deterministic and unit testable (and avoids depending on the
 * host's timezone database).
 */

import { timeToX, viewportSpanMs, type TimelineViewport } from './viewport'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/** Candidate tick steps, ascending. All sub-day steps divide a day evenly. */
const TICK_STEPS_MS: number[] = [
  1_000,
  5_000,
  15_000,
  30_000,
  MINUTE,
  2 * MINUTE,
  5 * MINUTE,
  10 * MINUTE,
  15 * MINUTE,
  30 * MINUTE,
  HOUR,
  2 * HOUR,
  3 * HOUR,
  6 * HOUR,
  12 * HOUR,
  DAY,
  2 * DAY,
  7 * DAY,
]

export interface TimeAxisOptions {
  /** Minutes east of UTC. e.g. UTC+8 → 480. */
  tzOffsetMinutes: number
}

export interface TimeTick {
  tMs: number
  x: number
  label: string
  /** Day boundaries are rendered as stronger gridlines. */
  major: boolean
}

/** Picks the smallest step that keeps labels at least `minLabelPx` apart. */
export function chooseTickStep(spanMs: number, widthPx: number, minLabelPx = 84): number {
  if (spanMs <= 0 || widthPx <= 0) return TICK_STEPS_MS[0]
  const maxTicks = Math.max(2, Math.floor(widthPx / Math.max(24, minLabelPx)))
  const target = spanMs / maxTicks
  for (const step of TICK_STEPS_MS) {
    if (step >= target) return step
  }
  return TICK_STEPS_MS[TICK_STEPS_MS.length - 1]
}

/** Shifts an instant so that reading UTC getters yields local wall-clock components. */
function toWallClock(tMs: number, tzOffsetMinutes: number): number {
  return tMs + tzOffsetMinutes * MINUTE
}

function pad2(n: number): string {
  return n < 10 ? `0${n}` : String(n)
}

/** Formats wall-clock components of an instant shifted by `tzOffsetMinutes`. */
export function formatWallClock(tMs: number, tzOffsetMinutes: number, withSeconds = false): string {
  const d = new Date(toWallClock(tMs, tzOffsetMinutes))
  const base = `${d.getUTCFullYear()}-${pad2(d.getUTCMonth() + 1)}-${pad2(d.getUTCDate())} ${pad2(
    d.getUTCHours()
  )}:${pad2(d.getUTCMinutes())}`
  return withSeconds ? `${base}:${pad2(d.getUTCSeconds())}` : base
}

/** UTC rendering of an instant, for evidence display alongside the local time. */
export function formatUtc(tMs: number, withSeconds = false): string {
  return formatWallClock(tMs, 0, withSeconds)
}

export function formatClock(tMs: number, tzOffsetMinutes: number): string {
  const d = new Date(toWallClock(tMs, tzOffsetMinutes))
  return `${pad2(d.getUTCHours())}:${pad2(d.getUTCMinutes())}`
}

function formatMonthDay(tMs: number, tzOffsetMinutes: number): string {
  const d = new Date(toWallClock(tMs, tzOffsetMinutes))
  return `${pad2(d.getUTCMonth() + 1)}-${pad2(d.getUTCDate())}`
}

/** Renders a tick label appropriate to the step size. */
export function formatTickLabel(tMs: number, stepMs: number, tzOffsetMinutes: number): string {
  if (stepMs >= DAY) return formatMonthDay(tMs, tzOffsetMinutes)
  return formatClock(tMs, tzOffsetMinutes)
}

/** Local midnight for the day containing `tMs`, expressed as epoch ms. */
export function localMidnightMs(tMs: number, tzOffsetMinutes: number): number {
  const wall = toWallClock(tMs, tzOffsetMinutes)
  const d = new Date(wall)
  d.setUTCHours(0, 0, 0, 0)
  return d.getTime() - tzOffsetMinutes * MINUTE
}

function addLocalDays(tMs: number, days: number, tzOffsetMinutes: number): number {
  const wall = toWallClock(tMs, tzOffsetMinutes)
  const d = new Date(wall)
  d.setUTCDate(d.getUTCDate() + days)
  d.setUTCHours(0, 0, 0, 0)
  return d.getTime() - tzOffsetMinutes * MINUTE
}

/**
 * Aligns the first tick at or after `fromMs`.
 *
 * Sub-day steps align to local wall-clock multiples of the step. Day-scale steps align to a
 * *stable* grid of local midnights anchored at the Unix epoch's local midnight and advanced by
 * whole calendar days, so the tick phase does not drift as the viewport pans.
 */
export function firstTickAtOrAfter(fromMs: number, stepMs: number, tzOffsetMinutes: number): number {
  if (stepMs >= DAY) {
    const days = Math.max(1, Math.round(stepMs / DAY))
    const origin = localMidnightMs(0, tzOffsetMinutes)

    // Index of the grid cell containing `fromMs`, then walk forward until the cell start
    // reaches `fromMs`. Calendar arithmetic keeps month/year and DST boundaries correct.
    const dayStart = localMidnightMs(fromMs, tzOffsetMinutes)
    let k = Math.floor(Math.round((dayStart - origin) / DAY) / days)
    if (k < 0) k = 0

    let candidate = addLocalDays(origin, k * days, tzOffsetMinutes)
    while (candidate < fromMs) {
      k += 1
      candidate = addLocalDays(origin, k * days, tzOffsetMinutes)
    }
    // Guard against rounding drift putting us one cell too far.
    while (k > 0) {
      const prev = addLocalDays(origin, (k - 1) * days, tzOffsetMinutes)
      if (prev < fromMs) break
      k -= 1
      candidate = prev
    }
    return candidate
  }

  const offsetMs = tzOffsetMinutes * MINUTE
  const wall = fromMs + offsetMs
  const alignedWall = Math.ceil(wall / stepMs) * stepMs
  return alignedWall - offsetMs
}

/** Builds the tick list covering the viewport, including pixel positions. */
export function buildTimeTicks(
  vp: TimelineViewport,
  opts: TimeAxisOptions,
  minLabelPx = 84
): TimeTick[] {
  const span = viewportSpanMs(vp)
  if (span <= 0 || vp.widthPx <= 0) return []

  const step = chooseTickStep(span, vp.widthPx, minLabelPx)
  const ticks: TimeTick[] = []

  let t = firstTickAtOrAfter(vp.startMs, step, opts.tzOffsetMinutes)
  // Guard against pathological loops if a step were ever 0 or negative.
  const maxTicks = Math.ceil(vp.widthPx / 8) + 8
  let guard = 0

  while (t <= vp.endMs && guard < maxTicks) {
    ticks.push({
      tMs: t,
      x: timeToX(t, vp),
      label: formatTickLabel(t, step, opts.tzOffsetMinutes),
      major: step >= DAY,
    })
    if (step >= DAY) {
      t = addLocalDays(t, Math.max(1, Math.round(step / DAY)), opts.tzOffsetMinutes)
    } else {
      t += step
    }
    guard += 1
  }

  return ticks
}

/** Host machine's UTC offset in minutes (east positive). */
export function hostTzOffsetMinutes(): number {
  return -new Date().getTimezoneOffset()
}

export { MINUTE as MS_MINUTE, HOUR as MS_HOUR, DAY as MS_DAY }
