import type { WorkbenchLatencySample } from '../../types'

export interface TimeBucket {
  index: number
  startTime: number
  endTime: number
  timeCenter: number
  startLabel: string
  endLabel: string
  sampleIndices: number[]
  sampleCount: number
  testCount: number
  successCount: number
  failCount: number
  successRate: number // 0 to 1
  minLatency: number | null
  maxLatency: number | null
  medianLatency: number | null
  p95Latency: number | null
  p10Latency: number | null
  hasOutlier: boolean
  representativeSampleIndex: number
}

export type LodMode = 'raw' | 'bucket'

export interface LodPlan {
  mode: LodMode
  maxUnits: number
  bucketIntervalMs?: number
  bucketIntervalLabel?: string
  buckets?: TimeBucket[]
}

const STANDARD_INTERVALS: { ms: number; label: string }[] = [
  { ms: 15 * 60 * 1000, label: '15分钟' },
  { ms: 30 * 60 * 1000, label: '30分钟' },
  { ms: 60 * 60 * 1000, label: '1小时' },
  { ms: 2 * 3600 * 1000, label: '2小时' },
  { ms: 4 * 3600 * 1000, label: '4小时' },
  { ms: 6 * 3600 * 1000, label: '6小时' },
  { ms: 12 * 3600 * 1000, label: '12小时' },
  { ms: 24 * 3600 * 1000, label: '1天' },
  { ms: 2 * 24 * 3600 * 1000, label: '2天' },
  { ms: 3 * 24 * 3600 * 1000, label: '3天' },
  { ms: 7 * 24 * 3600 * 1000, label: '7天' },
]

export function countDistinctTestRuns(samples: WorkbenchLatencySample[], thresholdMs = 25000): number {
  if (samples.length === 0) return 0
  let count = 0
  let prevTime = Number.NEGATIVE_INFINITY
  for (const s of samples) {
    const t = Date.parse(s.timestamp || '')
    if (!Number.isFinite(t)) continue
    if (t - prevTime > thresholdMs) {
      count += 1
      prevTime = t
    }
  }
  return Math.max(1, count)
}

function formatBucketDate(d: Date, spanMs: number): string {
  if (Number.isNaN(d.getTime())) return '时间未知'
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  if (spanMs > 14 * 24 * 3600 * 1000) {
    return `${m}/${day}`
  }
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  if (spanMs > 28 * 3600 * 1000) {
    return `${m}/${day} ${hh}:${mm}`
  }
  return `${hh}:${mm}`
}

