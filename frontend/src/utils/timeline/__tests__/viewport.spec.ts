import { describe, expect, it } from 'vitest'
import {
  MAX_VIEWPORT_SPAN_MS,
  MIN_VIEWPORT_SPAN_MS,
  clampViewportToDomain,
  isAtLiveEdge,
  msPerPixel,
  needsOlderData,
  panViewport,
  panViewportByMs,
  timeToX,
  viewportForRange,
  viewportSpanMs,
  xToTime,
  zoomViewport,
  type TimelineViewport,
} from '../viewport'

const T0 = Date.parse('2026-09-17T00:00:00.000Z')
const HOUR = 3_600_000

function vp(startMs: number, endMs: number, widthPx: number): TimelineViewport {
  return { startMs, endMs, widthPx }
}

describe('viewport projection', () => {
  it('maps the window edges onto the plot edges', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    expect(timeToX(T0, v)).toBe(0)
    expect(timeToX(T0 + HOUR, v)).toBe(1000)
  })

  it('maps a timestamp to a proportional x coordinate', () => {
    const v = vp(T0, T0 + HOUR, 600)
    expect(timeToX(T0 + HOUR / 4, v)).toBeCloseTo(150, 6)
    expect(timeToX(T0 + HOUR / 2, v)).toBeCloseTo(300, 6)
    expect(timeToX(T0 + (3 * HOUR) / 4, v)).toBeCloseTo(450, 6)
  })

  it('returns out-of-range x for samples outside the window (used for culling)', () => {
    const v = vp(T0, T0 + HOUR, 600)
    expect(timeToX(T0 - HOUR, v)).toBeLessThan(0)
    expect(timeToX(T0 + 2 * HOUR, v)).toBeGreaterThan(600)
  })

  it('xToTime is the inverse of timeToX', () => {
    const v = vp(T0, T0 + 6 * HOUR, 800)
    for (const t of [T0, T0 + 1, T0 + HOUR * 2.5, T0 + 6 * HOUR]) {
      expect(xToTime(timeToX(t, v), v)).toBeCloseTo(t, 6)
    }
  })

  it('exposes milliseconds per pixel', () => {
    const v = vp(T0, T0 + HOUR, 3600)
    expect(msPerPixel(v)).toBeCloseTo(1000, 6)
  })

  it('survives a degenerate zero-span viewport without producing NaN', () => {
    const v = vp(T0, T0, 500)
    expect(Number.isFinite(timeToX(T0, v))).toBe(true)
    expect(timeToX(T0, v)).toBe(0)
  })
})

describe('viewportForRange', () => {
  it('normalises the width to at least one pixel', () => {
    const v = viewportForRange(T0, T0 + HOUR, 0)
    expect(v.widthPx).toBe(1)
    expect(viewportSpanMs(v)).toBe(HOUR)
  })

  it('clamps a too-small span up to the minimum', () => {
    const v = viewportForRange(T0, T0 + 1, 100)
    expect(viewportSpanMs(v)).toBe(MIN_VIEWPORT_SPAN_MS)
  })
})

describe('zoomViewport', () => {
  it('keeps the instant under the anchor pixel fixed', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    const anchor = 250
    const anchoredTime = xToTime(anchor, v)

    const zoomed = zoomViewport(v, 0.5, anchor)

    expect(viewportSpanMs(zoomed)).toBeCloseTo(HOUR * 0.5, 6)
    expect(timeToX(anchoredTime, zoomed)).toBeCloseTo(anchor, 6)
  })

  it('zooms out symmetrically around the anchor', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    const zoomed = zoomViewport(v, 2, 500)
    expect(viewportSpanMs(zoomed)).toBeCloseTo(HOUR * 2, 6)
    expect(timeToX(T0 + HOUR / 2, zoomed)).toBeCloseTo(500, 6)
  })

  it('clamps zoom-in to the minimum span', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    const zoomed = zoomViewport(v, 1e-9, 500)
    expect(viewportSpanMs(zoomed)).toBe(MIN_VIEWPORT_SPAN_MS)
  })

  it('clamps zoom-out to the maximum span', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    const zoomed = zoomViewport(v, 1e9, 500)
    expect(viewportSpanMs(zoomed)).toBe(MAX_VIEWPORT_SPAN_MS)
  })

  it('is a no-op for a non-positive or non-finite factor', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    expect(zoomViewport(v, 0, 100)).toBe(v)
    expect(zoomViewport(v, Number.NaN, 100)).toBe(v)
  })
})

