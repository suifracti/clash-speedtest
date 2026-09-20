<script setup lang="ts">
import { computed, ref } from 'vue'
import type { WorkbenchLatencySample } from '../../types'

const props = withDefaults(defineProps<{
  samples: WorkbenchLatencySample[]
  width?: number
  height?: number
  hoveredIndex: number | null
  pinnedIndex: number | null
}>(), {
  width: 760,
  height: 112,
})

const emit = defineEmits<{
  (event: 'hover', index: number | null): void
  (event: 'pin', index: number): void
  (event: 'unpin'): void
}>()

const svgRef = ref<SVGSVGElement | null>(null)
const plotLeft = 52
const plotRight = computed(() => props.width - 16)
const plotTop = 16
const plotBottom = computed(() => props.height - 26)

const successSamples = computed(() => props.samples.filter((sample) => sample.success && sample.latency_ms > 0))
const yMax = computed(() => {
  const max = Math.max(...successSamples.value.map((sample) => sample.latency_ms), 0)
  return Math.max(100, Math.ceil((max * 1.15) / 10) * 10)
})

const timeBounds = computed(() => {
  const times = props.samples
    .map((sample) => Date.parse(sample.timestamp))
    .filter((time) => Number.isFinite(time))
  if (times.length === 0) return { min: 0, max: 1 }
  const min = Math.min(...times)
  const max = Math.max(...times)
  return min === max ? { min: min - 500, max: max + 500 } : { min, max }
})

function xAt(index: number): number {
  const timestamp = Date.parse(props.samples[index]?.timestamp || '')
  if (!Number.isFinite(timestamp)) return plotLeft
  const span = timeBounds.value.max - timeBounds.value.min
  return plotLeft + ((timestamp - timeBounds.value.min) / span) * (plotRight.value - plotLeft)
}

function yAt(sample: WorkbenchLatencySample): number {
  const value = Math.max(0, sample.latency_ms)
  return plotBottom.value - (value / yMax.value) * (plotBottom.value - plotTop)
}

