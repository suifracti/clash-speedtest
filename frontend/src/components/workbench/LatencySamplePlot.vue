<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { WorkbenchLatencySample } from '../../types'
import { computeLodPlan, type LodPlan, type TimeBucket } from './latencyLod'

const props = withDefaults(defineProps<{
  samples: WorkbenchLatencySample[]
  windowSince?: string
  windowUntil?: string
  width?: number
  height?: number
  hoveredIndex: number | null
  pinnedIndex: number | null
  showTimeBoundsLabels?: boolean
}>(), {
  height: 112,
  showTimeBoundsLabels: false,
})

const emit = defineEmits<{
  (event: 'hover', index: number | null): void
  (event: 'pin', index: number): void
  (event: 'unpin'): void
  (event: 'bucket-hover', bucket: TimeBucket | null): void
  (event: 'bucket-click', bucket: TimeBucket): void
}>()

const containerRef = ref<HTMLDivElement | null>(null)
const svgRef = ref<SVGSVGElement | null>(null)
const measuredWidth = ref<number>(props.width || 760)
const hoveredBucketIndex = ref<number | null>(null)

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  if (containerRef.value) {
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver((entries) => {
        for (const entry of entries) {
          const w = Math.round(entry.contentRect.width)
          if (w > 0) {
            measuredWidth.value = w
          }
        }
      })
      resizeObserver.observe(containerRef.value)
    }
    const initialWidth = containerRef.value.getBoundingClientRect().width
    if (initialWidth > 0) {
      measuredWidth.value = Math.round(initialWidth)
    }
  }
})

onUnmounted(() => {
  resizeObserver?.disconnect()
})

const effectiveWidth = computed(() => {
  if (props.width && props.width > 0) return props.width
  return measuredWidth.value > 0 ? measuredWidth.value : 760
})

const plotLeft = 52
const plotRight = computed(() => effectiveWidth.value - 16)
const plotTop = 16
const plotBottom = computed(() => props.height - 26)

const successSamples = computed(() => props.samples.filter((sample) => sample.success && sample.latency_ms > 0))

// Normal scale vs outlier break metrics
const scaleMetrics = computed(() => {
  if (successSamples.value.length === 0) {
    return {
      hasAxisBreak: false,
      normalYMax: 100,
      maxLatency: 0,
      breakY: plotTop + 14,
    }
  }

  const latencies = successSamples.value.map((s) => s.latency_ms).sort((a, b) => a - b)
  const maxLatency = latencies[latencies.length - 1]
  // In small sample sets, p75 accurately distinguishes the normal cluster from isolated spikes
  const p75Index = Math.min(latencies.length - 2 >= 0 ? latencies.length - 2 : 0, Math.floor(latencies.length * 0.75))
  const p75 = latencies[p75Index]

  // If the max outlier is > 2.0x of the bulk and > 500ms, break the axis so normal range isn't compressed
  const hasAxisBreak = maxLatency > Math.max(500, p75 * 2.0)
  let normalYMax: number
  if (hasAxisBreak) {
    const normalSamples = latencies.filter((l) => l <= Math.max(500, p75 * 2.0))
    const maxNormal = normalSamples.length ? normalSamples[normalSamples.length - 1] : p75
    normalYMax = Math.max(100, Math.ceil((maxNormal * 1.35) / 50) * 50)
  } else {
    normalYMax = Math.max(100, Math.ceil((maxLatency * 1.15) / 10) * 10)
  }

  const breakY = plotTop + 14
  return { hasAxisBreak, normalYMax, maxLatency, breakY }
})

const timeBounds = computed(() => {
  const windowMin = Date.parse(props.windowSince || '')
  const windowMax = Date.parse(props.windowUntil || '')
  const sampleTimes = props.samples
    .map((sample) => Date.parse(sample.timestamp))
    .filter((time) => Number.isFinite(time))
  const latestSampleTime = sampleTimes.length ? Math.max(...sampleTimes) : 0
  if (Number.isFinite(windowMin) && Number.isFinite(windowMax) && windowMin < windowMax) {
    const effectiveMax = Math.max(windowMax, latestSampleTime)
    return { min: windowMin, max: effectiveMax }
  }
  if (sampleTimes.length === 0) return { min: 0, max: 1 }
  const min = Math.min(...sampleTimes)
  const max = Math.max(...sampleTimes)
  return min === max ? { min: min - 500, max: max + 500 } : { min, max }
})

function xAt(index: number): number {
  const timestamp = Date.parse(props.samples[index]?.timestamp || '')
  if (!Number.isFinite(timestamp)) return plotLeft
  const span = timeBounds.value.max - timeBounds.value.min
  return plotLeft + ((timestamp - timeBounds.value.min) / span) * (plotRight.value - plotLeft)
}

