/**
 * Canvas renderer for the raw-sample timeline.
 *
 * Hard invariant: the renderer draws exactly one glyph per projected mark. There is no bucketing,
 * no interval summarisation and no "representative" sampling. When marks collide they simply
 * overlap visually; each one remains individually addressable through the projection's column
 * index (see `marks.ts`).
 *
 * All geometry is in CSS pixels. The caller is responsible for applying the device-pixel-ratio
 * transform before invoking this function.
 */

import type { RevisionBoundary } from './lanes'
import { laneBand, type TimelineProjection } from './marks'
import type { TimeTick } from './time'
import { toneColor, type TimelineTheme } from './theme'
import type { TimelineViewport } from './viewport'

export interface RenderParams {
  ctx: CanvasRenderingContext2D
  widthPx: number
  heightPx: number
  axisHeightPx: number
  projection: TimelineProjection
  viewport: TimelineViewport
  ticks: TimeTick[]
  boundaries: RevisionBoundary[]
  selectedSampleId: string | null
  hoverSampleId: string | null
  theme: TimelineTheme
  /** Lane band the pointer is currently over, if any. */
  hoverLaneIndex: number | null
}

const AXIS_FONT = '10px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif'

export function renderTimeline(params: RenderParams): void {
  const { ctx, widthPx, heightPx, axisHeightPx, theme } = params

  ctx.clearRect(0, 0, widthPx, heightPx)
  ctx.fillStyle = theme.background
  ctx.fillRect(0, 0, widthPx, heightPx)

  drawLaneBands(params)
  drawTimeGrid(params)
  drawRevisionBoundaries(params)
  drawMarks(params)
  drawSelection(params)
  drawAxis(params)

  // Separate the plot from the axis strip.
  ctx.strokeStyle = theme.border
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.moveTo(0, heightPx - axisHeightPx + 0.5)
  ctx.lineTo(widthPx, heightPx - axisHeightPx + 0.5)
  ctx.stroke()
}

function drawLaneBands(params: RenderParams): void {
  const { ctx, widthPx, projection, theme, hoverLaneIndex } = params

  projection.lanes.forEach((_, index) => {
    const band = laneBand(projection.layout, index)
    ctx.fillStyle =
      hoverLaneIndex === index ? theme.laneBandHover : index % 2 === 0 ? theme.laneBand : theme.laneBandAlt
    ctx.fillRect(0, band.top, widthPx, band.bottom - band.top)

    // Baseline: the reference the latency stems grow from.
    ctx.strokeStyle = theme.baseline
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(0, band.baseline + 0.5)
    ctx.lineTo(widthPx, band.baseline + 0.5)
    ctx.stroke()
  })
}

function drawTimeGrid(params: RenderParams): void {
  const { ctx, projection, ticks, theme, heightPx, axisHeightPx } = params
  if (projection.lanes.length === 0) return

  const top = projection.layout.plotTopPx
  const bottom = heightPx - axisHeightPx

  for (const tick of ticks) {
    ctx.strokeStyle = tick.major ? theme.gridlineMajor : theme.gridline
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(Math.round(tick.x) + 0.5, top)
    ctx.lineTo(Math.round(tick.x) + 0.5, bottom)
    ctx.stroke()
  }
}

function drawRevisionBoundaries(params: RenderParams): void {
  const { ctx, projection, boundaries, theme, heightPx, axisHeightPx } = params
  if (boundaries.length === 0 || projection.lanes.length === 0) return

  const top = projection.layout.plotTopPx
  const bottom = heightPx - axisHeightPx

  ctx.save()
  ctx.setLineDash([3, 3])
  ctx.strokeStyle = theme.boundary
  ctx.lineWidth = 1

  for (const boundary of boundaries) {
    const laneIndex = projection.lanes.findIndex((l) => l.nodeIdentityKey === boundary.nodeIdentityKey)
    if (laneIndex < 0) continue

    const x = timeToXSafe(boundary.atMs, params)
    if (x < -1 || x > params.widthPx + 1) continue

    ctx.beginPath()
    ctx.moveTo(Math.round(x) + 0.5, top)
    ctx.lineTo(Math.round(x) + 0.5, bottom)
    ctx.stroke()

    // Marker so the boundary is visible even when it lands on a gridline.
    const band = laneBand(projection.layout, laneIndex)
    ctx.setLineDash([])
    ctx.fillStyle = theme.boundary
    ctx.beginPath()
    ctx.moveTo(x, band.top + 2)
    ctx.lineTo(x - 3.5, band.top + 8)
    ctx.lineTo(x + 3.5, band.top + 8)
    ctx.closePath()
    ctx.fill()
    ctx.setLineDash([3, 3])
  }

  ctx.restore()
}

function timeToXSafe(tMs: number, params: RenderParams): number {
  const span = params.viewport.endMs - params.viewport.startMs
  if (span <= 0) return 0
  return ((tMs - params.viewport.startMs) / span) * params.widthPx
}

