import { describe, expect, it } from 'vitest'
import { computeLodPlan, countDistinctTestRuns } from '../latencyLod'
import type { WorkbenchLatencySample } from '../../../types'

describe('latencyLod Level of Detail plan', () => {
  it('counts distinct test runs based on timestamp gaps', () => {
    const samples: WorkbenchLatencySample[] = [
      // Test 1 (3 pings within 2s)
      { seq: 1, timestamp: '2026-09-25T10:00:00.000Z', latency_ms: 100, success: true },
      { seq: 2, timestamp: '2026-09-25T10:00:01.000Z', latency_ms: 105, success: true },
      { seq: 3, timestamp: '2026-09-25T10:00:02.000Z', latency_ms: 102, success: true },
      // Test 2 (1 hour later)
      { seq: 1, timestamp: '2026-09-25T11:00:00.000Z', latency_ms: 120, success: true },
      { seq: 2, timestamp: '2026-09-25T11:00:01.000Z', latency_ms: 122, success: true },
    ]
    expect(countDistinctTestRuns(samples)).toBe(2)
  })

  it('preserves raw mode for short windows (4h and 24h)', () => {
    const samples: WorkbenchLatencySample[] = [
      { seq: 1, timestamp: '2026-09-25T10:00:00.000Z', latency_ms: 100, success: true },
    ]
    const plan4h = computeLodPlan({
      samples,
      windowSince: '2026-09-25T08:00:00.000Z',
      windowUntil: '2026-09-25T12:00:00.000Z',
    })
    expect(plan4h.mode).toBe('raw')

    const plan24h = computeLodPlan({
      samples,
      windowSince: '2026-09-24T12:00:00.000Z',
      windowUntil: '2026-09-25T12:00:00.000Z',
    })
    expect(plan24h.mode).toBe('raw')
  })

  it('keeps raw mode in 7-day window when test count is sparse (density protection)', () => {
    // 20 tests across 7 days
    const samples: WorkbenchLatencySample[] = []
    for (let i = 0; i < 20; i += 1) {
      const d = new Date(Date.parse('2026-09-18T12:00:00.000Z') + i * 8 * 3600 * 1000)
      samples.push({ seq: 1, timestamp: d.toISOString(), latency_ms: 100 + i, success: true })
      samples.push({ seq: 2, timestamp: new Date(d.getTime() + 1000).toISOString(), latency_ms: 102 + i, success: true })
    }

    const plan = computeLodPlan({
      samples,
      windowSince: '2026-09-18T12:00:00.000Z',
      windowUntil: '2026-09-25T12:00:00.000Z',
      availableWidth: 692,
    })
    // 20 tests <= maxUnits (86), so raw mode is preserved!
    expect(plan.mode).toBe('raw')
  })

  it('automatically switches to bucket mode in 7-day window when test count is dense (> maxUnits)', () => {
    // 200 tests across 7 days
    const samples: WorkbenchLatencySample[] = []
    for (let i = 0; i < 200; i += 1) {
      const d = new Date(Date.parse('2026-09-18T12:00:00.000Z') + i * 45 * 60 * 1000)
      samples.push({ seq: 1, timestamp: d.toISOString(), latency_ms: 100, success: true })
    }

    const plan = computeLodPlan({
      samples,
      windowSince: '2026-09-18T12:00:00.000Z',
      windowUntil: '2026-09-25T12:00:00.000Z',
      availableWidth: 692,
    })
    expect(plan.mode).toBe('bucket')
    expect(plan.buckets).toBeDefined()
    expect(plan.bucketIntervalMs).toBe(2 * 3600 * 1000) // 2 hours
    expect(plan.bucketIntervalLabel).toBe('2小时')
  })

  it('uses macro bucket summary mode for 30-day and 180-day windows with robust statistics', () => {
    const samples: WorkbenchLatencySample[] = [
      // Day 1: 3 samples (100ms, 200ms, 300ms)
      { seq: 1, timestamp: '2026-09-01T12:00:00.000Z', latency_ms: 100, success: true },
      { seq: 2, timestamp: '2026-09-01T12:00:01.000Z', latency_ms: 200, success: true },
      { seq: 3, timestamp: '2026-09-01T12:00:02.000Z', latency_ms: 300, success: true },
      // Day 15: 2 samples (150ms, failed)
      { seq: 1, timestamp: '2026-09-15T12:00:00.000Z', latency_ms: 150, success: true },
      { seq: 2, timestamp: '2026-09-15T12:00:01.000Z', latency_ms: 0, success: false, error: '超时' },
    ]

    const plan30d = computeLodPlan({
      samples,
      windowSince: '2026-09-01T00:00:00.000Z',
      windowUntil: '2026-10-01T00:00:00.000Z',
      availableWidth: 692,
    })

    expect(plan30d.mode).toBe('bucket')
    expect(plan30d.buckets?.length).toBe(2) // 2 buckets contain data

    const bucket1 = plan30d.buckets![0]
    expect(bucket1.sampleCount).toBe(3)
    expect(bucket1.medianLatency).toBe(200)
    expect(bucket1.representativeSampleIndex).toBe(1) // sample with 200ms

    const bucket2 = plan30d.buckets![1]
    expect(bucket2.sampleCount).toBe(2)
    expect(bucket2.successCount).toBe(1)
    expect(bucket2.failCount).toBe(1)
    expect(bucket2.successRate).toBe(0.5)
  })
})