function dataYAtLatency(latencyMs: number): number {
  if (latencyMs <= 0) return plotBottom.value
  const { hasAxisBreak, normalYMax, maxLatency, breakY } = scaleMetrics.value
  if (!hasAxisBreak) {
    const ratio = Math.max(0, Math.min(1, latencyMs / normalYMax))
    return plotBottom.value - ratio * (plotBottom.value - plotTop)
  }

  // With axis break:
  // Normal zone: between (breakY + 6) and plotBottom
  if (latencyMs <= normalYMax) {
    const ratio = Math.max(0, latencyMs / normalYMax)
    return plotBottom.value - ratio * (plotBottom.value - (breakY + 6))
  }

  // Outlier overflow zone: between (breakY - 4) and (plotTop + 3)
  const outlierRatio = Math.max(0, Math.min(1, (latencyMs - normalYMax) / Math.max(1, maxLatency - normalYMax)))
  return (breakY - 4) - outlierRatio * ((breakY - 4) - (plotTop + 3))
}

function dataY(sample: WorkbenchLatencySample): number {
  return dataYAtLatency(sample.latency_ms)
}

function xForTime(timestampMs: number): number {
  const span = timeBounds.value.max - timeBounds.value.min
  if (span <= 0) return plotLeft
  const ratio = Math.max(0, Math.min(1, (timestampMs - timeBounds.value.min) / span))
  return plotLeft + ratio * (plotRight.value - plotLeft)
}

const lodPlan = computed<LodPlan>(() => {
  return computeLodPlan({
    samples: props.samples,
    windowSince: props.windowSince,
    windowUntil: props.windowUntil,
    availableWidth: plotRight.value - plotLeft,
    normalYMax: scaleMetrics.value.normalYMax,
  })
})

const isBucketMode = computed(() => lodPlan.value.mode === 'bucket')

const activeBucket = computed<TimeBucket | null>(() => {
  if (!isBucketMode.value || !lodPlan.value.buckets?.length) return null
  if (hoveredBucketIndex.value !== null) {
    return lodPlan.value.buckets.find((b) => b.index === hoveredBucketIndex.value) || null
  }
  return lodPlan.value.buckets[lodPlan.value.buckets.length - 1] || null
})

const bucketTrendSegments = computed(() => {
  if (!isBucketMode.value || !lodPlan.value.buckets?.length) return []
  const points: { x: number; y: number }[] = []
  lodPlan.value.buckets.forEach((b) => {
    if (b.medianLatency !== null) {
      points.push({ x: xForTime(b.timeCenter), y: dataYAtLatency(b.medianLatency) })
    }
  })
  return points.length > 1 ? [points] : []
})

interface TestCluster {
  x: number
  indices: number[]
  medianLatency: number | null
  medianY: number | null
  count: number
}

const clusters = computed<TestCluster[]>(() => {
  if (props.samples.length === 0) return []
  const list: { indices: number[] }[] = []
  props.samples.forEach((sample, idx) => {
    const t = Date.parse(sample.timestamp || '')
    const x = xAt(idx)
    const existing = list.find((c) => {
      const firstSample = props.samples[c.indices[0]]
      const firstT = Date.parse(firstSample?.timestamp || '')
      if (props.windowSince && props.windowUntil) {
        return Number.isFinite(t) && Number.isFinite(firstT) && Math.abs(t - firstT) <= 25000
      }
      return Math.abs(xAt(c.indices[0]) - x) < 5
    })
    if (existing) {
      existing.indices.push(idx)
    } else {
      list.push({ indices: [idx] })
    }
  })

  return list.map((c) => {
    const avgX = c.indices.reduce((sum, i) => sum + xAt(i), 0) / c.indices.length
    const successful = c.indices.filter((i) => props.samples[i].success && props.samples[i].latency_ms > 0)
    if (successful.length === 0) {
      return {
        x: avgX,
        indices: c.indices,
        medianLatency: null,
        medianY: null,
        count: c.indices.length,
      }
    }
    const latencies = successful.map((i) => props.samples[i].latency_ms).sort((a, b) => a - b)
    const medianLatency = latencies[Math.floor(latencies.length / 2)]
    return {
      x: avgX,
      indices: c.indices,
      medianLatency,
      medianY: dataYAtLatency(medianLatency),
      count: c.indices.length,
    }
  })
})

function xForSample(index: number): number {
  const cluster = clusters.value.find((c) => c.indices.includes(index))
  return cluster ? cluster.x : xAt(index)
}

function findClusterMedianIndex(cluster: TestCluster): number {
  if (cluster.indices.length === 1) return cluster.indices[0]
  const successful = cluster.indices.filter((i) => props.samples[i].success && props.samples[i].latency_ms > 0)
  if (successful.length === 0) {
    return cluster.indices[Math.floor(cluster.indices.length / 2)]
  }
  const target = cluster.medianLatency ?? 0
  let bestIdx = successful[0]
  let minDiff = Number.POSITIVE_INFINITY
  for (const idx of successful) {
    const diff = Math.abs(props.samples[idx].latency_ms - target)
    if (diff < minDiff) {
      minDiff = diff
      bestIdx = idx
    }
  }
  return bestIdx
}