function drawMarks(params: RenderParams): void {
  const { ctx, projection, theme } = params

  for (const mark of projection.marks) {
    const band = laneBand(projection.layout, mark.laneIndex)
    const color = toneColor(mark.semantic.tone, theme)
    const x = Math.round(mark.x) + 0.5

    if (mark.semantic.glyph === 'stem') {
      // Success: latency stem plus a dot at the tip.
      const top = band.baseline - mark.stemHeight
      ctx.strokeStyle = color
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(x, band.baseline)
      ctx.lineTo(x, top)
      ctx.stroke()

      ctx.fillStyle = color
      ctx.beginPath()
      ctx.arc(x, top, 1.7, 0, Math.PI * 2)
      ctx.fill()
      continue
    }

    if (mark.semantic.glyph === 'cross') {
      // Transport failure: a broken tick with an explicit cross.
      const cy = band.baseline - 5
      ctx.strokeStyle = color
      ctx.lineWidth = 1.4
      ctx.beginPath()
      ctx.moveTo(x - 3, cy - 3)
      ctx.lineTo(x + 3, cy + 3)
      ctx.moveTo(x + 3, cy - 3)
      ctx.lineTo(x - 3, cy + 3)
      ctx.stroke()

      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(x, band.baseline)
      ctx.lineTo(x, cy + 3)
      ctx.stroke()
      continue
    }

    if (mark.semantic.glyph === 'slash') {
      // Service-layer failure: a distinct slash so a blocked service is never mistaken for a
      // dead transport.
      ctx.strokeStyle = color
      ctx.lineWidth = 1.4
      ctx.beginPath()
      ctx.moveTo(x - 3, band.baseline - 1)
      ctx.lineTo(x + 3, band.baseline - 9)
      ctx.stroke()

      ctx.fillStyle = color
      ctx.beginPath()
      ctx.arc(x, band.baseline - 1, 1.4, 0, Math.PI * 2)
      ctx.fill()
      continue
    }

    // Unknown failure: hollow marker.
    ctx.strokeStyle = color
    ctx.lineWidth = 1.2
    ctx.beginPath()
    ctx.arc(x, band.baseline - 5, 2.6, 0, Math.PI * 2)
    ctx.stroke()
    ctx.beginPath()
    ctx.moveTo(x, band.baseline)
    ctx.lineTo(x, band.baseline - 2.4)
    ctx.stroke()
  }
}

function drawSelection(params: RenderParams): void {
  const { ctx, projection, theme, selectedSampleId, hoverSampleId, heightPx, axisHeightPx } = params
  if (projection.lanes.length === 0) return

  const top = projection.layout.plotTopPx
  const bottom = heightPx - axisHeightPx

  if (hoverSampleId && hoverSampleId !== selectedSampleId) {
    const mark = projection.marks.find((m) => m.sample.sampleId === hoverSampleId)
    if (mark) {
      const band = laneBand(projection.layout, mark.laneIndex)
      ctx.strokeStyle = theme.border
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(Math.round(mark.x) + 0.5, band.top)
      ctx.lineTo(Math.round(mark.x) + 0.5, band.bottom)
      ctx.stroke()
    }
  }

  if (!selectedSampleId) return
  const selected = projection.marks.find((m) => m.sample.sampleId === selectedSampleId)
  if (!selected) return

  const band = laneBand(projection.layout, selected.laneIndex)
  const x = Math.round(selected.x) + 0.5

  ctx.strokeStyle = theme.selection
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.moveTo(x, top)
  ctx.lineTo(x, bottom)
  ctx.stroke()

  ctx.fillStyle = theme.selection
  ctx.globalAlpha = 0.12
  ctx.fillRect(Math.floor(selected.x) - 2, band.top, 5, band.bottom - band.top)
  ctx.globalAlpha = 1

  const top2 = band.baseline - selected.stemHeight
  ctx.strokeStyle = theme.selection
  ctx.lineWidth = 1.6
  ctx.beginPath()
  ctx.arc(x, top2, 4.2, 0, Math.PI * 2)
  ctx.stroke()
}

function drawAxis(params: RenderParams): void {
  const { ctx, ticks, theme, heightPx, axisHeightPx, widthPx } = params

  ctx.font = AXIS_FONT
  ctx.textBaseline = 'middle'
  ctx.fillStyle = theme.axisText

  const y = heightPx - axisHeightPx / 2

  for (const tick of ticks) {
    const x = Math.round(tick.x) + 0.5
    ctx.strokeStyle = tick.major ? theme.gridlineMajor : theme.gridline
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(x, heightPx - axisHeightPx)
    ctx.lineTo(x, heightPx - axisHeightPx + 4)
    ctx.stroke()

    const textWidth = ctx.measureText(tick.label).width
    let textX = x + 3
    if (textX + textWidth > widthPx) textX = x - 3 - textWidth
    if (textX < 0) textX = 1
    ctx.fillText(tick.label, textX, y)
  }
}
