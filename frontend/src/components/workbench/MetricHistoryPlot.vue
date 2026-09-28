<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'

export interface MetricHistoryPoint {
  id: string
  timestamp: string
  value: number | null
  label: string
  detail: string
  success: boolean
}

const props = withDefaults(defineProps<{
  points: MetricHistoryPoint[]
  unit: string
  windowSince: string
  windowUntil: string
  hoveredIndex: number | null
  pinnedIndex: number | null
  height?: number
  width?: number
}>(), { height: 132 })

const emit = defineEmits<{
  (event: 'hover', index: number | null): void
  (event: 'pin', index: number): void
  (event: 'unpin'): void
}>()

const container = ref<HTMLDivElement | null>(null)
function onWheel(event: WheelEvent) {
  if (!event.shiftKey || event.ctrlKey || event.metaKey || !event.deltaY || !props.points.length) return
  event.preventDefault()
  event.stopPropagation()
  step(event.deltaY > 0 ? 1 : -1)
}
const svg = ref<SVGSVGElement | null>(null)
const measuredWidth = ref(760)
let observer: ResizeObserver | null = null

onMounted(() => {
  if (!container.value) return
  const initialWidth = container.value.getBoundingClientRect().width
  if (initialWidth > 0) measuredWidth.value = Math.round(initialWidth)
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width
      if (width && width > 0) measuredWidth.value = Math.round(width)
    })
    observer.observe(container.value)
  }
})
onUnmounted(() => observer?.disconnect())

const chartWidth = computed(() => props.width || measuredWidth.value)
const left = 58
const right = computed(() => chartWidth.value - 16)
const top = 18
const bottom = computed(() => props.height - 27)
const bounds = computed(() => {
  const since = Date.parse(props.windowSince)
  const until = Date.parse(props.windowUntil)
  if (Number.isFinite(since) && Number.isFinite(until) && until > since) return { since, until }
  const times = props.points.map((point) => Date.parse(point.timestamp)).filter(Number.isFinite)
  return { since: Math.min(...times, Date.now()), until: Math.max(...times, Date.now() + 1) }
})
const maxValue = computed(() => {
  const highest = Math.max(0, ...props.points.map((point) => point.value || 0))
  if (highest <= 0) return 100
  const step = 10 ** Math.floor(Math.log10(highest))
  return Math.ceil(highest * 1.15 / step) * step
})
const activeIndex = computed(() => props.hoveredIndex ?? props.pinnedIndex ?? (props.points.length ? props.points.length - 1 : null))
const activePoint = computed(() => activeIndex.value === null ? null : props.points[activeIndex.value] || null)

function xAt(index: number): number {
  const time = Date.parse(props.points[index]?.timestamp || '')
  if (!Number.isFinite(time)) return left
  const ratio = Math.max(0, Math.min(1, (time - bounds.value.since) / (bounds.value.until - bounds.value.since)))
  return left + ratio * (right.value - left)
}
function yAt(index: number): number {
  const value = props.points[index]?.value
  return value === null || value === undefined ? bottom.value - 2 : bottom.value - Math.max(0, Math.min(1, value / maxValue.value)) * (bottom.value - top)
}
const segments = computed(() => {
  const result: string[] = []
  let current: string[] = []
  props.points.forEach((point, index) => {
    if (point.success && point.value !== null) current.push(`${xAt(index)},${yAt(index)}`)
    else if (current.length) { result.push(current.join(' ')); current = [] }
  })
  if (current.length) result.push(current.join(' '))
  return result
})