// Single Straight Trend Line connecting only cluster medians
const trendSegments = computed(() => {
  const segments: { x: number; y: number }[][] = []
  let current: { x: number; y: number }[] = []
  clusters.value.forEach((cluster) => {
    if (cluster.medianY !== null) {
      current.push({ x: cluster.x, y: cluster.medianY })
    } else {
      if (current.length > 0) segments.push(current)
      current = []
    }
  })
  if (current.length > 0) segments.push(current)
  return segments
})

const activeIndex = computed(() => {
  if (props.hoveredIndex !== null) return props.hoveredIndex
  if (props.pinnedIndex !== null) return props.pinnedIndex
  return props.samples.length > 0 ? props.samples.length - 1 : null
})

const activeSample = computed(() => {
  const index = activeIndex.value
  return index === null ? null : props.samples[index] || null
})

const activePointX = computed(() => {
  if (isBucketMode.value && activeBucket.value) {
    return xForTime(activeBucket.value.timeCenter)
  }
  if (activeIndex.value === null) return 0
  return xForSample(activeIndex.value)
})
const hoverLabel = computed(() => {
  if (isBucketMode.value && hoveredBucketIndex.value !== null && activeBucket.value) {
    return `${activeBucket.value.startLabel} · ${activeBucket.value.sampleCount} 条样本`
  }
  if (props.hoveredIndex === null || !activeSample.value) return ''
  return activeSampleLabel(activeSample.value)
})
const hoverLabelX = computed(() => Math.max(plotLeft + 80, Math.min(plotRight.value - 80, activePointX.value)))
const hoverLabelY = computed(() => {
  if (activeIndex.value === null) return plotTop + 22
  return Math.max(plotTop + 21, Math.min(plotBottom.value - 4, visualYFor(activeIndex.value) - 15))
})

const effectiveFocusedClusterIndex = computed<number | null>(() => {
  if (activeIndex.value !== null) {
    const idx = clusters.value.findIndex((c) => c.indices.includes(activeIndex.value!))
    if (idx !== -1) return idx
  }
  return null
})

interface SampleLayout {
  visualY: number
  sliceTop: number
  sliceBottom: number
  isOutlier: boolean
}

// Decoupled Visual & Interactive positions with Focus Lens
const visualLayout = computed<Record<number, SampleLayout>>(() => {
  const map: Record<number, SampleLayout> = {}
  if (!props.samples.length) return map

  const top = plotTop + 2
  const bottom = plotBottom.value - 2
  const focusedCluster = effectiveFocusedClusterIndex.value

  clusters.value.forEach((cluster, clusterIdx) => {
    const isFocused = clusterIdx === focusedCluster
    const count = cluster.indices.length

    if (count === 1) {
      const idx = cluster.indices[0]
      const y = dataY(props.samples[idx])
      map[idx] = {
        visualY: y,
        sliceTop: top,
        sliceBottom: bottom,
        isOutlier: scaleMetrics.value.hasAxisBreak && props.samples[idx].latency_ms > scaleMetrics.value.normalYMax,
      }
      return
    }

    // Sort cluster samples strictly by test sequence (#1 at top, #N at bottom)
    // so moving from top to bottom naturally and predictably scans 1 -> 2 -> ... -> N
    const sorted = [...cluster.indices].map((idx) => ({
      idx,
      sample: props.samples[idx],
      rawY: dataY(props.samples[idx]),
      latency: props.samples[idx].latency_ms,
      seq: props.samples[idx].seq ?? (idx + 1),
      isOutlier: scaleMetrics.value.hasAxisBreak && props.samples[idx].latency_ms > scaleMetrics.value.normalYMax,
    })).sort((a, b) => a.seq - b.seq)

    const sliceH = (bottom - top) / count
    sorted.forEach((item, r) => {
      const sTop = top + r * sliceH
      const sBottom = sTop + sliceH
      const focusY = (sTop + sBottom) / 2
      map[item.idx] = {
        visualY: isFocused ? focusY : item.rawY,
        sliceTop: sTop,
        sliceBottom: sBottom,
        isOutlier: item.isOutlier,
      }
    })
  })

  return map
})

function visualYFor(index: number): number {
  return visualLayout.value[index]?.visualY ?? dataY(props.samples[index])
}

function clusterMinVisualY(cluster: TestCluster): number {
  return Math.min(...cluster.indices.map((i) => visualYFor(i)))
}

function clusterMaxVisualY(cluster: TestCluster): number {
  return Math.max(...cluster.indices.map((i) => visualYFor(i)))
}

function diamondPoints(cx: number, cy: number, r = 4.5): string {
  return `${cx},${cy - r} ${cx + r},${cy} ${cx},${cy + r} ${cx - r},${cy}`
}

