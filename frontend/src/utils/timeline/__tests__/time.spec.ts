import { describe, expect, it } from 'vitest'
import {
  buildTimeTicks,
  chooseTickStep,
  firstTickAtOrAfter,
  formatClock,
  formatTickLabel,
  formatUtc,
  formatWallClock,
  localMidnightMs,
  MS_DAY,
  MS_HOUR,
} from '../time'
import { viewportForRange } from '../viewport'

const UTC_PLUS_8 = 480
const UTC_PLUS_0 = 0
const UTC_MINUS_5 = -300

// 2026-09-17T10:00:00Z → 18:00 in UTC+8, 10:00 in UTC, 05:00 in UTC-5.
const T = Date.parse('2026-09-17T10:00:00.000Z')

describe('timezone-aware formatting', () => {
  it('renders the same instant differently per UTC offset', () => {
    expect(formatClock(T, UTC_PLUS_8)).toBe('18:00')
    expect(formatClock(T, UTC_PLUS_0)).toBe('10:00')
    expect(formatClock(T, UTC_MINUS_5)).toBe('05:00')
  })

  it('formats a full local wall-clock timestamp', () => {
    expect(formatWallClock(T, UTC_PLUS_8)).toBe('2026-09-17 18:00')
    expect(formatWallClock(T, UTC_PLUS_8, true)).toBe('2026-09-17 18:00:00')
  })

  it('formats UTC independently of the local offset', () => {
    expect(formatUtc(T)).toBe('2026-09-17 10:00')
    expect(formatUtc(T, true)).toBe('2026-09-17 10:00:00')
  })

  it('rolls the date over when the offset crosses midnight', () => {
    const late = Date.parse('2026-09-17T20:30:00.000Z')
    expect(formatWallClock(late, UTC_PLUS_8)).toBe('2026-09-18 04:30')
  })

  it('zero-pads every component', () => {
    const early = Date.parse('2026-01-02T03:04:05.000Z')
    expect(formatWallClock(early, UTC_PLUS_0, true)).toBe('2026-01-02 03:04:05')
  })
})

describe('chooseTickStep', () => {
  it('picks a step that keeps labels apart', () => {
    // 24h across 1200px → 14 label slots → 2h is the first step wide enough.
    expect(chooseTickStep(24 * MS_HOUR, 1200, 84)).toBe(2 * MS_HOUR)
  })

  it('picks finer steps when zoomed in', () => {
    expect(chooseTickStep(10 * 60_000, 1200, 84)).toBe(60_000)
    expect(chooseTickStep(60_000, 1200, 84)).toBe(5_000)
  })

  it('picks coarser steps when zoomed out', () => {
    expect(chooseTickStep(30 * MS_DAY, 1200, 84)).toBe(7 * MS_DAY)
  })

  it('falls back to the coarsest step beyond the table', () => {
    expect(chooseTickStep(3650 * MS_DAY, 1200, 84)).toBe(7 * MS_DAY)
  })

  it('never returns a non-positive step for degenerate input', () => {
    expect(chooseTickStep(0, 1200)).toBeGreaterThan(0)
    expect(chooseTickStep(MS_HOUR, 0)).toBeGreaterThan(0)
  })
})