describe('panViewport', () => {
  it('shifts the window forward in time for a positive pixel delta', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    const panned = panViewport(v, 100)
    expect(panned.startMs).toBeCloseTo(T0 + HOUR * 0.1, 6)
    expect(viewportSpanMs(panned)).toBe(HOUR)
  })

  it('shifts backward for a negative pixel delta', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    const panned = panViewport(v, -100)
    expect(panned.startMs).toBeCloseTo(T0 - HOUR * 0.1, 6)
  })

  it('panViewportByMs shifts by an absolute duration', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    expect(panViewportByMs(v, 2 * HOUR).startMs).toBe(T0 + 2 * HOUR)
  })

  it('is a no-op for a zero delta', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    expect(panViewport(v, 0)).toBe(v)
  })
})

describe('clampViewportToDomain', () => {
  const domainStart = T0
  const domainEnd = T0 + 24 * HOUR

  it('clamps the start edge to the domain start', () => {
    const v = vp(T0 - 10 * HOUR, T0 - 9 * HOUR, 1000)
    const clamped = clampViewportToDomain(v, domainStart, domainEnd)
    expect(clamped.startMs).toBe(domainStart)
  })

  it('clamps the end edge to the domain end', () => {
    const v = vp(T0 + 23 * HOUR, T0 + 24 * HOUR, 1000)
    const clamped = clampViewportToDomain(v, domainStart, domainEnd)
    expect(clamped.endMs).toBe(domainEnd)
  })

  it('leaves an in-domain viewport untouched', () => {
    const v = vp(T0 + 4 * HOUR, T0 + 5 * HOUR, 1000)
    const clamped = clampViewportToDomain(v, domainStart, domainEnd)
    expect(clamped.startMs).toBe(v.startMs)
    expect(clamped.endMs).toBe(v.endMs)
  })

  it('anchors a viewport wider than the domain to the domain newest edge', () => {
    const v = vp(T0, T0 + 72 * HOUR, 1000)
    const clamped = clampViewportToDomain(v, domainStart, domainEnd)
    expect(clamped.endMs).toBe(domainEnd)
    expect(viewportSpanMs(clamped)).toBe(72 * HOUR)
  })

  it('is a no-op when the domain is degenerate', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    expect(clampViewportToDomain(v, domainStart, domainStart)).toBe(v)
  })
})

describe('live edge and older-data detection', () => {
  it('detects being at the live edge within tolerance', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    expect(isAtLiveEdge(v, T0 + HOUR, 1000)).toBe(true)
    expect(isAtLiveEdge(v, T0 + HOUR + 5000, 1000)).toBe(false)
  })

  it('requests older data once the oldest loaded sample nears the left edge', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    // Oldest loaded sample sits at the very left edge → the loaded window ends here.
    expect(needsOlderData(v, T0, 120)).toBe(true)
    // Oldest loaded sample is at the right edge: everything left of it is still unloaded.
    expect(needsOlderData(v, T0 + HOUR, 120)).toBe(true)
    // Oldest loaded sample is far off to the left → plenty of loaded history still visible.
    expect(needsOlderData(v, T0 - 10 * HOUR, 120)).toBe(false)
  })

  it('never asks for older data when nothing is loaded', () => {
    const v = vp(T0, T0 + HOUR, 1000)
    expect(needsOlderData(v, null, 120)).toBe(false)
  })
})