function trianglePoints(cx: number, cy: number, r = 4.5): string {
  return `${cx},${cy - r} ${cx + r * 0.866},${cy + r * 0.5} ${cx - r * 0.866},${cy + r * 0.5}`
}

function isClusterActive(index: number): boolean {
  if (effectiveFocusedClusterIndex.value === null) return false
  const cluster = clusters.value[effectiveFocusedClusterIndex.value]
  return cluster ? cluster.indices.includes(index) : false
}

// Interactive Hover Model:
// 1. Bucket Mode (Macro LOD): hover closest time bucket, no vertical jitter
// 2. Raw Mode (High Fidelity): Stage 1 horizontal cluster locking + Stage 2 intra-cluster Focus Lens when near
function onMouseMove(event: MouseEvent) {
  if (!props.samples.length || !svgRef.value) return
  const rect = svgRef.value.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  const viewX = ((event.clientX - rect.left) / rect.width) * effectiveWidth.value
  const viewY = ((event.clientY - rect.top) / rect.height) * props.height

  // Bucket Mode (LOD Macro Summary):
  if (isBucketMode.value && lodPlan.value.buckets?.length) {
    let bestBucket: TimeBucket | null = null
    let minDx = Number.POSITIVE_INFINITY
    lodPlan.value.buckets.forEach((b) => {
      const bx = xForTime(b.timeCenter)
      const dx = Math.abs(bx - viewX)
      if (dx < minDx) {
        minDx = dx
        bestBucket = b
      }
    })
    if (bestBucket) {
      hoveredBucketIndex.value = (bestBucket as TimeBucket).index
      emit('hover', (bestBucket as TimeBucket).representativeSampleIndex)
      emit('bucket-hover', bestBucket)
    }
    return
  }

  const clusterList = clusters.value
  if (!clusterList.length) return

  // Raw Mode: Find horizontally closest cluster (zero hysteresis, switches cleanly at midpoint)
  let closestClusterIdx = 0
  let minDx = Number.POSITIVE_INFINITY
  clusterList.forEach((c, idx) => {
    const dx = Math.abs(c.x - viewX)
    if (dx < minDx) {
      minDx = dx
      closestClusterIdx = idx
    }
  })

  const cluster = clusterList[closestClusterIdx]
  if (!cluster || !cluster.indices.length) return

  if (cluster.indices.length === 1) {
    emit('hover', cluster.indices[0])
    return
  }

  // Focus Lens: only engage vertical intra-cluster sample slicing if mouse is near the cluster column
  const clusterDistX = Math.abs(cluster.x - viewX)
  const isNearCluster = clusterDistX <= 18

  if (!isNearCluster) {
    emit('hover', findClusterMedianIndex(cluster))
    return
  }

  const top = plotTop + 2
  const bottom = plotBottom.value - 2
  const clampedY = Math.max(top, Math.min(bottom - 0.001, viewY))

  for (const idx of cluster.indices) {
    const slot = visualLayout.value[idx]
    if (slot && clampedY >= slot.sliceTop && clampedY < slot.sliceBottom) {
      emit('hover', idx)
      return
    }
  }

  let bestSampleIdx = cluster.indices[0]
  let minDy = Number.POSITIVE_INFINITY
  for (const idx of cluster.indices) {
    const slot = visualLayout.value[idx]
    if (slot) {
      const dy = Math.abs(slot.visualY - clampedY)
      if (dy < minDy) {
        minDy = dy
        bestSampleIdx = idx
      }
    }
  }
  emit('hover', bestSampleIdx)
}

function onMouseLeave() {
  hoveredBucketIndex.value = null
  emit('bucket-hover', null)
  emit('hover', null)
}

function onClick() {
  if (isBucketMode.value && activeBucket.value) {
    emit('bucket-click', activeBucket.value)
    return
  }
  if (activeIndex.value !== null) {
    emit('pin', activeIndex.value)
  }
}

function onWheel(event: WheelEvent) {
  if (!event.shiftKey || event.ctrlKey || event.metaKey || !event.deltaY) return
  if (props.samples.length) { event.preventDefault(); event.stopPropagation() }
  if (!props.samples.length) return
  const currentClusterIdx = effectiveFocusedClusterIndex.value ?? 0
  const cluster = clusters.value[currentClusterIdx]
  if (!cluster) return

  if (cluster.indices.length > 1) {
    // Sort cluster samples from top to bottom
    const sorted = [...cluster.indices].sort((a, b) => visualYFor(a) - visualYFor(b))
    const current = activeIndex.value ?? sorted[0]
    let pos = sorted.indexOf(current)
    if (pos === -1) pos = event.deltaY > 0 ? 0 : sorted.length - 1

    let nextPos = pos
    if (event.deltaY > 0) nextPos = Math.min(sorted.length - 1, pos + 1)
    else if (event.deltaY < 0) nextPos = Math.max(0, pos - 1)

    if (nextPos !== pos) {
      emit('hover', sorted[nextPos])
    }
  } else {
    // Single-sample cluster: step to adjacent clusters
    let nextCIdx = currentClusterIdx
    if (event.deltaY > 0) nextCIdx = Math.min(clusters.value.length - 1, currentClusterIdx + 1)
    else if (event.deltaY < 0) nextCIdx = Math.max(0, currentClusterIdx - 1)

    if (nextCIdx !== currentClusterIdx) {
      emit('hover', clusters.value[nextCIdx].indices[0])
    }
  }
}