describe('firstTickAtOrAfter', () => {
  it('aligns sub-day ticks to local wall-clock multiples', () => {
    const from = Date.parse('2026-09-17T10:07:00.000Z')
    // 18:07 local in UTC+8 → next whole hour is 19:00 local → 11:00Z.
    expect(firstTickAtOrAfter(from, MS_HOUR, UTC_PLUS_8)).toBe(
      Date.parse('2026-09-17T11:00:00.000Z')
    )
  })

  it('returns the boundary itself when it lands exactly on a step', () => {
    const from = Date.parse('2026-09-17T10:00:00.000Z')
    expect(firstTickAtOrAfter(from, MS_HOUR, UTC_PLUS_0)).toBe(from)
  })

  it('aligns day-scale ticks to local midnight', () => {
    // 18:00 local on 09-17 → next local midnight is 09-18 00:00 local = 09-17 16:00Z.
    expect(firstTickAtOrAfter(T, MS_DAY, UTC_PLUS_8)).toBe(
      Date.parse('2026-09-17T16:00:00.000Z')
    )
  })

  it('aligns day-scale ticks to local midnight and advances by whole days', () => {
    const first = firstTickAtOrAfter(T, 7 * MS_DAY, UTC_PLUS_8)
    // A day-scale tick is exactly a local midnight, at or after the query instant.
    expect(formatClock(first, UTC_PLUS_8)).toBe('00:00')
    expect(first).toBeGreaterThanOrEqual(T)
    // And it is the earliest such grid point.
    expect(first - 7 * MS_DAY).toBeLessThan(T)
    // Advancing one millisecond past it yields exactly the next grid point.
    expect(firstTickAtOrAfter(first + 1, 7 * MS_DAY, UTC_PLUS_8)).toBe(first + 7 * MS_DAY)
  })

  it('keeps a stable grid phase as the viewport pans', () => {
    const offsetsInDays = [-400, -40, -1, 0, 1, 40, 400]
    const ticks = offsetsInDays.map((d) =>
      firstTickAtOrAfter(T + d * MS_DAY, 7 * MS_DAY, UTC_PLUS_8)
    )
    // Every resolved tick must sit on the same 7-day lattice.
    for (const tick of ticks) {
      expect((tick - ticks[0]) % (7 * MS_DAY)).toBe(0)
    }
  })

  it('resolves every day-scale tick to local midnight across timezones', () => {
    for (const tz of [UTC_PLUS_8, UTC_PLUS_0, UTC_MINUS_5]) {
      const tick = firstTickAtOrAfter(T, MS_DAY, tz)
      expect(formatClock(tick, tz)).toBe('00:00')
    }
  })
})

describe('localMidnightMs', () => {
  it('returns local midnight expressed in epoch milliseconds', () => {
    expect(localMidnightMs(T, UTC_PLUS_8)).toBe(Date.parse('2026-09-16T16:00:00.000Z'))
    expect(localMidnightMs(T, UTC_PLUS_0)).toBe(Date.parse('2026-09-17T00:00:00.000Z'))
  })
})

describe('formatTickLabel', () => {
  it('uses clock labels for sub-day steps', () => {
    expect(formatTickLabel(T, MS_HOUR, UTC_PLUS_8)).toBe('18:00')
  })

  it('uses month-day labels for day-scale steps', () => {
    expect(formatTickLabel(T, MS_DAY, UTC_PLUS_8)).toBe('09-17')
    expect(formatTickLabel(T, 7 * MS_DAY, UTC_PLUS_8)).toBe('09-17')
  })
})

describe('buildTimeTicks', () => {
  it('covers the viewport with in-range, labelled ticks', () => {
    const start = Date.parse('2026-09-17T00:00:00.000Z')
    const vp = viewportForRange(start, start + 24 * MS_HOUR, 1200)
    const ticks = buildTimeTicks(vp, { tzOffsetMinutes: UTC_PLUS_8 })

    expect(ticks.length).toBeGreaterThan(5)
    for (const tick of ticks) {
      expect(tick.tMs).toBeGreaterThanOrEqual(vp.startMs)
      expect(tick.tMs).toBeLessThanOrEqual(vp.endMs)
      expect(tick.x).toBeGreaterThanOrEqual(0)
      expect(tick.x).toBeLessThanOrEqual(vp.widthPx)
      expect(tick.label.length).toBeGreaterThan(0)
    }
  })

  it('produces strictly increasing tick times', () => {
    const start = Date.parse('2026-09-17T00:00:00.000Z')
    const vp = viewportForRange(start, start + 7 * MS_DAY, 900)
    const ticks = buildTimeTicks(vp, { tzOffsetMinutes: UTC_PLUS_8 })
    for (let i = 1; i < ticks.length; i += 1) {
      expect(ticks[i].tMs).toBeGreaterThan(ticks[i - 1].tMs)
    }
  })

  it('marks day-scale ticks as major', () => {
    const start = Date.parse('2026-09-17T00:00:00.000Z')
    const vp = viewportForRange(start, start + 7 * MS_DAY, 900)
    const ticks = buildTimeTicks(vp, { tzOffsetMinutes: UTC_PLUS_8 })
    expect(ticks.some((t) => t.major)).toBe(true)
  })

  it('returns nothing for a degenerate viewport', () => {
    // viewportForRange normalises the span, so build the degenerate case explicitly.
    expect(buildTimeTicks({ startMs: T, endMs: T, widthPx: 800 }, { tzOffsetMinutes: UTC_PLUS_8 })).toEqual([])
    expect(buildTimeTicks({ startMs: T, endMs: T + 1000, widthPx: 0 }, { tzOffsetMinutes: UTC_PLUS_8 })).toEqual([])
  })
})