function move(event: MouseEvent): void {
  if (!svg.value || !props.points.length) return
  const rect = svg.value.getBoundingClientRect()
  if (!rect.width || !rect.height) return
  const x = (event.clientX - rect.left) / rect.width * chartWidth.value
  const y = (event.clientY - rect.top) / rect.height * props.height
  let best = 0
  let distance = Number.POSITIVE_INFINITY
  props.points.forEach((_, index) => {
    const dx = Math.abs(xAt(index) - x)
    // Near-simultaneous attempts remain individually selectable up/down.
    const score = dx <= 8 ? dx + Math.abs(yAt(index) - y) * 0.12 : dx
    if (score < distance) { distance = score; best = index }
  })
  emit('hover', best)
}
function step(direction: number): void {
  if (!props.points.length) return
  emit('hover', Math.max(0, Math.min(props.points.length - 1, (activeIndex.value ?? 0) + direction)))
}
function keydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') { emit('unpin'); event.preventDefault() }
  else if ((event.key === 'Enter' || event.key === ' ') && activeIndex.value !== null) { emit('pin', activeIndex.value); event.preventDefault() }
  else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') { step(-1); event.preventDefault() }
  else if (event.key === 'ArrowRight' || event.key === 'ArrowDown') { step(1); event.preventDefault() }
}
const labelX = computed(() => activeIndex.value === null ? left : Math.max(left + 70, Math.min(right.value - 70, xAt(activeIndex.value))))
const labelY = computed(() => activeIndex.value === null ? top : Math.max(top + 22, Math.min(bottom.value - 2, yAt(activeIndex.value) - 14)))
</script>

<template>
  <div ref="container" class="metric-plot" :style="{ minHeight: `${height}px` }">
    <div v-if="!points.length" class="metric-empty" role="status">当前窗口暂无{{ unit === 'Mbps' ? '下载测速' : '该服务检测' }}记录</div>
    <svg v-else ref="svg" :viewBox="`0 0 ${chartWidth} ${height}`" :height="height" preserveAspectRatio="none"
      role="img" tabindex="0" :aria-label="points.length ? `${unit}历史走势图，可悬停或使用方向键选择记录，回车固定` : `暂无${unit}历史`"
      @mousemove="move" @mouseleave="emit('hover', null)" @click="activeIndex !== null && emit('pin', activeIndex)"
      @keydown="keydown" @wheel="onWheel">
      <line v-for="ratio in [0, 0.5, 1]" :key="ratio" :x1="left" :x2="right" :y1="bottom - ratio * (bottom - top)" :y2="bottom - ratio * (bottom - top)" stroke="currentColor" stroke-opacity="0.13" />
      <text :x="left - 8" :y="top + 4" text-anchor="end">{{ Math.round(maxValue) }} {{ unit }}</text>
      <text :x="left - 8" :y="bottom + 4" text-anchor="end">0</text>
      <polyline v-for="(pointsText, index) in segments" :key="index" :points="pointsText" fill="none" stroke="var(--primary)" stroke-width="1.8" />
      <g v-for="(point, index) in points" :key="point.id">
        <circle :cx="xAt(index)" :cy="yAt(index)" :r="activeIndex === index ? 5 : 3.5"
          :fill="point.success ? 'var(--primary)' : 'var(--danger)'" stroke="var(--card-bg)" stroke-width="1.5" />
      </g>
      <line v-if="activeIndex !== null" :x1="xAt(activeIndex)" :x2="xAt(activeIndex)" :y1="top" :y2="bottom" stroke="var(--primary)" stroke-opacity="0.45" stroke-dasharray="3 3" class="pointer-events-none" />
      <g v-if="hoveredIndex !== null && activePoint" class="metric-hover-label pointer-events-none">
        <rect :x="labelX - 68" :y="labelY - 15" width="136" height="25" rx="5" fill="var(--card-bg)" stroke="var(--border)" />
        <text :x="labelX" :y="labelY + 1" text-anchor="middle">{{ activePoint.label }}</text>
      </g>
      <rect :x="left" :y="top" :width="right - left" :height="bottom - top" fill="transparent" class="cursor-crosshair" />
    </svg>
  </div>
</template>

<style scoped>
.metric-plot { width: 100%; min-width: 0; }
.metric-empty { display: grid; place-items: center; min-height: 132px; border-top: 1px solid var(--border); border-bottom: 1px solid var(--border); color: var(--text-muted); font-size: 12px; }
.metric-plot svg { display: block; width: 100%; outline: none; }
.metric-plot svg:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
.metric-plot text { fill: var(--text-muted); font-size: 10px; }
.metric-hover-label text { fill: var(--text-main); font-size: 11px; font-weight: 700; }
</style>