function onKeydown(event: KeyboardEvent) {
  if (!props.samples.length) return
  const current = activeIndex.value ?? 0
  if (event.key === 'Escape') {
    emit('unpin')
    event.preventDefault()
    return
  }
  if (event.key === 'Enter' || event.key === ' ') {
    emit('pin', current)
    event.preventDefault()
    return
  }

  const currentCluster = clusters.value.find((c) => c.indices.includes(current))
  if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
    if (currentCluster && currentCluster.indices.length > 1) {
      const sorted = [...currentCluster.indices].sort((a, b) => visualYFor(a) - visualYFor(b))
      const pos = sorted.indexOf(current)
      if (pos !== -1) {
        let nextPos = pos
        if (event.key === 'ArrowDown') nextPos = Math.min(sorted.length - 1, pos + 1)
        if (event.key === 'ArrowUp') nextPos = Math.max(0, pos - 1)
        if (nextPos !== pos) {
          emit('hover', sorted[nextPos])
          event.preventDefault()
          return
        }
      }
    }
  }
  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight' || event.key === 'Home' || event.key === 'End') {
    let next = current
    if (event.key === 'ArrowLeft') next = Math.max(0, current - 1)
    if (event.key === 'ArrowRight') next = Math.min(props.samples.length - 1, current + 1)
    if (event.key === 'Home') next = 0
    if (event.key === 'End') next = props.samples.length - 1
    emit('hover', next)
    event.preventDefault()
  }
}

function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'
  const span = timeBounds.value.max - timeBounds.value.min
  if (span > 28 * 60 * 60 * 1000) {
    const m = String(date.getMonth() + 1).padStart(2, '0')
    const d = String(date.getDate()).padStart(2, '0')
    const hh = String(date.getHours()).padStart(2, '0')
    const mm = String(date.getMinutes()).padStart(2, '0')
    return `${m}/${d} ${hh}:${mm}`
  }
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

const boundsStartLabel = computed(() => {
  if (props.windowSince && props.windowUntil) {
    return formatTime(props.windowSince)
  }
  return props.samples.length ? formatTime(props.samples[0].timestamp) : ''
})

const boundsEndLabel = computed(() => {
  if (props.windowSince && props.windowUntil) {
    return formatTime(props.windowUntil)
  }
  return props.samples.length ? formatTime(props.samples[props.samples.length - 1].timestamp) : ''
})

function activeSampleLabel(sample: WorkbenchLatencySample | null): string {
  if (!sample) return '暂无原始样本'
  const seqPrefix = sample.seq ? `#${sample.seq} · ` : ''
  if (sample.success && sample.latency_ms > 0) return `${seqPrefix}${Math.round(sample.latency_ms)} ms`
  return `${seqPrefix}${/timeout|timed out|超时/i.test(sample.error || '') ? '超时' : '失败'}`
}
</script>

