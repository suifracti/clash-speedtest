import { describe, expect, it } from 'vitest'
import {
  computeLatencyScale,
  errorClassMeta,
  latencyToStemHeight,
  sampleSemantic,
} from '../encoding'
import { makeSample } from './fixtures'

describe('error class taxonomy', () => {
  it('keeps every persisted class distinguishable by label', () => {
    const classes = [
      'timeout',
      'dns_error',
      'tls_error',
      'conn_refused',
      'blocked',
      'http_status_error',
    ]
    const labels = classes.map((c) => errorClassMeta(c).label)
    expect(new Set(labels).size).toBe(classes.length)
  })

  it('classifies transport failures separately from service failures', () => {
    for (const c of ['timeout', 'dns_error', 'tls_error', 'conn_refused']) {
      expect(errorClassMeta(c).family).toBe('transport_failure')
    }
    for (const c of ['blocked', 'http_status_error']) {
      expect(errorClassMeta(c).family).toBe('service_failure')
    }
  })

  it('treats a region/service block as a service failure, not a dead node', () => {
    const meta = errorClassMeta('blocked')
    expect(meta.family).toBe('service_failure')
    expect(meta.family).not.toBe('transport_failure')
    expect(meta.detail).toContain('节点本身可能仍然可用')
  })

  it('preserves the raw class for unknown values instead of silently mapping to success', () => {
    const meta = errorClassMeta('some_future_class')
    expect(meta.family).toBe('unknown_failure')
    expect(meta.errorClass).toBe('some_future_class')
  })

  it('treats a missing class as unknown, not as success', () => {
    expect(errorClassMeta(undefined).family).toBe('unknown_failure')
    expect(errorClassMeta('').family).toBe('unknown_failure')
  })
})

describe('sample semantic mapping', () => {
  it('maps a successful sample to the success stem', () => {
    const s = sampleSemantic(makeSample({ success: true, errorClass: 'none' }))
    expect(s.outcome).toBe('success')
    expect(s.glyph).toBe('stem')
    expect(s.tone).toBe('success')
  })

  it('maps a transport failure to a danger cross', () => {
    const s = sampleSemantic(makeSample({ success: false, errorClass: 'timeout' }))
    expect(s.outcome).toBe('transport_failure')
    expect(s.glyph).toBe('cross')
    expect(s.tone).toBe('danger')
    expect(s.errorClass).toBe('timeout')
  })

  it('maps a service failure to a warning slash, visually distinct from a transport failure', () => {
    const service = sampleSemantic(makeSample({ success: false, errorClass: 'blocked' }))
    const transport = sampleSemantic(makeSample({ success: false, errorClass: 'timeout' }))
    expect(service.outcome).toBe('service_failure')
    expect(service.tone).toBe('warning')
    expect(service.glyph).not.toBe(transport.glyph)
    expect(service.tone).not.toBe(transport.tone)
  })

  it('never paints a success flag over a real error class', () => {
    const s = sampleSemantic(makeSample({ success: true, errorClass: 'timeout' }))
    expect(s.outcome).not.toBe('success')
  })

  it('never paints success when the backend reports failure without a class', () => {
    const s = sampleSemantic(makeSample({ success: false, errorClass: 'none' }))
    expect(s.outcome).toBe('unknown_failure')
  })

  it('carries the exact error class through for evidence', () => {
    for (const c of ['timeout', 'dns_error', 'tls_error', 'conn_refused', 'blocked']) {
      expect(sampleSemantic(makeSample({ success: false, errorClass: c })).errorClass).toBe(c)
    }
  })
})

describe('latency stem scaling', () => {
  it('is monotonic in latency', () => {
    const heights = [1, 10, 50, 100, 200, 500].map((ms) => latencyToStemHeight(ms, 500, 40))
    for (let i = 1; i < heights.length; i += 1) {
      expect(heights[i]).toBeGreaterThanOrEqual(heights[i - 1])
    }
  })

  it('never exceeds the maximum height', () => {
    expect(latencyToStemHeight(10_000, 500, 40)).toBeLessThanOrEqual(40)
  })

  it('keeps a visible minimum for tiny or missing latencies', () => {
    expect(latencyToStemHeight(0, 500, 40, 3)).toBe(3)
    expect(latencyToStemHeight(Number.NaN, 500, 40, 3)).toBe(3)
  })

  it('handles a zero scale ceiling without dividing by zero', () => {
    expect(Number.isFinite(latencyToStemHeight(50, 0, 40))).toBe(true)
  })
})

describe('computeLatencyScale', () => {
  it('uses a high percentile so a single spike does not flatten the rest', () => {
    const samples = [
      ...Array.from({ length: 99 }, () => makeSample({ latencyMs: 50 })),
      makeSample({ latencyMs: 5000 }),
    ]
    const scale = computeLatencyScale(samples, 100)
    expect(scale).toBeLessThan(5000)
    expect(scale).toBeGreaterThan(50)
  })

  it('applies a floor so idle jitter is not amplified', () => {
    const samples = [makeSample({ latencyMs: 2 }), makeSample({ latencyMs: 3 })]
    expect(computeLatencyScale(samples, 100)).toBe(100)
  })

  it('ignores failures when computing the scale', () => {
    const samples = [
      makeSample({ latencyMs: 40 }),
      makeSample({ success: false, errorClass: 'timeout', latencyMs: 9999 }),
    ]
    expect(computeLatencyScale(samples, 10)).toBeCloseTo(40 * 1.25, 6)
  })

  it('falls back to the floor when there is no successful sample', () => {
    expect(computeLatencyScale([], 120)).toBe(120)
    expect(
      computeLatencyScale([makeSample({ success: false, errorClass: 'timeout' })], 120)
    ).toBe(120)
  })
})
