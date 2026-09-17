/**
 * Projection of raw samples into drawable marks, plus hit testing.
 *
 * Hard invariant: **every loaded sample produces its own mark.** Overlapping marks may collide
 * in pixel space when zoomed out, but they are never merged, dropped, or replaced by a
 * synthesized "interval" mark. The column index below keeps every colliding sample reachable
 * through hover, nearest-hit, click and keyboard stepping.
 *
 * Culling only decides what gets *painted*; it never removes a sample from the projection's
 * addressable set for the current viewport.
 */

import type { MonitorSample } from '../../types'
import {
  FAILURE_MARK_HEIGHT_PX,
  latencyToStemHeight,
  sampleSemantic,
  type SampleSemantic,
} from './encoding'
import { compareSamplesAsc, type TimelineLane } from './lanes'
import { timeToX, type TimelineViewport } from './viewport'

export interface TimelineLayout {
  /** Y of the first lane band's top edge. */
  plotTopPx: number
  laneHeightPx: number
  laneGapPx: number
}

export interface SampleMark {
  sample: MonitorSample
  laneKey: string
  laneIndex: number
  x: number
  /** Integer pixel column used for collision grouping. */
  column: number
  /** Position within this lane's colliding group at this column (0-based). */
  coincidentIndex: number
  /** Size of the colliding group, including this mark. */
  coincidentCount: number
  /** Stem height in pixels (latency for successes, fixed for failures). */
  stemHeight: number
  semantic: SampleSemantic
}

export interface TimelineProjection {
  marks: SampleMark[]
  lanes: TimelineLane[]
  layout: TimelineLayout
  /** `${laneIndex}|${column}` → indices into `marks`. */
  columnIndex: Map<string, number[]>
  /** Marks whose x falls inside the plot area (what actually gets painted). */
  visibleCount: number
  /** Total marks produced from all loaded samples. */
  totalCount: number
}

export function laneBand(layout: TimelineLayout, laneIndex: number): { top: number; bottom: number; baseline: number } {
  const top = layout.plotTopPx + laneIndex * (layout.laneHeightPx + layout.laneGapPx)
  const bottom = top + layout.laneHeightPx
  return { top, bottom, baseline: bottom }
}

export function laneIndexAtY(layout: TimelineLayout, y: number, laneCount: number): number | null {
  const stride = layout.laneHeightPx + layout.laneGapPx
  if (stride <= 0) return null
  const idx = Math.floor((y - layout.plotTopPx) / stride)
  if (idx < 0 || idx >= laneCount) return null
  return idx
}

function columnKey(laneIndex: number, column: number): string {
  return `${laneIndex}|${column}`
}

export interface ProjectSamplesInput {
  lanes: TimelineLane[]
  viewport: TimelineViewport
  layout: TimelineLayout
  latencyScaleMs: number
  /** Maximum stem height; derived from the lane band height by the caller. */
  maxStemHeightPx: number
  /** Extra pixels beyond the plot edges that still produce marks (for smooth panning). */
  overscanPx?: number
}

export function projectSamples(input: ProjectSamplesInput): TimelineProjection {
  const { lanes, viewport, layout, latencyScaleMs, maxStemHeightPx } = input
  const overscan = input.overscanPx ?? 2

  const marks: SampleMark[] = []
  let visibleCount = 0

  lanes.forEach((lane, laneIndex) => {
    for (const sample of lane.samples) {
      const x = timeToX(sample.timestampMs, viewport)
      if (!Number.isFinite(x)) continue
      if (x < -overscan || x > viewport.widthPx + overscan) continue

      const semantic = sampleSemantic(sample)
      const stemHeight =
        semantic.outcome === 'success'
          ? latencyToStemHeight(sample.latencyMs, latencyScaleMs, maxStemHeightPx)
          : FAILURE_MARK_HEIGHT_PX

      marks.push({
        sample,
        laneKey: lane.key,
        laneIndex,
        x,
        column: Math.round(x),
        coincidentIndex: 0,
        coincidentCount: 1,
        stemHeight,
        semantic,
      })
      visibleCount += 1
    }
  })

  // Group colliding marks per (lane, column) so each one stays individually addressable.
  const groups = new Map<string, SampleMark[]>()
  for (const mark of marks) {
    const key = columnKey(mark.laneIndex, mark.column)
    const list = groups.get(key)
    if (list) list.push(mark)
    else groups.set(key, [mark])
  }

  const columnIndex = new Map<string, number[]>()
  const markIndexById = new Map<SampleMark, number>()
  marks.forEach((mark, index) => markIndexById.set(mark, index))

  for (const [key, group] of groups) {
    group.sort((a, b) => compareSamplesAsc(a.sample, b.sample))
    const indices: number[] = []
    group.forEach((mark, i) => {
      mark.coincidentIndex = i
      mark.coincidentCount = group.length
      indices.push(markIndexById.get(mark)!)
    })
    columnIndex.set(key, indices)
  }

  return {
    marks,
    lanes,
    layout,
    columnIndex,
    visibleCount,
    totalCount: lanes.reduce((acc, lane) => acc + lane.samples.length, 0),
  }
}