const contiguousSuccessSegments = computed(() => {
  const segments: number[][] = []
  let current: number[] = []
  props.samples.forEach((sample, index) => {
    if (sample.success && sample.latency_ms > 0) {
      current.push(index)
      return
    }
    if (current.length > 0) segments.push(current)
    current = []
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

function nearestIndex(clientX: number): number | null {
  if (!props.samples.length || !svgRef.value) return null
  const rect = svgRef.value.getBoundingClientRect()
  if (rect.width <= 0) return null
  const viewX = ((clientX - rect.left) / rect.width) * props.width
  let nearest = 0
  let distance = Number.POSITIVE_INFINITY
  props.samples.forEach((_sample, index) => {
    const currentDistance = Math.abs(xAt(index) - viewX)
    if (currentDistance < distance) {
      distance = currentDistance
      nearest = index
    }
  })
  return nearest
}

function onMouseMove(event: MouseEvent) {
  emit('hover', nearestIndex(event.clientX))
}

function onClick(event: MouseEvent) {
  const index = nearestIndex(event.clientX)
  if (index !== null) emit('pin', index)
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
  return Number.isNaN(date.getTime()) ? '时间未知' : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function segmentPoints(segment: number[]): string {
  return segment.map((index) => `${xAt(index)},${yAt(props.samples[index])}`).join(' ')
}

function activeSampleLabel(sample: WorkbenchLatencySample | null): string {
  if (!sample) return '暂无原始样本'
  if (sample.success && sample.latency_ms > 0) return `${Math.round(sample.latency_ms)} ms`
  return /timeout|timed out|超时/i.test(sample.error || '') ? '超时' : '失败'
}

function activeSampleDetail(sample: WorkbenchLatencySample | null): string {
  if (!sample) return ''
  const date = new Date(sample.timestamp)
  const time = Number.isNaN(date.getTime()) ? '时间未知' : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  return sample.success && sample.latency_ms > 0 ? `${time} · 成功` : `${time} · ${sample.error || '连接失败'}`
}
</script>

<template>
  <div class="relative w-full" :style="{ minHeight: `${height}px` }">
    <svg
      ref="svgRef"
      class="block w-full overflow-visible outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 rounded"
      :viewBox="`0 0 ${width} ${height}`"
      :height="height"
      role="img"
      tabindex="0"
      :aria-label="samples.length ? '延迟原始样本图，可用方向键浏览，回车固定，Esc 取消固定' : '暂无延迟样本'"
      @mousemove="onMouseMove"
      @mouseleave="emit('hover', null)"
      @click="onClick"
      @keydown="onKeydown"
    >
      <line :x1="plotLeft" :x2="plotRight" :y1="plotTop" :y2="plotTop" stroke="currentColor" stroke-opacity="0.12" />
      <line :x1="plotLeft" :x2="plotRight" :y1="(plotTop + plotBottom) / 2" :y2="(plotTop + plotBottom) / 2" stroke="currentColor" stroke-opacity="0.12" />
      <line :x1="plotLeft" :x2="plotRight" :y1="plotBottom" :y2="plotBottom" stroke="currentColor" stroke-opacity="0.24" />
      <text :x="plotLeft - 8" :y="plotTop + 4" text-anchor="end" class="fill-content-muted text-[10px]">{{ yMax }} ms</text>
      <text :x="plotLeft - 8" :y="plotBottom + 4" text-anchor="end" class="fill-content-muted text-[10px]">0</text>
      <text :x="plotLeft" :y="height - 5" class="fill-content-muted text-[10px]">{{ samples.length ? formatTime(samples[0].timestamp) : '' }}</text>
      <text :x="plotRight" :y="height - 5" text-anchor="end" class="fill-content-muted text-[10px]">{{ samples.length ? formatTime(samples[samples.length - 1].timestamp) : '' }}</text>

      <polyline
        v-for="(segment, segmentIndex) in contiguousSuccessSegments"
        :key="`segment-${segmentIndex}`"
        :points="segmentPoints(segment)"
        fill="none"
        stroke="#2563eb"
        stroke-width="1.8"
        stroke-linejoin="round"
        stroke-linecap="round"
      />

      <g v-for="(sample, index) in samples" :key="`${sample.seq}-${sample.timestamp}`">
        <circle
          v-if="sample.success && sample.latency_ms > 0"
          :cx="xAt(index)"
          :cy="yAt(sample)"
          :r="activeIndex === index ? 4.5 : 2.5"
          :fill="activeIndex === index ? '#1d4ed8' : '#60a5fa'"
          :stroke="pinnedIndex === index ? '#0f172a' : 'none'"
          :stroke-width="pinnedIndex === index ? 1.5 : 0"
        ><title>{{ sample.success ? `${Math.round(sample.latency_ms)} ms · ${formatTime(sample.timestamp)}` : `${activeSampleLabel(sample)} · ${sample.error || '连接失败'} · ${formatTime(sample.timestamp)}` }}</title></circle>
        <g v-else :transform="`translate(${xAt(index)},${plotBottom})`" stroke="#dc2626" stroke-width="1.8" stroke-linecap="round">
          <title>{{ `${activeSampleLabel(sample)} · ${sample.error || '连接失败'} · ${formatTime(sample.timestamp)}` }}</title>
          <line x1="-4" y1="-4" x2="4" y2="4" />
          <line x1="4" y1="-4" x2="-4" y2="4" />
        </g>
      </g>

      <line
        v-if="activeIndex !== null"
        :x1="xAt(activeIndex)"
        :x2="xAt(activeIndex)"
        :y1="plotTop"
        :y2="plotBottom"
        :stroke="pinnedIndex === activeIndex ? '#0f172a' : '#2563eb'"
        stroke-dasharray="3 3"
        stroke-opacity="0.75"
      />
      <rect
        :x="plotLeft"
        :y="plotTop"
        :width="plotRight - plotLeft"
        :height="plotBottom - plotTop"
        fill="transparent"
        class="cursor-crosshair"
      />
    </svg>
    <div v-if="activeSample" class="plot-readout" :class="{ pinned: pinnedIndex !== null }" aria-live="polite">
      <strong>{{ activeSampleLabel(activeSample) }}</strong>
      <span>{{ activeSampleDetail(activeSample) }}</span>
      <em v-if="pinnedIndex !== null">已固定</em>
    </div>
  </div>
</template>

<style scoped>
.plot-readout { position: absolute; top: 2px; right: 2px; z-index: 3; display: flex; align-items: baseline; gap: 6px; max-width: calc(100% - 8px); padding: 3px 6px; overflow: hidden; border: 1px solid var(--border, #d9e0e6); background: rgba(255, 255, 255, .92); color: var(--text-main, #18212b); font-size: 10px; line-height: 1.2; pointer-events: none; white-space: nowrap; }
.plot-readout strong { font-size: 11px; }.plot-readout span { min-width: 0; overflow: hidden; color: var(--text-secondary, #66727d); text-overflow: ellipsis; }.plot-readout em { color: var(--primary, #246b86); font-size: 9px; font-style: normal; font-weight: 700; }
</style>
