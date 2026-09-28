import type { WorkbenchLatencySample, WorkbenchLatencyTest } from '../../types'
import { countDistinctTestRuns } from './latencyLod'

export interface LatencyHistogramBin {
  min: number
  max: number
  label: string
  count: number
  percentage: number
  isNormalRange: boolean
}

export interface NodeHealthReport {
  sampleCount: number
  testCount: number
  successCount: number
  failCount: number
  successRate: number // 0 ~ 100
  successRateText: string // e.g. "98.7%"
  successRateClass: 'success' | 'warning' | 'danger'
  p50: number | null
  p95: number | null
  p10: number | null
  minLatency: number | null
  maxLatency: number | null
  normalMin: number | null
  normalMax: number | null
  normalPercentage: number // 0 ~ 100
  normalRangeText: string // e.g. "300–340 ms · 61%"
  spikeCount: number
  spikeRate: number // 0 ~ 100
  spikeRateText: string // e.g. "2.1%"
  driftRatio: number // recent vs older P50 drift, e.g. +0.12 or -0.08
  trend: 'stable' | 'improving' | 'degrading' | 'spiking' | 'failing' | 'insufficient'
  trendText: string // "↔ 稳定" | "↘ 改善" | "↗ 变慢" | "⚡ 偶发尖刺" | "⚡ 频发尖刺" | "✕ 丢包"
  trendClass: 'success' | 'info' | 'warning' | 'danger'
  diagnosisTitle: string // e.g. "健康 · 稳定"
  diagnosisDetail: string
  histogramBins: LatencyHistogramBin[]
}

export interface LatencyAttemptComparison {
  currentMs: number | null
  previousMs: number | null
  previousExists: boolean
  priorAverageMs: number | null
  priorWorstMs: number | null
  priorCount: number
}

// Compare like with like: each attempt contributes the mean of its successful raw samples.
export function compareLatencyAttempts(tests: WorkbenchLatencyTest[]): LatencyAttemptComparison {
  const ordered = [...tests].sort((a, b) =>
    Date.parse(b.finished_at) - Date.parse(a.finished_at) || b.attempt_id.localeCompare(a.attempt_id)
  )
  const means = ordered.map((test) => {
    const successful = (test.samples || []).filter((sample) => sample.success && sample.latency_ms > 0)
    return successful.length ? Math.round(successful.reduce((sum, sample) => sum + sample.latency_ms, 0) / successful.length) : null
  })
  const prior = means.slice(1).filter((value): value is number => value !== null)
  return {
    currentMs: means[0] ?? null,
    previousMs: means[1] ?? null,
    previousExists: means.length > 1,
    priorAverageMs: prior.length ? Math.round(prior.reduce((sum, value) => sum + value, 0) / prior.length) : null,
    priorWorstMs: prior.length ? Math.max(...prior) : null,
    priorCount: prior.length,
  }
}

/**
 * Computes an explainable, multi-dimensional health summary for a node based on observation window samples.
 */
