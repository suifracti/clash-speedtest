/**
 * Visual grammar for raw monitor samples.
 *
 * Design rule: shape and colour encode the *severity family* so a glance is enough to spot a
 * transport outage versus a single blocked service. The *exact* error class is never collapsed
 * away — it is always available in the inspector, in the mark tooltip, and in the error
 * breakdown of the observation summary.
 *
 * In particular a service/geo block (e.g. an AI or Google region restriction) must never be
 * rendered the same way as the node's transport being down.
 */

import type { MonitorSample } from '../../types'

export type SampleOutcome =
  | 'success'
  | 'transport_failure'
  | 'service_failure'
  | 'unknown_failure'

export type MarkGlyph = 'stem' | 'cross' | 'slash' | 'hollow'

export type MarkTone = 'success' | 'danger' | 'warning' | 'muted'

export interface ErrorClassMeta {
  /** Raw class exactly as persisted by the backend. */
  errorClass: string
  label: string
  family: SampleOutcome
  detail: string
}

const ERROR_CLASS_TABLE: Record<string, ErrorClassMeta> = {
  none: {
    errorClass: 'none',
    label: '成功',
    family: 'success',
    detail: '探针成功返回',
  },
  timeout: {
    errorClass: 'timeout',
    label: '超时',
    family: 'transport_failure',
    detail: '传输层未在超时窗口内返回，节点可能不可达',
  },
  conn_refused: {
    errorClass: 'conn_refused',
    label: '连接被拒绝',
    family: 'transport_failure',
    detail: '传输层连接被拒绝，节点入口不可用',
  },
  dns_error: {
    errorClass: 'dns_error',
    label: 'DNS 解析失败',
    family: 'transport_failure',
    detail: '域名解析失败，通常是链路或解析层问题',
  },
  tls_error: {
    errorClass: 'tls_error',
    label: 'TLS 握手失败',
    family: 'transport_failure',
    detail: 'TLS 握手失败，可能是链路干扰或证书问题',
  },
  blocked: {
    errorClass: 'blocked',
    label: '服务/地区阻断',
    family: 'service_failure',
    detail: '目标服务按策略或地区拒绝，节点本身可能仍然可用',
  },
  http_status_error: {
    errorClass: 'http_status_error',
    label: '服务层状态错误',
    family: 'service_failure',
    detail: 'HTTP 状态异常，属于目标服务层失败而非传输失败',
  },
}

const UNKNOWN_ERROR_META: ErrorClassMeta = {
  errorClass: 'unknown',
  label: '未知失败',
  family: 'unknown_failure',
  detail: '未归类的失败类型，请在 Inspector 中查看原始错误明细',
}

/** Resolves the raw error class to its presentation metadata. */
export function errorClassMeta(errorClass: string | undefined | null): ErrorClassMeta {
  if (!errorClass) return UNKNOWN_ERROR_META
  return ERROR_CLASS_TABLE[errorClass] ?? {
    ...UNKNOWN_ERROR_META,
    errorClass,
  }
}

export interface SampleSemantic {
  outcome: SampleOutcome
  glyph: MarkGlyph
  tone: MarkTone
  /** Raw error class, always preserved for evidence. */
  errorClass: string
  label: string
  detail: string
}

const OUTCOME_PRESENTATION: Record<SampleOutcome, { glyph: MarkGlyph; tone: MarkTone }> = {
  success: { glyph: 'stem', tone: 'success' },
  transport_failure: { glyph: 'cross', tone: 'danger' },
  service_failure: { glyph: 'slash', tone: 'warning' },
  unknown_failure: { glyph: 'hollow', tone: 'muted' },
}

/**
 * Derives the visual semantic for a single sample.
 *
 * `success` wins only when the sample actually reports success AND carries no error class, so a
 * malformed row can never be painted as healthy.
 */
export function sampleSemantic(sample: MonitorSample): SampleSemantic {
  const meta = errorClassMeta(sample.errorClass)
  const outcome: SampleOutcome = sample.success
    ? meta.family === 'success'
      ? 'success'
      : 'unknown_failure'
    : meta.family === 'success'
      ? 'unknown_failure'
      : meta.family

  const presentation = OUTCOME_PRESENTATION[outcome]
  return {
    outcome,
    glyph: presentation.glyph,
    tone: presentation.tone,
    errorClass: meta.errorClass,
    label: outcome === 'success' ? '成功' : meta.label,
    detail: meta.detail,
  }
}

export const TONE_COLORS: Record<MarkTone, string> = {
  success: '#059669',
  danger: '#dc2626',
  warning: '#d97706',
  muted: '#64748b',
}

export const OUTCOME_LABELS: Record<SampleOutcome, string> = {
  success: '成功',
  transport_failure: '传输失败',
  service_failure: '服务层失败',
  unknown_failure: '未归类失败',
}

/**
 * Robust latency ceiling for the stem-height scale.
 *
 * Uses a high percentile rather than the raw maximum so one spike does not flatten every other
 * mark, and applies a floor so an idle node's jitter is not visually amplified into drama.
 */
export function computeLatencyScale(samples: MonitorSample[], floorMs = 100): number {
  const values = samples
    .filter((s) => s.success && Number.isFinite(s.latencyMs) && s.latencyMs > 0)
    .map((s) => s.latencyMs)
    .sort((a, b) => a - b)

  if (values.length === 0) return floorMs

  const idx = Math.min(values.length - 1, Math.floor(values.length * 0.95))
  const p95 = values[idx]
  return Math.max(floorMs, p95 * 1.25)
}

/**
 * Maps a latency to a stem height in pixels using a square-root scale, which keeps small
 * differences legible at the bottom of the range without letting outliers dominate.
 * Exact numbers stay in the inspector; this only drives "shape for a glance".
 */
export function latencyToStemHeight(
  latencyMs: number,
  scaleMaxMs: number,
  maxHeightPx: number,
  minHeightPx = 3
): number {
  if (!Number.isFinite(latencyMs) || latencyMs <= 0) return minHeightPx
  const ceiling = scaleMaxMs > 0 ? scaleMaxMs : 1
  const ratio = Math.min(1, Math.max(0, latencyMs / ceiling))
  return minHeightPx + Math.sqrt(ratio) * Math.max(0, maxHeightPx - minHeightPx)
}

/** Height used for failure marks so they remain visible without implying a latency value. */
export const FAILURE_MARK_HEIGHT_PX = 8
