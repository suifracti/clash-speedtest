/**
 * Pure viewport math for the monitor timeline.
 *
 * The timeline's horizontal axis is real time. Every function here is deterministic and
 * side-effect free so the mapping between a raw sample timestamp and its pixel position can
 * be unit tested independently of rendering.
 */

/** The visible time window plus the pixel width it is drawn into. */
export interface TimelineViewport {
  /** Left (oldest) edge of the visible window, epoch milliseconds. */
  startMs: number
  /** Right (newest) edge of the visible window, epoch milliseconds. */
  endMs: number
  /** Pixel width of the plotting area. */
  widthPx: number
}

export const MIN_VIEWPORT_SPAN_MS = 60_000
export const MAX_VIEWPORT_SPAN_MS = 400 * 24 * 60 * 60 * 1000

export function viewportSpanMs(vp: TimelineViewport): number {
  return vp.endMs - vp.startMs
}

/**
 * Maps an instant to a pixel column.
 *
 * The result is intentionally NOT clamped to the plot area: callers use out-of-range values
 * to cull samples that fall outside the viewport.
 */
export function timeToX(tMs: number, vp: TimelineViewport): number {
  const span = viewportSpanMs(vp)
  if (span <= 0 || vp.widthPx <= 0) return 0
  return ((tMs - vp.startMs) / span) * vp.widthPx
}

/** Inverse of {@link timeToX}. */
export function xToTime(x: number, vp: TimelineViewport): number {
  const span = viewportSpanMs(vp)
  if (vp.widthPx <= 0) return vp.startMs
  return vp.startMs + (x / vp.widthPx) * span
}

/** Milliseconds represented by one horizontal pixel. */
export function msPerPixel(vp: TimelineViewport): number {
  if (vp.widthPx <= 0) return 0
  return viewportSpanMs(vp) / vp.widthPx
}

function clampSpan(span: number): number {
  if (!Number.isFinite(span) || span <= 0) return MIN_VIEWPORT_SPAN_MS
  return Math.min(MAX_VIEWPORT_SPAN_MS, Math.max(MIN_VIEWPORT_SPAN_MS, span))
}

/**
 * Builds a viewport for an explicit time range.
 * The width is normalised to at least 1px so downstream division stays well defined.
 */
export function viewportForRange(startMs: number, endMs: number, widthPx: number): TimelineViewport {
  const width = Math.max(1, Math.floor(widthPx))
  const span = clampSpan(endMs - startMs)
  return { startMs, endMs: startMs + span, widthPx: width }
}

/**
 * Zooms around a pixel anchor, keeping the instant currently under that anchor fixed.
 *
 * @param factor >1 zooms out (wider span), <1 zooms in.
 * @param anchorPx pixel column that must not move.
 */
export function zoomViewport(vp: TimelineViewport, factor: number, anchorPx: number): TimelineViewport {
  const span = viewportSpanMs(vp)
  if (span <= 0 || !Number.isFinite(factor) || factor <= 0) return vp

  const nextSpan = clampSpan(span * factor)
  if (nextSpan === span) return vp

  const anchor = Math.min(Math.max(anchorPx, 0), vp.widthPx)
  const anchorTime = xToTime(anchor, vp)
  const ratio = vp.widthPx > 0 ? anchor / vp.widthPx : 0

  const startMs = anchorTime - ratio * nextSpan
  return { startMs, endMs: startMs + nextSpan, widthPx: vp.widthPx }
}

/** Pans by a pixel delta (positive delta moves the window forward in time). */
export function panViewport(vp: TimelineViewport, deltaPx: number): TimelineViewport {
  const dt = deltaPx * msPerPixel(vp)
  if (!Number.isFinite(dt) || dt === 0) return vp
  return { startMs: vp.startMs + dt, endMs: vp.endMs + dt, widthPx: vp.widthPx }
}

/** Pans by an absolute time delta. */
export function panViewportByMs(vp: TimelineViewport, deltaMs: number): TimelineViewport {
  if (!Number.isFinite(deltaMs) || deltaMs === 0) return vp
  return { startMs: vp.startMs + deltaMs, endMs: vp.endMs + deltaMs, widthPx: vp.widthPx }
}

/**
 * Constrains the viewport to the selectable domain (the active time-range filter).
 *
 * The domain — not the set of currently loaded samples — is the clamp boundary, because
 * panning towards the oldest edge of the domain is exactly what triggers loading more pages.
 * A viewport wider than the domain is anchored to the domain's newest edge so the "latest"
 * instant stays reachable.
 */
export function clampViewportToDomain(
  vp: TimelineViewport,
  domainStartMs: number,
  domainEndMs: number
): TimelineViewport {
  const span = viewportSpanMs(vp)
  const domainSpan = domainEndMs - domainStartMs
  if (span <= 0 || domainSpan <= 0) return vp

  if (span >= domainSpan) {
    return { startMs: domainEndMs - span, endMs: domainEndMs, widthPx: vp.widthPx }
  }

  let startMs = vp.startMs
  if (startMs < domainStartMs) startMs = domainStartMs
  if (startMs + span > domainEndMs) startMs = domainEndMs - span

  return { startMs, endMs: startMs + span, widthPx: vp.widthPx }
}

/** True when the viewport's newest edge is within `toleranceMs` of `latestMs`. */
export function isAtLiveEdge(vp: TimelineViewport, latestMs: number, toleranceMs = 1000): boolean {
  return Math.abs(vp.endMs - latestMs) <= toleranceMs
}

/**
 * True when the viewport is close enough to the oldest loaded sample that the caller should
 * request the next (older) page.
 */
export function needsOlderData(
  vp: TimelineViewport,
  oldestLoadedMs: number | null,
  thresholdPx = 120
): boolean {
  if (oldestLoadedMs === null) return false
  const x = timeToX(oldestLoadedMs, vp)
  return x > -thresholdPx
}