export function computeNodeHealthReport(samples: WorkbenchLatencySample[]): NodeHealthReport | null {
  if (!samples.length) return null

  const sampleCount = samples.length
  const testCount = countDistinctTestRuns(samples)
  const successSamples = samples.filter((s) => s.success && s.latency_ms > 0)
  const successCount = successSamples.length
  const failCount = sampleCount - successCount

  const successRate = Math.round((successCount / sampleCount) * 1000) / 10
  const successRateText = `${successRate.toFixed(1)}%`
  const successRateClass: 'success' | 'warning' | 'danger' =
    successRate >= 98 ? 'success' : successRate >= 90 ? 'warning' : 'danger'

  if (successCount === 0) {
    return {
      sampleCount,
      testCount,
      successCount: 0,
      failCount,
      successRate: 0,
      successRateText: '0.0%',
      successRateClass: 'danger',
      p50: null,
      p95: null,
      p10: null,
      minLatency: null,
      maxLatency: null,
      normalMin: null,
      normalMax: null,
      normalPercentage: 0,
      normalRangeText: '全数失败',
      spikeCount: 0,
      spikeRate: 0,
      spikeRateText: '0.0%',
      driftRatio: 0,
      trend: 'failing',
      trendText: '✕ 全部失败',
      trendClass: 'danger',
      diagnosisTitle: '这些测试均未成功',
      diagnosisDetail: `观察周期内 ${testCount} 次测试共 ${sampleCount} 条样本全部连接失败或超时`,
      histogramBins: [],
    }
  }

  const sorted = [...successSamples].map((s) => s.latency_ms).sort((a, b) => a - b)
  const minLatency = Math.round(sorted[0])
  const maxLatency = Math.round(sorted[sorted.length - 1])
  const p50 = Math.round(sorted[Math.floor(sorted.length * 0.5)])
  const p95 = Math.round(sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))])
  const p10 = Math.round(sorted[Math.max(0, Math.floor(sorted.length * 0.10))])

  // 1. Compute Normal Latency Range (High Density Window)
  // Finds the interval of span W that captures the highest concentration of samples.
  let normalMin = p50
  let normalMax = p50
  let normalPercentage = 100

  if (sorted.length < 3) {
    normalMin = minLatency
    normalMax = maxLatency
    normalPercentage = 100
  } else {
    // Adaptive window span W: ~15% of median, minimum 20ms
    const W = Math.max(20, Math.round(p50 * 0.15))
    let maxClusterCount = 0
    let bestStartIdx = 0
    let bestEndIdx = 0

    let r = 0
    for (let l = 0; l < sorted.length; l++) {
      while (r < sorted.length && sorted[r] - sorted[l] <= W) {
        r++
      }
      const count = r - l
      if (count > maxClusterCount) {
        maxClusterCount = count
        bestStartIdx = l
        bestEndIdx = r - 1
      }
    }

    normalMin = Math.round(sorted[bestStartIdx])
    normalMax = Math.round(sorted[bestEndIdx])
    normalPercentage = Math.round((maxClusterCount / sorted.length) * 100)
  }

  const normalRangeText =
    normalMin === normalMax
      ? `~${normalMin} ms · ${normalPercentage}%`
      : `${normalMin}–${normalMax} ms · ${normalPercentage}%`

  // 2. Outlier Spike Detection (> max(500ms, P50 * 1.8))
  const spikeThreshold = Math.max(500, Math.round(p50 * 1.8))
  const spikeCount = sorted.filter((l) => l > spikeThreshold).length
  const spikeRate = Math.round((spikeCount / sorted.length) * 1000) / 10
  const spikeRateText = `${spikeRate.toFixed(1)}%`

  // 3. Drift & Trend Analysis (compare older half vs newer half by sample timestamp)
  const chronological = [...successSamples].sort(
    (a, b) => Date.parse(a.timestamp || '') - Date.parse(b.timestamp || '')
  )
  let driftRatio = 0
  if (chronological.length >= 6) {
    const half = Math.floor(chronological.length / 2)
    const oldHalf = chronological.slice(0, half).map((s) => s.latency_ms).sort((a, b) => a - b)
    const newHalf = chronological.slice(half).map((s) => s.latency_ms).sort((a, b) => a - b)
    const oldP50 = oldHalf[Math.floor(oldHalf.length * 0.5)] || 1
    const newP50 = newHalf[Math.floor(newHalf.length * 0.5)] || 1
    driftRatio = Math.round(((newP50 - oldP50) / oldP50) * 1000) / 1000
  }

  let trend: NodeHealthReport['trend'] = 'stable'
  let trendText = '↔ 延迟稳定'
  let trendClass: 'success' | 'info' | 'warning' | 'danger' = 'success'
  let diagnosisTitle = '健康 · 延迟稳定'
  let diagnosisDetail = `平时延迟 ${normalRangeText}，连接通畅率 ${successRateText}，表现平稳`

  if (successRate < 85) {
    trend = 'failing'
    trendText = '✕ 请求失败较多'
    trendClass = 'danger'
    diagnosisTitle = '异常 · 请求失败较多'
    diagnosisDetail = `连接通畅率仅 ${successRateText}，存在较多超时或连接失败，建议检查节点连通性`
  } else if (spikeRate >= 8) {
    trend = 'spiking'
    trendText = '⚡ 经常跳 ping'
    trendClass = 'warning'
    diagnosisTitle = '波动较大 · 经常跳 ping'
    diagnosisDetail = `平时延迟 ${normalMin}–${normalMax} ms，但有 ${spikeRateText} 的请求出现明显跳 ping (超过 ${spikeThreshold} ms)`
  } else if (spikeRate >= 3) {
    trend = 'spiking'
    trendText = '⚡ 偶有跳 ping'
    trendClass = 'warning'
    diagnosisTitle = '基本平稳 · 偶有跳 ping'
    diagnosisDetail = `平时稳定在 ${normalMin}–${normalMax} ms，仅偶发 ${spikeRateText} 的请求略有卡顿 (超过 ${spikeThreshold} ms)`
  } else if (driftRatio > 0.15) {
    trend = 'degrading'
    const pct = Math.round(driftRatio * 100)
    trendText = `↗ 延迟变慢 (+${pct}%)`
    trendClass = 'warning'
    diagnosisTitle = '变慢 · 延迟明显上升'
    diagnosisDetail = `近期测试延迟比前半周期上升约 ${pct}%，常态延迟出现上涨`
  } else if (driftRatio < -0.10) {
    trend = 'improving'
    const pct = Math.abs(Math.round(driftRatio * 100))
    trendText = `↘ 延迟改善 (-${pct}%)`
    trendClass = 'success'
    diagnosisTitle = '健康 · 延迟改善'
    diagnosisDetail = `近期测试延迟比前半周期降低约 ${pct}%，节点速度向好`
  }

  if (testCount < 2 || successCount < 6) {
    trend = 'insufficient'
    trendText = '样本不足，暂不判断趋势'
    trendClass = 'info'
    diagnosisTitle = '还需要更多测试'
    diagnosisDetail = `已记录 ${sampleCount} 条样本，其中 ${successCount} 条成功；仅凭这些记录不能判断长期是否稳定`
  }

  // 4. Histogram Bins (Distribution Breakdown)
  const histogramBins: LatencyHistogramBin[] = []
  if (sorted.length > 0 && sorted.length < 4) {
    // A few readings can still be counted honestly; they are not a long-term trend.
    const counts = new Map<number, number>()
    for (const latency of sorted) {
      const start = Math.floor(latency / 10) * 10
      counts.set(start, (counts.get(start) || 0) + 1)
    }
    for (const [start, count] of [...counts].sort((a, b) => a[0] - b[0])) {
      histogramBins.push({
        min: start,
        max: start + 9,
        label: `${start}–${start + 9} ms`,
        count,
        percentage: Math.round(count / sorted.length * 100),
        isNormalRange: normalMin >= start && normalMin < start + 10,
      })
    }
  } else if (sorted.length >= 4) {
    const binCount = Math.min(6, Math.max(4, Math.floor(Math.sqrt(sorted.length))))
    const binMin = Math.floor(minLatency / 10) * 10
    const binMax = Math.ceil(Math.min(p95 * 1.25, maxLatency) / 10) * 10
    const step = Math.max(10, Math.ceil((binMax - binMin) / binCount / 10) * 10)

    for (let b = 0; b < binCount; b++) {
      const bStart = binMin + b * step
      const bEnd = b === binCount - 1 ? Number.POSITIVE_INFINITY : bStart + step
      const inBin = sorted.filter((v) => v >= bStart && (bEnd === Number.POSITIVE_INFINITY ? true : v < bEnd))
      const count = inBin.length
      const percentage = Math.round((count / sorted.length) * 100)
      const label = bEnd === Number.POSITIVE_INFINITY ? `≥ ${bStart} ms` : `${bStart}–${bEnd} ms`
      const isNormal = normalMin >= bStart && (bEnd === Number.POSITIVE_INFINITY ? true : normalMin < bEnd)

      histogramBins.push({
        min: bStart,
        max: bEnd === Number.POSITIVE_INFINITY ? maxLatency : bEnd,
        label,
        count,
        percentage,
        isNormalRange: isNormal,
      })
    }
  }

  return {
    sampleCount,
    testCount,
    successCount,
    failCount,
    successRate,
    successRateText,
    successRateClass,
    p50,
    p95,
    p10,
    minLatency,
    maxLatency,
    normalMin,
    normalMax,
    normalPercentage,
    normalRangeText,
    spikeCount,
    spikeRate,
    spikeRateText,
    driftRatio,
    trend,
    trendText,
    trendClass,
    diagnosisTitle,
    diagnosisDetail,
    histogramBins,
  }
}