export function computeLodPlan(params: {
  samples: WorkbenchLatencySample[]
  windowSince?: string
  windowUntil?: string
  availableWidth?: number
  normalYMax?: number
}): LodPlan {
  const { samples, windowSince, windowUntil, availableWidth = 692, normalYMax } = params
  const maxUnits = Math.min(100, Math.max(40, Math.floor(availableWidth / 8)))

  if (!samples.length || !windowSince || !windowUntil) {
    return { mode: 'raw', maxUnits }
  }

  const min = Date.parse(windowSince)
  const max = Date.parse(windowUntil)
  if (!Number.isFinite(min) || !Number.isFinite(max) || min >= max) {
    return { mode: 'raw', maxUnits }
  }

  const spanMs = max - min
  const isShortWindow = spanMs <= 28 * 3600 * 1000 // 4h or 24h
  const isSevenDayWindow = spanMs <= 8 * 24 * 3600 * 1000 // 7d

  const distinctTests = countDistinctTestRuns(samples)

  // Level of Detail Decision Rule:
  // 1. Short windows (4h, 24h): always raw mode for high-fidelity evidence
  // 2. 7d: density protection (raw if tests <= maxUnits, bucket if dense)
  // 3. 30d, 180d: trend summary mode (always bucket mode)
  if (isShortWindow) {
    return { mode: 'raw', maxUnits }
  }

  if (isSevenDayWindow && distinctTests <= maxUnits) {
    return { mode: 'raw', maxUnits }
  }

  // Bucket Mode
  let chosen = STANDARD_INTERVALS[STANDARD_INTERVALS.length - 1]
  for (const interval of STANDARD_INTERVALS) {
    if (Math.ceil(spanMs / interval.ms) <= maxUnits) {
      chosen = interval
      break
    }
  }

  const intervalMs = chosen.ms
  const bucketCount = Math.max(1, Math.ceil(spanMs / intervalMs))

  const bucketMap = new Map<number, {
    startTime: number
    endTime: number
    sampleIndices: number[]
  }>()

  // Initialize empty buckets so timeline is continuous and evenly spaced
  for (let b = 0; b < bucketCount; b += 1) {
    const bStart = min + b * intervalMs
    const bEnd = Math.min(max, bStart + intervalMs)
    bucketMap.set(b, {
      startTime: bStart,
      endTime: bEnd,
      sampleIndices: [],
    })
  }

  // Populate samples into buckets
  samples.forEach((sample, idx) => {
    const t = Date.parse(sample.timestamp || '')
    if (!Number.isFinite(t) || t < min || t > max) return
    const bIdx = Math.min(bucketCount - 1, Math.max(0, Math.floor((t - min) / intervalMs)))
    const b = bucketMap.get(bIdx)
    if (b) {
      b.sampleIndices.push(idx)
    }
  })

  const buckets: TimeBucket[] = []

  for (let b = 0; b < bucketCount; b += 1) {
    const bInfo = bucketMap.get(b)!
    const indices = bInfo.sampleIndices
    if (indices.length === 0) continue

    const bStart = bInfo.startTime
    const bEnd = bInfo.endTime
    const timeCenter = (bStart + bEnd) / 2
    const startLabel = formatBucketDate(new Date(bStart), spanMs)
    const endLabel = formatBucketDate(new Date(bEnd), spanMs)

    const bucketSamples = indices.map((i) => samples[i])
    const testCount = countDistinctTestRuns(bucketSamples)
    const successSamples = bucketSamples.filter((s) => s.success && s.latency_ms > 0)
    const successCount = successSamples.length
    const failCount = indices.length - successCount
    const successRate = indices.length > 0 ? successCount / indices.length : 0

    let minLatency: number | null = null
    let maxLatency: number | null = null
    let medianLatency: number | null = null
    let p95Latency: number | null = null
    let p10Latency: number | null = null
    let hasOutlier = false
    let representativeSampleIndex = indices[0]

    if (successCount > 0) {
      const sorted = [...successSamples].sort((a, b) => a.latency_ms - b.latency_ms)
      minLatency = sorted[0].latency_ms
      maxLatency = sorted[sorted.length - 1].latency_ms
      const midIdx = Math.floor(sorted.length / 2)
      medianLatency = sorted[midIdx].latency_ms
      const p95Idx = Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))
      p95Latency = sorted[p95Idx].latency_ms
      const p10Idx = Math.max(0, Math.floor(sorted.length * 0.10))
      p10Latency = sorted[p10Idx].latency_ms

      if (normalYMax !== undefined && maxLatency > normalYMax) {
        hasOutlier = true
      }

      // Find sample closest to median
      let bestIdx = indices[0]
      let minDiff = Number.POSITIVE_INFINITY
      for (const idx of indices) {
        const s = samples[idx]
        if (s.success && s.latency_ms > 0) {
          const diff = Math.abs(s.latency_ms - medianLatency)
          if (diff < minDiff) {
            minDiff = diff
            bestIdx = idx
          }
        }
      }
      representativeSampleIndex = bestIdx
    }

    buckets.push({
      index: b,
      startTime: bStart,
      endTime: bEnd,
      timeCenter,
      startLabel,
      endLabel,
      sampleIndices: indices,
      sampleCount: indices.length,
      testCount,
      successCount,
      failCount,
      successRate,
      minLatency,
      maxLatency,
      medianLatency,
      p95Latency,
      p10Latency,
      hasOutlier,
      representativeSampleIndex,
    })
  }

  return {
    mode: 'bucket',
    maxUnits,
    bucketIntervalMs: intervalMs,
    bucketIntervalLabel: chosen.label,
    buckets,
  }
}