<template>
  <div ref="containerRef" class="relative w-full" :style="{ minHeight: `${height}px` }">
    <svg
      ref="svgRef"
      class="block w-full overflow-visible outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 rounded"
      :viewBox="`0 0 ${effectiveWidth} ${height}`"
      :height="height"
      preserveAspectRatio="none"
      role="img"
      tabindex="0"
      :aria-label="samples.length ? '延迟原始样本图，可用方向键浏览，回车固定，Esc 取消固定' : '暂无延迟样本'"
      @mousemove="onMouseMove"
      @mouseleave="onMouseLeave"
      @click="onClick"
      @keydown="onKeydown"
      @wheel="onWheel"
    >
      <!-- Axis Lines & Tick Labels -->
      <template v-if="scaleMetrics.hasAxisBreak">
        <!-- Overflow Top Axis (Outlier Region) -->
        <line :x1="plotLeft" :x2="plotRight" :y1="plotTop" :y2="plotTop" stroke="currentColor" stroke-opacity="0.10" />
        <text :x="plotLeft - 8" :y="plotTop + 4" text-anchor="end" fill="var(--warning)" class="text-[9px] font-semibold">▲ {{ Math.round(scaleMetrics.maxLatency) }} ms</text>

        <!-- Axis Break Line -->
        <line :x1="plotLeft" :x2="plotRight" :y1="scaleMetrics.breakY" :y2="scaleMetrics.breakY" stroke="currentColor" stroke-opacity="0.25" stroke-dasharray="3 3" />
        <text :x="plotLeft - 8" :y="scaleMetrics.breakY + 3" text-anchor="end" fill="var(--text-muted)" class="text-[9px]">{{ scaleMetrics.normalYMax }} ms</text>

        <!-- Normal Range Mid Line -->
        <line :x1="plotLeft" :x2="plotRight" :y1="(scaleMetrics.breakY + plotBottom) / 2" :y2="(scaleMetrics.breakY + plotBottom) / 2" stroke="currentColor" stroke-opacity="0.10" />
        <text :x="plotLeft - 8" :y="(scaleMetrics.breakY + plotBottom) / 2 + 4" text-anchor="end" fill="var(--text-muted)" class="text-[9px]">{{ Math.round(scaleMetrics.normalYMax / 2) }} ms</text>

        <!-- Baseline -->
        <line :x1="plotLeft" :x2="plotRight" :y1="plotBottom" :y2="plotBottom" stroke="currentColor" stroke-opacity="0.24" />
        <text :x="plotLeft - 8" :y="plotBottom + 4" text-anchor="end" fill="var(--text-muted)" class="text-[10px]">0</text>
      </template>
      <template v-else>
        <!-- Standard Continuous Axis -->
        <line :x1="plotLeft" :x2="plotRight" :y1="plotTop" :y2="plotTop" stroke="currentColor" stroke-opacity="0.12" />
        <line :x1="plotLeft" :x2="plotRight" :y1="(plotTop + plotBottom) / 2" :y2="(plotTop + plotBottom) / 2" stroke="currentColor" stroke-opacity="0.12" />
        <line :x1="plotLeft" :x2="plotRight" :y1="plotBottom" :y2="plotBottom" stroke="currentColor" stroke-opacity="0.24" />
        <text :x="plotLeft - 8" :y="plotTop + 4" text-anchor="end" fill="var(--text-muted)" class="text-[10px]">{{ scaleMetrics.normalYMax }} ms</text>
        <text :x="plotLeft - 8" :y="(plotTop + plotBottom) / 2 + 4" text-anchor="end" fill="var(--text-muted)" class="text-[9px]">{{ Math.round(scaleMetrics.normalYMax / 2) }} ms</text>
        <text :x="plotLeft - 8" :y="plotBottom + 4" text-anchor="end" fill="var(--text-muted)" class="text-[10px]">0</text>
      </template>

      <!-- Time Bounds Labels (only shown when showTimeBoundsLabels is true) -->
      <template v-if="showTimeBoundsLabels">
        <text :x="plotLeft" :y="height - 5" fill="var(--text-muted)" class="text-[10px]">{{ samples.length ? boundsStartLabel : '' }}</text>
        <text :x="plotRight" :y="height - 5" text-anchor="end" fill="var(--text-muted)" class="text-[10px]">{{ samples.length ? boundsEndLabel : '' }}</text>
      </template>

      <!-- 1. BUCKET MODE (LOD Trend Summary for 30d/180d or dense 7d) -->
      <template v-if="isBucketMode">
        <!-- Bucket Trend Polyline connecting all medians -->
        <polyline
          v-for="(segment, segmentIndex) in bucketTrendSegments"
          :key="`b-seg-${segmentIndex}`"
          :points="segment.map((p) => `${p.x},${p.y}`).join(' ')"
          fill="none"
          stroke="var(--primary)"
          stroke-width="1.8"
          stroke-linejoin="round"
          stroke-linecap="round"
          stroke-opacity="0.85"
        />

        <!-- Bucket Whisker Stems & Median Points -->
        <g v-for="b in lodPlan.buckets" :key="`bucket-${b.index}`">
          <!-- Whisker from P10 to P95 -->
          <line
            v-if="b.p10Latency !== null && b.p95Latency !== null && Math.abs(dataYAtLatency(b.p10Latency) - dataYAtLatency(b.p95Latency)) > 2"
            :x1="xForTime(b.timeCenter)"
            :x2="xForTime(b.timeCenter)"
            :y1="dataYAtLatency(b.p95Latency)"
            :y2="dataYAtLatency(b.p10Latency)"
            stroke="var(--primary)"
            stroke-width="1.5"
            :stroke-opacity="activeBucket?.index === b.index ? 0.6 : 0.25"
            stroke-linecap="round"
          />

          <!-- Median Diamond Point (◆) -->
          <polygon
            v-if="b.medianLatency !== null"
            :points="diamondPoints(xForTime(b.timeCenter), dataYAtLatency(b.medianLatency), activeBucket?.index === b.index ? 5.5 : 3.5)"
            :fill="activeBucket?.index === b.index ? 'var(--primary)' : 'var(--card-bg)'"
            :stroke="activeBucket?.index === b.index ? 'var(--card-bg)' : 'var(--primary)'"
            :stroke-width="activeBucket?.index === b.index ? 2 : 1.5"
            class="cursor-pointer transition-all duration-150"
          >
            <title>{{ `${b.startLabel} – ${b.endLabel} · ${b.testCount} 次测试 (${b.sampleCount} 条样本) · P50: ${Math.round(b.medianLatency)} ms · P95: ${Math.round(b.p95Latency || 0)} ms · 成功率: ${(b.successRate * 100).toFixed(0)}%` }}</title>
          </polygon>

          <!-- Outlier Warning Spike (▲) -->
          <polygon
            v-if="b.hasOutlier && b.maxLatency !== null"
            :points="trianglePoints(xForTime(b.timeCenter), dataYAtLatency(b.maxLatency), 3.5)"
            fill="var(--warning)"
            fill-opacity="0.85"
            stroke="var(--card-bg)"
            stroke-width="0.8"
          >
            <title>{{ `${b.startLabel} · 出现极端延迟尖刺 (${Math.round(b.maxLatency)} ms)` }}</title>
          </polygon>

          <!-- Failure indicator pip at baseline -->
          <circle
            v-if="b.failCount > 0"
            :cx="xForTime(b.timeCenter)"
            :cy="plotBottom - 2"
            r="2"
            fill="var(--danger)"
          >
            <title>{{ `${b.startLabel} · 包含 ${b.failCount} 次超时/失败` }}</title>
          </circle>
        </g>
      </template>

      <!-- 2. RAW EVIDENCE MODE (4h, 24h, or sparse 7d) -->
      <template v-else>
        <!-- 2. Vertical Whisker Stems for Multi-Sample Clusters (Discrete Test Columns) -->
        <g v-for="(cluster, cIdx) in clusters" :key="`cluster-${cIdx}`">
          <template v-if="cluster.indices.length > 1">
            <line
              v-if="clusterMaxVisualY(cluster) - clusterMinVisualY(cluster) > 2"
              :x1="cluster.x"
              :x2="cluster.x"
              :y1="clusterMinVisualY(cluster)"
              :y2="clusterMaxVisualY(cluster)"
              stroke="var(--primary)"
              stroke-width="1.5"
              :stroke-opacity="effectiveFocusedClusterIndex === cIdx ? 0.45 : 0.18"
              stroke-linecap="round"
              class="transition-all duration-150"
            />
            <line
              v-if="clusterMaxVisualY(cluster) - clusterMinVisualY(cluster) > 4"
              :x1="cluster.x - 2.5"
              :x2="cluster.x + 2.5"
              :y1="clusterMinVisualY(cluster)"
              :y2="clusterMinVisualY(cluster)"
              stroke="var(--primary)"
              stroke-width="1"
              :stroke-opacity="effectiveFocusedClusterIndex === cIdx ? 0.6 : 0.25"
              class="transition-all duration-150"
            />
            <line
              v-if="clusterMaxVisualY(cluster) - clusterMinVisualY(cluster) > 4"
              :x1="cluster.x - 2.5"
              :x2="cluster.x + 2.5"
              :y1="clusterMaxVisualY(cluster)"
              :y2="clusterMaxVisualY(cluster)"
              stroke="var(--primary)"
              stroke-width="1"
              :stroke-opacity="effectiveFocusedClusterIndex === cIdx ? 0.6 : 0.25"
              class="transition-all duration-150"
            />
          </template>
        </g>

        <!-- 1. Single Straight Trend Line connecting only Cluster Medians (P50) -->
        <polyline
          v-for="(segment, segmentIndex) in trendSegments"
          :key="`segment-${segmentIndex}`"
          :points="segment.map((p) => `${p.x},${p.y}`).join(' ')"
          fill="none"
          stroke="var(--primary)"
          stroke-width="1.8"
          stroke-linejoin="round"
          stroke-linecap="round"
          stroke-opacity="0.85"
        />

        <!-- Cluster Median Diamond Points (◆) on the Trend Line -->
        <g v-for="(cluster, cIdx) in clusters" :key="`median-${cIdx}`">
          <polygon
            v-if="cluster.medianY !== null"
            :points="diamondPoints(cluster.x, cluster.medianY, effectiveFocusedClusterIndex === cIdx ? 5.5 : 4)"
            :fill="effectiveFocusedClusterIndex === cIdx ? 'var(--primary)' : 'var(--card-bg)'"
            :stroke="effectiveFocusedClusterIndex === cIdx ? 'var(--card-bg)' : 'var(--primary)'"
            :stroke-width="effectiveFocusedClusterIndex === cIdx ? 2 : 1.5"
            class="transition-all duration-150"
          >
            <title>{{ `本次测试代表值 (中位数 P50): ${Math.round(cluster.medianLatency || 0)} ms` }}</title>
          </polygon>
        </g>

        <!-- Raw Sample Distribution Points (● / ▲ / ✕) -->
        <g v-for="(sample, index) in samples" :key="`${sample.seq}-${sample.timestamp}-${index}`">
          <!-- Outlier sample in resting state: rendered as restrained triangle glyph ▲ -->
          <template v-if="visualLayout[index]?.isOutlier && effectiveFocusedClusterIndex !== clusters.findIndex(c => c.indices.includes(index))">
            <polygon
              :points="trianglePoints(xForSample(index), visualYFor(index), 3.5)"
              fill="var(--warning)"
              fill-opacity="0.85"
              stroke="var(--card-bg)"
              stroke-width="0.8"
              class="cursor-pointer transition-all duration-150"
            >
              <title>{{ `#${sample.seq || index + 1} · ${Math.round(sample.latency_ms)} ms (极端尖刺) · ${formatTime(sample.timestamp)}` }}</title>
            </polygon>
          </template>

          <!-- Normal successful sample: rendered as circle ● -->
          <template v-else-if="sample.success && sample.latency_ms > 0">
            <circle
              :cx="xForSample(index)"
              :cy="visualYFor(index)"
              :r="activeIndex === index ? 5.5 : isClusterActive(index) ? 4 : 2"
              :fill="activeIndex === index ? 'var(--primary)' : 'var(--primary)'"
              :fill-opacity="activeIndex === index ? 1 : isClusterActive(index) ? 1.0 : 0.35"
              :stroke="activeIndex === index ? 'var(--card-bg)' : pinnedIndex === index ? 'var(--text-main)' : 'var(--card-bg)'"
              :stroke-width="activeIndex === index ? 2 : isClusterActive(index) ? 1.5 : 0.5"
              class="cursor-pointer transition-all duration-150"
            >
              <title>{{ `#${sample.seq || index + 1} · ${Math.round(sample.latency_ms)} ms · ${formatTime(sample.timestamp)}` }}</title>
            </circle>
          </template>

          <!-- Failed sample: rendered as cross ✕ -->
          <g v-else :transform="`translate(${xForSample(index)},${visualYFor(index)})`" stroke="var(--danger)" stroke-width="1.8" stroke-linecap="round" class="cursor-pointer">
            <title>{{ `${activeSampleLabel(sample)} · ${sample.error || '连接失败'} · ${formatTime(sample.timestamp)}` }}</title>
            <line x1="-3.5" y1="-3.5" x2="3.5" y2="3.5" />
            <line x1="3.5" y1="-3.5" x2="-3.5" y2="3.5" />
          </g>
        </g>
      </template>

      <!-- Active vertical crosshair guideline -->
      <line
        v-if="activePointX > 0"
        :x1="activePointX"
        :x2="activePointX"
        :y1="plotTop"
        :y2="plotBottom"
        :stroke="pinnedIndex === activeIndex ? 'var(--text-main)' : 'var(--primary)'"
        stroke-dasharray="3 3"
        stroke-opacity="0.6"
      />

      <!-- Active item horizontal tick on stem -->
      <template v-if="isBucketMode">
        <line
          v-if="activeBucket && activeBucket.medianLatency !== null"
          :x1="activePointX - 8"
          :x2="activePointX + 8"
          :y1="dataYAtLatency(activeBucket.medianLatency)"
          :y2="dataYAtLatency(activeBucket.medianLatency)"
          stroke="var(--primary)"
          stroke-width="2"
        />
        <circle
          v-if="activeBucket && activeBucket.medianLatency !== null"
          :cx="activePointX"
          :cy="dataYAtLatency(activeBucket.medianLatency)"
          r="5.5"
          fill="var(--primary)"
          stroke="var(--card-bg)"
          stroke-width="2"
          class="pointer-events-none"
        />
      </template>
      <template v-else>
        <line
          v-if="activeIndex !== null && activeSample && activeSample.success && activeSample.latency_ms > 0"
          :x1="activePointX - 8"
          :x2="activePointX + 8"
          :y1="visualYFor(activeIndex)"
          :y2="visualYFor(activeIndex)"
          stroke="var(--primary)"
          stroke-width="2"
        />
        <circle
          v-if="activeIndex !== null && activeSample && activeSample.success && activeSample.latency_ms > 0"
          :cx="activePointX"
          :cy="visualYFor(activeIndex)"
          r="5.5"
          fill="var(--primary)"
          stroke="var(--card-bg)"
          stroke-width="2"
          class="pointer-events-none"
        />
      </template>

      <!-- Interactive transparent hit overlay -->
      <rect
        :x="plotLeft"
        :y="plotTop"
        :width="plotRight - plotLeft"
        :height="plotBottom - plotTop"
        fill="transparent"
        class="cursor-crosshair"
      />
      <g v-if="hoverLabel" class="pointer-events-none sample-hover-label">
        <rect :x="hoverLabelX - 78" :y="hoverLabelY - 16" width="156" height="25" rx="5" fill="var(--card-bg)" stroke="var(--border)" />
        <text :x="hoverLabelX" :y="hoverLabelY + 1" text-anchor="middle" fill="var(--text-main)" class="text-[11px] font-semibold">{{ hoverLabel }}</text>
      </g>
    </svg>
  </div>
</template>
