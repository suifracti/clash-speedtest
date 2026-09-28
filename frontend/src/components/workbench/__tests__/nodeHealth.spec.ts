import { describe, expect, it } from 'vitest'
import { compareLatencyAttempts, computeNodeHealthReport } from '../nodeHealth'
import type { WorkbenchLatencySample, WorkbenchLatencyTest } from '../../../types'

describe('nodeHealth computation', () => {
  it('compares the latest attempt mean with prior attempt means, not the latest raw point', () => {
    const makeAttempt = (id: string, minute: number, latencies: number[]) => ({
      attempt_id: id,
      finished_at: `2026-09-26T10:0${minute}:00Z`,
      samples: latencies.map((latency_ms, index) => ({ seq: index + 1, timestamp: `2026-09-26T10:0${minute}:00Z`, latency_ms, success: true })),
    }) as WorkbenchLatencyTest
    const comparison = compareLatencyAttempts([
      makeAttempt('older', 1, [100, 100]),
      makeAttempt('latest', 3, [250, 350]),
      makeAttempt('previous', 2, [200, 200]),
    ])
    expect(comparison).toMatchObject({ currentMs: 300, previousMs: 200, priorAverageMs: 150, priorWorstMs: 200, priorCount: 2 })
  })

  it('returns null for empty samples', () => {
    expect(computeNodeHealthReport([])).toBeNull()
  })

  it('does not infer stable health from a single successful reading', () => {
    const report = computeNodeHealthReport([{ seq: 1, timestamp: '2026-09-26T10:00:00Z', latency_ms: 100, success: true }])
    expect(report?.successCount).toBe(1)
    expect(report?.trend).toBe('insufficient')
    expect(report?.histogramBins).toEqual([{ min: 100, max: 109, label: '100–109 ms', count: 1, percentage: 100, isNormalRange: true }])
  })

  it('counts small-sample latency ranges without calling them stable', () => {
    const report = computeNodeHealthReport([253, 257, 1488].map((latency_ms, index) => ({ seq: index + 1, timestamp: '2026-09-26T10:00:00Z', latency_ms, success: true })))
    expect(report?.trend).toBe('insufficient')
    expect(report?.histogramBins.map(bin => [bin.label, bin.count])).toEqual([['250–259 ms', 2], ['1480–1489 ms', 1]])
  })

  it('correctly handles all failing samples', () => {
    const samples: WorkbenchLatencySample[] = [
      { seq: 1, timestamp: '2026-09-25T10:00:00Z', latency_ms: 0, success: false, error: '连接超时' },
      { seq: 2, timestamp: '2026-09-25T10:00:01Z', latency_ms: 0, success: false, error: '连接拒绝' },
    ]
    const report = computeNodeHealthReport(samples)
    expect(report).not.toBeNull()
    expect(report?.successRate).toBe(0)
    expect(report?.trend).toBe('failing')
    expect(report?.normalRangeText).toBe('全数失败')
  })

  it('computes normal latency range and high concentration window', () => {
    // 10 samples: 6 of them tightly clustered between 310 and 330ms
    const samples: WorkbenchLatencySample[] = [
      { seq: 1, timestamp: '2026-09-25T10:00:00Z', latency_ms: 312, success: true },
      { seq: 2, timestamp: '2026-09-25T10:00:01Z', latency_ms: 315, success: true },
      { seq: 3, timestamp: '2026-09-25T10:00:02Z', latency_ms: 318, success: true },
      { seq: 4, timestamp: '2026-09-25T10:00:03Z', latency_ms: 320, success: true },
      { seq: 5, timestamp: '2026-09-25T10:00:04Z', latency_ms: 325, success: true },
      { seq: 6, timestamp: '2026-09-25T10:00:05Z', latency_ms: 328, success: true },
      { seq: 1, timestamp: '2026-09-25T11:00:00Z', latency_ms: 410, success: true },
      { seq: 2, timestamp: '2026-09-25T11:00:01Z', latency_ms: 420, success: true },
      { seq: 3, timestamp: '2026-09-25T11:00:02Z', latency_ms: 450, success: true },
      { seq: 4, timestamp: '2026-09-25T11:00:03Z', latency_ms: 480, success: true },
    ]

    const report = computeNodeHealthReport(samples)
    expect(report).not.toBeNull()
    expect(report?.sampleCount).toBe(10)
    expect(report?.successRate).toBe(100)
    expect(report?.p50).toBe(328) // middle value
    // Normal range should capture the 312-328 cluster
    expect(report?.normalMin).toBeLessThanOrEqual(315)
    expect(report?.normalMax).toBeGreaterThanOrEqual(325)
    expect(report?.normalPercentage).toBeGreaterThanOrEqual(60)
    expect(report?.normalRangeText).toContain('60%')
  })

  it('detects outlier spikes and flags trend as spiking', () => {
    const samples: WorkbenchLatencySample[] = [
      { seq: 1, timestamp: '2026-09-25T10:00:00Z', latency_ms: 300, success: true },
      { seq: 2, timestamp: '2026-09-25T10:00:01Z', latency_ms: 305, success: true },
      { seq: 3, timestamp: '2026-09-25T10:00:02Z', latency_ms: 310, success: true },
      { seq: 4, timestamp: '2026-09-25T10:00:03Z', latency_ms: 308, success: true },
      { seq: 5, timestamp: '2026-09-25T10:00:04Z', latency_ms: 312, success: true },
      { seq: 6, timestamp: '2026-09-25T10:00:05Z', latency_ms: 304, success: true },
      { seq: 1, timestamp: '2026-09-25T11:00:00Z', latency_ms: 310, success: true },
      { seq: 2, timestamp: '2026-09-25T11:00:01Z', latency_ms: 315, success: true },
      // 2 extreme spikes (> 500ms and > 1.8x median)
      { seq: 3, timestamp: '2026-09-25T11:00:02Z', latency_ms: 1250, success: true },
      { seq: 4, timestamp: '2026-09-25T11:00:03Z', latency_ms: 1400, success: true },
    ]

    const report = computeNodeHealthReport(samples)
    expect(report?.spikeCount).toBe(2)
    expect(report?.spikeRate).toBe(20)
    expect(report?.trend).toBe('spiking')
    expect(report?.trendText).toContain('经常跳 ping')
  })
})