/**
 * Returns every mark addressable near a pointer position, nearest first.
 *
 * A small column window is searched so a mark stays reachable even when the pointer lands a
 * pixel or two away — this is the "pointer nearest-hit" affordance, not a snap-to-nearest-mark
 * that would hide neighbouring samples.
 */
export function hitTestMarks(
  projection: TimelineProjection,
  x: number,
  y: number,
  columnTolerance = 2
): number[] {
  const laneIndex = laneIndexAtY(projection.layout, y, projection.lanes.length)
  if (laneIndex === null) return []

  const centre = Math.round(x)
  const candidates: { index: number; distance: number }[] = []

  for (let col = centre - columnTolerance; col <= centre + columnTolerance; col += 1) {
    const indices = projection.columnIndex.get(columnKey(laneIndex, col))
    if (!indices) continue
    for (const index of indices) {
      const mark = projection.marks[index]
      candidates.push({ index, distance: Math.abs(mark.x - x) })
    }
  }

  candidates.sort((a, b) => {
    if (a.distance !== b.distance) return a.distance - b.distance
    const ma = projection.marks[a.index]
    const mb = projection.marks[b.index]
    if (ma.sample.timestampMs !== mb.sample.timestampMs) {
      return ma.sample.timestampMs - mb.sample.timestampMs
    }
    return ma.sample.sampleId < mb.sample.sampleId ? -1 : 1
  })

  // Deduplicate while preserving order (a mark can only be in one column bucket).
  const seen = new Set<number>()
  const ordered: number[] = []
  for (const c of candidates) {
    if (seen.has(c.index)) continue
    seen.add(c.index)
    ordered.push(c.index)
  }
  return ordered
}

/** Nearest mark in a specific lane, or null. */
export function pickNearestMark(
  projection: TimelineProjection,
  x: number,
  y: number,
  columnTolerance = 2
): number | null {
  const hits = hitTestMarks(projection, x, y, columnTolerance)
  return hits.length > 0 ? hits[0] : null
}

/**
 * Steps through marks in a lane in time order. Returns null at the boundaries so the caller can
 * decide whether to move to an adjacent lane.
 */
export function stepMarkInLane(
  projection: TimelineProjection,
  currentIndex: number,
  direction: 1 | -1
): number | null {
  const current = projection.marks[currentIndex]
  if (!current) return null

  const laneMarks = projection.marks
    .map((mark, index) => ({ mark, index }))
    .filter((entry) => entry.mark.laneKey === current.laneKey)
    .sort((a, b) => compareSamplesAsc(a.mark.sample, b.mark.sample))

  const position = laneMarks.findIndex((entry) => entry.index === currentIndex)
  if (position < 0) return null

  const next = position + direction
  if (next < 0 || next >= laneMarks.length) return null
  return laneMarks[next].index
}

/** Index of the mark in a lane closest to an instant, used by revision-boundary selection. */
export function markIndexAtOrAfter(
  projection: TimelineProjection,
  laneKey: string,
  atMs: number
): number | null {
  let best: number | null = null
  let bestDelta = Number.POSITIVE_INFINITY

  projection.marks.forEach((mark, index) => {
    if (mark.laneKey !== laneKey) return
    const delta = mark.sample.timestampMs - atMs
    if (delta < 0) return
    if (delta < bestDelta) {
      bestDelta = delta
      best = index
    }
  })

  return best
}
